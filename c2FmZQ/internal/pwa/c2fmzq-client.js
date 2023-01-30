/*
 * Copyright 2021-2023 TTBT Enterprises LLC
 *
 * This file is part of c2FmZQ (https://c2FmZQ.org/).
 *
 * c2FmZQ is free software: you can redistribute it and/or modify it under the
 * terms of the GNU General Public License as published by the Free Software
 * Foundation, either version 3 of the License, or (at your option) any later
 * version.
 *
 * c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
 * WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
 * A PARTICULAR PURPOSE. See the GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with
 * c2FmZQ. If not, see <https://www.gnu.org/licenses/>.
 */

/* jshint -W083 */
/* jshint -W097 */
'use strict';

let so;

/**
 * c2FmZQ / Stingle client.
 *
 * @class
 */
class c2FmZQClient {
  #options;
  #skey;
  #store;
  #sw;
  #capabilities;
  #state;

  constructor(options) {
    options = options || {};
    options.pathPrefix = options.pathPrefix || '/';
    this.#options = options;
    this.#store = options.store;
    this.#sw = options.sw;
    this.#capabilities = options.capabilities || [];
    this.#state = {};
    this.vars_ = {};
    this.resetDB_();
  }

  /*
   * Initialize / restore saved data.
   */
  async init() {
    await this.sodiumInit();
    return Promise.all([
      this.loadVars_(),
      this.#store.get('albums').then(v => {
        this.db_.albums = v || {};
      }),
      this.#store.get('contacts').then(v => {
        this.db_.contacts = v || {};
      }),
    ])
    .then(async () => {
      if (this.vars_.sk) {
        // Clear state from older version
        this.resetDB_();
        await this.#store.clear();
        await self.caches.delete('local');
        return this.init();
      }
      this.cache_ = await self.caches.open('local');
      if ('connection' in navigator) {
        this.#state.currentNetworkType = navigator.connection.type;
        navigator.connection.addEventListener('change', event => {
          if (this.#state.currentNetworkType !== navigator.connection.type) {
            console.log(`SW network changed ${this.currentNetworkType_} -> ${navigator.connection.type}`);
            this.#state.currentNetworkType = navigator.connection.type;
            this.onNetworkChange(event);
          }
        });
      }
      if (this.vars_.maxCacheSize === undefined) {
        if ('estimate' in navigator.storage) {
          const est = await navigator.storage.estimate();
          this.vars_.maxCacheSize = 0.9*est.quota;
        } else {
          this.vars_.maxCacheSize = 10*1024*1024*1024; // 10 GB
        }
      }
      if (this.vars_.cachePref === undefined) {
        this.vars_.cachePref = 'encrypted';
        this.vars_.downloadOnMobile = false;
        this.vars_.prefetchThumbnails = false;
      }
      this.cm_ = new CacheManager(this.#store, this.cache_, this.vars_.maxCacheSize);
    });
  }

  /*
   */
  async saveVars_() {
    return this.#store.set('vars', this.vars_);
  }

  /*
   */
  async loadVars_() {
    this.vars_ = await this.#store.get('vars') || {};
    for (let v of ['albumsTimeStamp', 'galleryTimeStamp', 'trashTimeStamp', 'albumFilesTimeStamp', 'contactsTimeStamp', 'deletesTimeStamp']) {
      if (this.vars_[v] === undefined) {
        this.vars_[v] = 0;
      }
    }
    if (this.vars_.server === undefined) {
      this.vars_.server = this.#options.pathPrefix;
    }
    if (this.vars_.decryptPath === undefined) {
      this.vars_.decryptPath = (await so.randombytes(16)).toString('hex');
      return this.saveVars_();
    }
  }

  /*
   */
  resetDB_() {
    this.db_ = {
      albums: {},
      contacts: {},
    };
  }

  async sodiumInit() {
    if (so) return;
    const sodium = new SodiumWrapper();
    await sodium.init();
    so = sodium;
  }

  async #setSessionKey(opt) {
    opt = opt || {};
    if (!opt.reset && this.#skey) {
      return;
    }
    let skey;
    if (!opt.reset) {
      skey = await this.#store.get('skey');
    }
    if (!skey) {
      skey = self.base64StdEncode(await so.secretbox_keygen());
      this.#store.set('skey', skey);
    }
    this.#skey = () => skey;
  }

  async #encrypt(data) {
    await this.#setSessionKey();
    const nonce = await so.randombytes(24);
    const ct = new Uint8Array(await so.secretbox(data, nonce, this.#skey()));
    const res = new Uint8Array(nonce.byteLength + ct.byteLength);
    res.set(nonce);
    res.set(ct, nonce.byteLength);
    return self.base64StdEncode(res);
  }

  async #decrypt(data) {
    await this.#setSessionKey();
    data = self.base64DecodeToBytes(data);
    try {
      return so.secretbox_open(data.slice(24), data.slice(0, 24), this.#skey());
    } catch (error) {
      console.error('SW #decrypt', error);
      throw error;
    }
  }

  async #encryptString(data) {
    return this.#encrypt(self.bytesFromString(data));
  }
  async #decryptString(data) {
    return this.#decrypt(data).then(r => self.bytesToString(r));
  }

  async #sk() {
    if (!this.vars_.esk) {
      return this.logout('');
    }
    return this.#decrypt(this.vars_.esk);
  }

  async #token() {
    if (!this.vars_.etoken) {
      return null;
    }
    return this.#decryptString(this.vars_.etoken);
  }

  /*
   */
  async isLoggedIn(clientId) {
    const token = await this.#token();
    const loggedIn = typeof token === "string" && token !== '' ? this.vars_.email : '';
    const needKey = this.vars_.esk === undefined;
    return Promise.resolve({
      account: loggedIn,
      isAdmin: this.vars_.isAdmin,
      needKey: needKey
    });
  }

  async quota(clientId) {
    return Promise.resolve({
      usage: this.vars_.spaceUsed,
      quota: this.vars_.spaceQuota,
    });
  }

  async passwordForLogin_(salt, password) {
    return so.pwhash(64, password, salt,
      so.PWHASH_OPSLIMIT_MODERATE,
      so.PWHASH_MEMLIMIT_MODERATE,
      so.PWHASH_ALG_ARGON2ID13)
      .then(p => p.toString('hex').toUpperCase());
  }

  async passwordForEncryption_(salt, password) {
    return so.pwhash(32, password, salt,
      so.PWHASH_OPSLIMIT_MODERATE,
      so.PWHASH_MEMLIMIT_MODERATE,
      so.PWHASH_ALG_ARGON2ID13);
  }

  async passwordForValidation_(salt, password) {
    return so.hex2bin(salt)
      .then(salt => {
        return so.pwhash(128, password, salt,
          so.PWHASH_OPSLIMIT_INTERACTIVE,
          so.PWHASH_MEMLIMIT_MODERATE,
          so.PWHASH_ALG_ARGON2ID13);
      })
      .then(p => p.toString('hex').toUpperCase());
  }

  /*
   * Perform the login sequence:
   * - hash the password
   * - send login request
   * - decode / decrypt the keybundle
   */
  async login(clientId, args) {
    const {email, password, server} = args;
    console.log(`SW login ${email}`);
    if (!SAMEORIGIN) {
      this.vars_.server = server || this.vars_.server;
    }

    return this.sendRequest_(clientId, 'v2/login/preLogin', {email})
      .then(async resp => {
        console.log('SW hashing password');
        this.vars_.loginSalt = resp.parts.salt;
        const salt = await so.hex2bin(resp.parts.salt);
        const hashed = await this.passwordForLogin_(salt, password);
        return this.sendRequest_(clientId, 'v2/login/login', {email: email, password: hashed});
      })
      .then(async resp => {
        if (resp.status !== 'ok') {
          throw resp.status;
        }
        await this.#setSessionKey({reset:args.resetSkey!==false});
        this.vars_.etoken = await this.#encryptString(resp.parts.token);
        this.vars_.serverPK = resp.parts.serverPublicKey;
        console.log('SW decrypting secret key');
        const keys = await this.decodeKeyBundle_(password, resp.parts.keyBundle);
        this.vars_.pk = keys.pk;
        if (keys.sk !== undefined) {
          this.vars_.esk = await this.#encrypt(keys.sk);
          this.vars_.keyIsBackedUp = true;
        } else {
          this.vars_.keyIsBackedUp = false;
        }
        console.log('SW logged in');
        this.vars_.email = email;
        this.vars_.userId = resp.parts.userId;
        this.vars_.isAdmin = resp.parts._admin === '1';
        this.vars_.enableNotifications = args.enableNotifications;

        console.log('SW save password hash');
        this.vars_.passwordSalt = (await so.randombytes(16)).toString('hex');
        this.vars_.password = await this.passwordForValidation_(this.vars_.passwordSalt, password);

        await this.saveVars_();
        if (this.vars_.keyIsBackedUp) {
          this.enableNotifications(clientId, this.vars_.enableNotifications);
        }
        return {
          account: email,
          isAdmin: this.vars_.isAdmin,
          needKey: this.vars_.esk === undefined,
        };
      });
  }

  async enableNotifications(clientId, onoff) {
    if (!self.registration.pushManager || !self.registration.pushManager.getSubscription) {
      return;
    }
    if (!onoff) {
      return self.registration.pushManager.getSubscription()
        .then(async sub => {
          if (sub === null) {
            return;
          }
          console.log('SW disable push notifications');
          const ep = sub.endpoint;
          sub.unsubscribe();
          return this.sendRequest_(clientId, 'v2x/config/push', {
            token: this.#token(),
            params: this.makeParams_({endpoint: ep}),
          })
          .then(() => false)
          .catch(() => false);
        });
    }
    const options = {
      'userVisibleOnly': true,
    };
    return self.registration.pushManager.getSubscription()
      .then(async sub => {
        if (sub !== null) {
          return sub;
        }
        return this.sendRequest_(clientId, 'v2x/config/push', {token: this.#token()})
          .then(resp => {
            if (resp.status !== 'ok') {
              throw resp.status;
            }
            options.applicationServerKey = resp.parts.applicationServerKey;
            return self.registration.pushManager.permissionState(options);
          })
          .then(state => {
            if (state !== 'granted') {
              throw 'permission state: ' + state;
            }
            console.log('SW enable push notifications');
            return self.registration.pushManager.subscribe(options);
          });
      })
      .then(async sub => {
        return this.sendRequest_(clientId, 'v2x/config/push', {
          token: this.#token(),
          params: this.makeParams_({
            endpoint: sub.endpoint,
            auth: self.base64RawUrlEncode(new Uint8Array(sub.getKey('auth'))),
            p256dh: self.base64RawUrlEncode(new Uint8Array(sub.getKey('p256dh'))),
          }),
        });
      })
      .then(resp => {
        if (resp.status !== 'ok') {
          throw resp.status;
        }
        return true;
      });
  }

  async checkPassword_(password) {
    const hash = await this.passwordForValidation_(this.vars_.passwordSalt, password);
    return hash === this.vars_.password;
  }

  async keyBackupEnabled(clientId) {
    return this.vars_.keyIsBackedUp === true;
  }

  async changeKeyBackup(clientId, password, doBackup) {
    if (!await this.checkPassword_(password)) {
      throw new Error('incorrect password');
    }
    console.log('SW reuploading keys');
    const params = {
      keyBundle: await this.makeKeyBundle_(password, this.vars_.pk, doBackup ? await this.#sk() : undefined),
    };
    return this.sendRequest_(clientId, 'v2/keys/reuploadKeys', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      this.vars_.keyIsBackedUp = doBackup;
      return this.saveVars_()
        .then(() => resp.status);
    });
  }

  async restoreSecretKey(clientId, backupPhrase) {
    return so.hex2bin(bip39.mnemonicToEntropy(backupPhrase.trim()))
      .then(sk => {
        return this.checkKey_(clientId, this.vars_.email, this.vars_.pk, sk)
          .then(res => {
            if (res !== true) {
              throw new Error('incorrect backup phrase');
            }
            this.enableNotifications(clientId, this.vars_.enableNotifications);
            return this.#encrypt(sk);
        });
      })
      .then(esk => {
        this.vars_.esk = esk;
      })
      .then(() => this.saveVars_());
  }

  async checkKey_(clientId, email, pk, sk) {
    return this.sendRequest_(clientId, 'v2/login/checkKey', {email})
      .then(async resp => {
        if (resp.status !== 'ok') {
          throw resp.status;
        }
        this.vars_.serverPK = resp.parts.serverPK;
        const challenge = self.base64DecodeToBytes(resp.parts.challenge);
        return so.box_seal_open(challenge, pk, sk);
      })
      .then(r => r.toString().startsWith('validkey_'));
  }

  async createAccount(clientId, args) {
    const {email, password, enableBackup, server} = args;
    console.log('SW createAccount', email, enableBackup);
    if (!SAMEORIGIN) {
      this.vars_.server = server || this.vars_.server;
    }
    const {sk, pk} = await so.box_keypair();
    console.log('SW encrypting secret key');
    const bundle = await this.makeKeyBundle_(password, pk, enableBackup ? sk : undefined);
    const salt = await so.randombytes(16);
    console.log('SW hashing password');
    const hashed = await this.passwordForLogin_(salt, password);
    const form = {
      email: email,
      password: hashed,
      salt: salt.toString('hex').toUpperCase(),
      keyBundle: bundle,
      isBackup: enableBackup ? '1' : '0',
    };
    console.log('SW creating account');
    return this.sendRequest_(clientId, 'v2/register/createAccount', form)
      .then(resp => {
        if (resp.status !== 'ok') {
          throw resp.status;
        }
        return this.#setSessionKey({reset:true});
      })
      .then(() => {
        args.resetSkey = false;
        return this.#encrypt(sk);
      })
      .then(esk => {
        this.vars_.esk = esk;
        this.vars_.pk = self.base64StdEncode(pk);
        return this.saveVars_();
      })
      .then(() => this.login(clientId, args))
      .then(v => {
        if (!enableBackup) {
          this.#sw.sendMessage(clientId, {type: 'info', msg: 'Your secret key is NOT backed up. You will need a backup phrase next time you login.'});
        }
        return v;
      });
  }

  async recoverAccount(clientId, args) {
    const {email, password, enableBackup, backupPhrase, server} = args;
    console.log('SW recoverAccount', enableBackup);
    if (!SAMEORIGIN) {
      this.vars_.server = server || this.vars_.server;
    }
    const sk = await so.hex2bin(bip39.mnemonicToEntropy(backupPhrase));
    const pk = await so.box_publickey_from_secretkey(sk);
    if (await this.checkKey_(clientId, email, pk, sk) !== true) {
      throw new Error('incorrect backup phrase');
    }
    await this.#setSessionKey({reset:true});
    args.resetSkey = false;
    this.vars_.pk = self.base64StdEncode(pk);
    this.vars_.esk = await this.#encrypt(sk);
    await this.saveVars_();
    console.log('SW encrypting secret key');
    const bundle = await this.makeKeyBundle_(password, pk, enableBackup ? sk : undefined);
    const salt = await so.randombytes(16);
    console.log('SW hashing password');
    const hashed = await this.passwordForLogin_(salt, password);
    const params = {
      newPassword: hashed,
      newSalt: salt.toString('hex').toUpperCase(),
      keyBundle: bundle,
      isBackup: enableBackup ? '1' : '0',
    };
    const form = {
      email: email,
      params: this.makeParams_(params),
    };
    console.log('SW recovering account');
    return this.sendRequest_(clientId, 'v2/login/recoverAccount', form)
      .then(resp => {
        if (resp.status !== 'ok') {
          throw resp.status;
        }
        return this.login(clientId, args);
      })
      .then(v => {
        if (!enableBackup) {
          this.#sw.sendMessage(clientId, {type: 'info', msg: _T('no-key-backup-warning')});
        }
        return v;
      });
  }

  async updateProfile(clientId, args) {
    console.log('SW updateProfile');
    if (!await this.checkPassword_(args.password)) {
      throw new Error('incorrect password');
    }
    const curr = await this.mfaStatus(clientId);
    const maybeSetMFA = async () => {
      if (args.setMFA !== curr.mfaEnabled || args.passKey !== curr.passKey) {
        const params = {
          requireMFA: args.setMFA ? '1' : '0',
          passKey: args.passKey ? '1' : '0',
        };
        const resp = await this.sendRequest_(clientId, 'v2x/mfa/enable', {
          token: this.#token(),
          params: this.makeParams_(params),
        });
        if (resp.status !== 'ok') {
          throw new Error('MFA update failed');
        }
      }
    };
    if (!args.setMFA) {
      await maybeSetMFA();
    }

    if (args.setOTP !== curr.otpEnabled) {
      const params = {
        key: ''+args.otpKey,
        code: ''+args.otpCode,
      };
      const resp = await this.sendRequest_(clientId, 'v2x/config/setOTP', {
        token: this.#token(),
        params: this.makeParams_(params),
      });
      if (resp.status !== 'ok') {
        throw new Error('OTP update failed');
      }
    }
    if (this.vars_.email !== args.email) {
      const resp = await this.sendRequest_(clientId, 'v2/login/changeEmail', {
        token: this.#token(),
        params: this.makeParams_({newEmail: args.email}),
      });
      if (resp.status !== 'ok') {
        throw new Error('email update failed');
      }
      this.vars_.email = args.email;
    }
    if (args.newPassword !== '') {
      const salt = await so.randombytes(16);
      const bundle = await this.makeKeyBundle_(args.newPassword, this.vars_.pk, this.vars_.keyIsBackedUp ? this.#sk() : undefined);
      const hashed = await this.passwordForLogin_(salt, args.newPassword);
      const params = {
        keyBundle: bundle,
        newPassword: hashed,
        newSalt: salt.toString('hex').toUpperCase(),
      };
      const resp = await this.sendRequest_(clientId, 'v2/login/changePass', {
        token: this.#token(),
        params: this.makeParams_(params),
      });
      if (resp.status !== 'ok') {
        throw new Error('password update failed');
      }
      this.vars_.loginSalt = salt.toString('hex').toUpperCase();
      this.vars_.etoken = await this.#encryptString(resp.parts.token);
      const salt2 = (await so.randombytes(16)).toString('hex');
      this.vars_.passwordSalt = salt2;
      this.vars_.password = await this.passwordForValidation_(salt2, args.newPassword);
    }
    if (args.keyChanges.length > 0) {
      const params = {
        updates: JSON.stringify(args.keyChanges),
      };
      const resp = await this.sendRequest_(clientId, 'v2x/config/webauthn/updateKeys', {
        token: this.#token(),
        params: this.makeParams_(params),
      });
      if (resp.status !== 'ok') {
        throw new Error('key update failed');
      }
    }
    if (args.setMFA) {
      await maybeSetMFA();
    }
    return this.saveVars_();
  }

  async listSecurityKeys(clientId) {
    console.log('SW listSecurityKeys');
    const resp = await this.sendRequest_(clientId, 'v2x/config/webauthn/keys', {
      token: this.#token(),
    });
    if (resp.status !== 'ok') {
      throw new Error('error');
    }
    return resp.parts.keys;
  }

  async addSecurityKey(clientId, args) {
    console.log('SW addSecurityKey');
    if (!args?.password || args.attestationObject && !await this.checkPassword_(args.password)) {
      throw new Error('incorrect password');
    }
    const params = {};
    if (args?.keyName) {
      params.keyName = args.keyName;
      params.discoverable = args.discoverable ? '1' : '0';
      params.clientDataJSON = self.base64RawUrlEncode(args.clientDataJSON);
      params.attestationObject = self.base64RawUrlEncode(args.attestationObject);
      params.transports = JSON.stringify(args.transports);
    } else {
      params.passKey = args.usePassKey ? '1' : '0';
    }
    const resp = await this.sendRequest_(clientId, 'v2x/config/webauthn/register', {
      token: this.#token(),
      params: this.makeParams_(params),
    });
    if (resp.status !== 'ok') {
      throw new Error('error');
    }
    return resp.parts.attestationOptions;
  }

  async mfaStatus(clientId) {
    console.log('SW mfaStatus');
    const resp = await this.sendRequest_(clientId, 'v2x/mfa/status', {token: this.#token()});
    if (resp.status !== 'ok') {
      throw new Error('error');
    }
    return resp.parts;
  }

  async mfaCheck(clientId, passKey) {
    console.log('SW mfaCheck');
    const resp = await this.sendRequest_(clientId, 'v2x/mfa/check', {
      token: this.#token(),
      params: this.makeParams_({
        passKey: passKey ? '1' : '0',
      }),
    });
    if (resp.status !== 'ok') {
      throw new Error('error');
    }
    return true;
  }

  async deleteAccount(clientId, password) {
    console.log('SW DELETE ACCOUNT!');
    const salt = await so.hex2bin(this.vars_.loginSalt);
    const params = {
      password: await this.passwordForLogin_(salt, password),
    };
    const resp = await this.sendRequest_(clientId, 'v2/login/deleteUser', {
      token: this.#token(),
      params: this.makeParams_(params),
    });
    if (resp.status !== 'ok') {
      throw resp.status;
    }
    return this.logout(clientId);
  }

  async makeKeyBundle_(password, pk, sk) {
    const out = [0x53, 0x50, 0x4B, 0x1]; // 'SPK', 1
    out.push(sk === undefined ? 0x2 : 0x0);
    if (typeof pk === 'string') {
      pk = self.base64DecodeToBytes(pk);
    }
    out.push(...pk);

    if (sk === undefined) {
      if (out.length !== 37) {
        throw new Error('created invalid bundle');
      }
    } else  {
      const salt = await so.randombytes(16);
      const key = await this.passwordForEncryption_(salt, password);
      const nonce = await so.randombytes(24);
      const esk = await so.secretbox(sk, nonce, key);
      out.push(...esk);
      out.push(...salt);
      out.push(...nonce);
      if (out.length !== 125) {
        throw new Error('created invalid bundle');
      }
    }
    return self.base64StdEncode(out);
  }

  async backupPhrase(clientId, password) {
    return this.checkPassword_(password)
    .then(async ok => {
      if (!ok) {
        throw new Error('incorrect password');
      }
      return bip39.entropyToMnemonic(await this.#sk());
    });
  }

  /*
   * Logout and clear all saved data.
   */
  async logout(clientId) {
    console.log('SW logout');
    return this.enableNotifications(clientId, false)
      .then(async () => this.sendRequest_(clientId, 'v2/login/logout', {token: this.#token()}))
      .then(() => console.log('SW logged out'))
      .catch(console.error)
      .finally(async () => {
        this.vars_ = {};
        this.resetDB_();
        await this.#store.clear();
        await this.deleteCache_();
        this.loadVars_();
        console.log('SW internal data cleared');
      });
  }

  /*
   * Send a getUpdates request, and process the response.
   */
  async getUpdates(clientId) {
    if (this.#state.gettingUpdates) {
      return;
    }
    this.#state.gettingUpdates = true;
    const data = {
      token: this.#token(),
      filesST: this.vars_.galleryTimeStamp,
      trashST: this.vars_.trashTimeStamp,
      albumsST: this.vars_.albumsTimeStamp,
      albumFilesST: this.vars_.albumFilesTimeStamp,
      cntST: this.vars_.contactsTimeStamp,
      delST: this.vars_.deletesTimeStamp,
    };
    return this.sendRequest_(clientId, 'v2/sync/getUpdates', data)
      .then(async resp => {
        //console.log('SW getUpdates', resp);
        console.log('SW getUpdates');
        if (resp.status !== 'ok') {
          throw resp.status;
        }
        // Quota
        this.vars_.spaceUsed = parseInt(resp.parts.spaceUsed);
        this.vars_.spaceQuota = parseInt(resp.parts.spaceQuota);

        /* contacts */
        for (let c of resp.parts.contacts) {
          this.db_.contacts[''+c.userId] = c;
          if (c.dateModified > this.vars_.contactsTimeStamp) {
            this.vars_.contactsTimeStamp = c.dateModified;
          }
        }

        /*  albums */
        const pk = this.vars_.pk;
        const sk = this.#sk();
        for (let a of resp.parts.albums) {
          try {
            const apk = self.base64DecodeToBytes(a.publicKey);
            const ask = await so.box_seal_open(self.base64DecodeToBytes(a.encPrivateKey), pk, sk);

            const md = await so.box_seal_open(self.base64DecodeToBytes(a.metadata), apk, ask);
            const bytes = new Uint8Array(md);
            if (bytes[0] !== 1) {
              throw new Error('unexpected metadata version');
            }
            let size = 0;
            for (let i = 1; i < 5; i++) {
              size = (size << 8) + bytes[i];
            }
            if (5+size > bytes.length) {
              throw new Error('invalid album metadata');
            }
            const name = self.bytesToString(md.slice(5, 5+size));
            let members = [];
            if (typeof a.members === 'string') {
              members = a.members.split(',').filter(m => m !== '');
            }
            const obj = {
              'albumId': a.albumId,
              'pk': self.base64StdEncode(apk),
              'encSK': a.encPrivateKey,
              'encName': await this.#encryptString(name),
              'cover': a.cover,
              'members': members,
              'isOwner': a.isOwner === 1,
              'isShared': a.isShared === 1,
              'isOffline': a.albumId in this.db_.albums ? this.db_.albums[a.albumId].isOffline : false,
              'permissions': a.permissions,
              'dateModified': a.dateModified,
              'dateCreated': a.dateCreated,
            };
            if (a.dateModified > this.vars_.albumsTimeStamp) {
              this.vars_.albumsTimeStamp = a.dateModified;
            }
            this.db_.albums[a.albumId] = obj;
          } catch (error) {
            console.error('SW getUpdates', a, error);
          }
        }

        let changed = {};

        /* gallery files */
        for (let f of resp.parts.files) {
          try {
            changed.gallery = true;
            const obj = await this.convertFileUpdate_(f, 0);
            this.insertFile_('gallery', f.file, obj);
            if (f.dateModified > this.vars_.galleryTimeStamp) {
              this.vars_.galleryTimeStamp = f.dateModified;
            }
          } catch (error) {
            console.error('SW getUpdates', f, error);
          }
        }

        /* trash files */
        for (let f of resp.parts.trash) {
          try {
            changed.trash = true;
            const obj = await this.convertFileUpdate_(f, 1);
            this.insertFile_('trash', f.file, obj);
            if (f.dateModified > this.vars_.trashTimeStamp) {
              this.vars_.trashTimeStamp = f.dateModified;
            }
          } catch (error) {
            console.error('SW getUpdates', f, error);
          }
        }

        /* album files */
        for (let f of resp.parts.albumFiles) {
          try {
            changed[f.albumId] = true;
            let obj = await this.convertFileUpdate_(f, 2);
            obj.albumId = f.albumId;
            this.insertFile_(f.albumId, f.file, obj);
            if (f.dateModified > this.vars_.albumFilesTimeStamp) {
              this.vars_.albumFilesTimeStamp = f.dateModified;
            }
          } catch (error) {
            console.error('SW getUpdates', f, error);
          }
        }

        /* deletes */
        for (let d of resp.parts.deletes) {
          try {
            let f;
            switch (d.type) {
              case 1: // A file is removed from the gallery.
                f = await this.getFile_('gallery', d.file);
                if (f?.dateModified < d.date) {
                  this.deleteFile_('gallery', d.file);
                  changed.gallery = true;
                }
                break;
              case 2: // A file is removed from the trash (and moved somewhere else).
              case 3: // A file is deleted from the trash.
                f = await this.getFile_('trash', d.file);
                if (f?.dateModified < d.date) {
                  this.deleteFile_('trash', d.file);
                  changed.trash = true;
                }
                break;
              case 4: // An album is deleted.
                if (this.db_.albums[d.albumId]?.dateModified < d.date) {
                  delete this.db_.albums[d.albumId];
                  changed[d.albumId] = true;
                }
                break;
              case 5: // A file is removed from an album.
                f = await this.getFile_(d.albumId, d.file);
                if (f?.dateModified < d.date) {
                  this.deleteFile_(d.albumId, d.file);
                  changed[d.albumId] = true;
                }
                break;
              case 6: // A contact is removed.
                let id = ''+d.file;
                if (this.db_.contacts[id]?.dateModified < d.date) {
                  delete this.db_.contacts[id];
                }
                break;
              default:
                console.error('SW Unexpected delete type', d);
                break;
            }
            if (d.date > this.vars_.deletesTimeStamp) {
              this.vars_.deletesTimeStamp = d.date;
            }
          } catch (error) {
            console.error('SW getUpdates', d, error);
          }
        }
        const p = [
          this.saveVars_(),
          this.#store.set('albums', this.db_.albums),
          this.#store.set('contacts', this.db_.contacts),
          Promise.all(Object.keys(changed).map(collection => this.indexCollection_(collection))),
        ];
        return Promise.all(p)
          .then(v => {
            this.checkCachedFiles_();
            return v;
          });
      })
      .finally(() => {
        this.#state.gettingUpdates = false;
      });
  }

  async emptyTrash(clientId) {
    let params = {
      time: ''+Date.now(),
    };
    return this.sendRequest_(clientId, 'v2/sync/emptyTrash', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.status;
    });
  }

  async deleteFiles(clientId, files) {
    let params = {
      count: ''+files.length,
    };
    for (let i = 0; i < files.length; i++) {
      params[`filename${i}`] = files[i];
    }
    return this.sendRequest_(clientId, 'v2/sync/delete', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.status;
    });
  }

  async changeCover(clientId, albumId, cover) {
    let params = {
      albumId: albumId,
      cover: cover,
    };
    return this.sendRequest_(clientId, 'v2/sync/changeAlbumCover', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.status;
    });
  }

  async moveFiles(clientId, from, to, files, isMove) {
    const fromAlbumId = from === 'gallery' || from === 'trash' ? '' : from;
    const toAlbumId = to === 'gallery' || to === 'trash' ? '' : to;
    const headers = [];
    if (fromAlbumId !== '' || toAlbumId !== '') {
      // Need new headers
      const pk = fromAlbumId === '' ? this.vars_.pk : this.db_.albums[fromAlbumId].pk;
      const sk = fromAlbumId === '' ? this.#sk() : this.decryptAlbumSK_(fromAlbumId);
      const pk2 = toAlbumId === '' ? this.vars_.pk : this.db_.albums[toAlbumId].pk;

      for (let i = 0; i < files.length; i++) {
        let f = await this.getFile_(from, files[i]);
        let hdrs = f.origHeaders.split('*');
        hdrs[0] = await this.reEncryptHeader_(hdrs[0], pk, sk, pk2);
        hdrs[1] = await this.reEncryptHeader_(hdrs[1], pk, sk, pk2);
        headers[i] = hdrs.join('*');
      }
    }
    let params = {
      setFrom: from === 'gallery' ? '0' : from === 'trash' ? '1' : '2',
      setTo: to === 'gallery' ? '0' : to === 'trash' ? '1' : '2',
      albumIdFrom: fromAlbumId,
      albumIdTo: toAlbumId,
      isMoving: isMove ? '1' : '0',
      count: ''+files.length,
    };
    for (let i = 0; i < files.length; i++) {
      params[`filename${i}`] = files[i];
      if (headers.length > 0) {
        params[`headers${i}`] = headers[i];
      }
    }
    return this.sendRequest_(clientId, 'v2/sync/moveFile', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.status;
    });
  }

  async decryptAlbumSK_(albumId) {
    if (!(albumId in this.db_.albums)) {
      throw new Error('invalid albumId');
    }
    const a = this.db_.albums[albumId];
    return so.box_seal_open(self.base64DecodeToBytes(a.encSK), this.vars_.pk, this.#sk());
  }

  async insertFile_(collection, file, obj) {
    return this.#store.set(`files/${collection}/${file}`, obj);
  }

  async deleteFile_(collection, file) {
    return this.#store.del(`files/${collection}/${file}`);
  }

  async getFile_(collection, file) {
    return this.#store.get(`files/${collection}/${file}`);
  }

  async deletePrefix_(prefix) {
    return this.#store.keys()
      .then(keys => keys.filter(k => k.startsWith(prefix)))
      .then(keys => Promise.all(keys.map(k => this.#store.del(k))));
  }

  /*
   */
  async convertFileUpdate_(up, set) {
    const encHeaders = up.headers.split('*');
    return {
      'file': up.file,
      'set': set,
      'headers': [
        await this.decryptHeader_(encHeaders[0], up.albumId),
        await this.decryptHeader_(encHeaders[1], up.albumId),
      ],
      'origHeaders': up.headers,
      'dateCreated': up.dateCreated,
      'dateModified': up.dateModified,
    };
  }

  async indexCollection_(collection) {
    await this.deletePrefix_(`index/${collection}`);

    const prefix = `files/${collection}/`;
    const keys = (await this.#store.keys()).filter(k => k.startsWith(prefix));
    let out = [];
    for (let k of keys) {
      const file = k.substring(prefix.length);
      const f = await this.getFile_(collection, file);
      if (!f) {
        continue;
      }
      const obj = {
        'collection': collection,
        'file': f.file,
        'isImage': f.headers[0].fileType === 2,
        'isVideo': f.headers[0].fileType === 3,
        'fileName': await this.#decryptString(f.headers[0].encFileName),
        'dateCreated': f.dateCreated,
        'dateModified': f.dateModified,
        'size': f.headers[0].dataSize,
      };
      obj.contentType = this.contentType_(obj.fileName.replace(/^.*(\.[^.]+)$/, '$1').toLowerCase());
      if (obj.isVideo) {
        obj.duration =  f.headers[0].duration;
      }
      obj.url = await this.getDecryptUrl_(f, false);
      obj.thumbUrl = await this.getDecryptUrl_(f, true);
      out.push(obj);
    }
    out.sort((a, b) => b.dateCreated - a.dateCreated);
    let p = [];
    for (let i = 0; i < out.length; i+=100) {
      let n = ('000000' + i).slice(-6);
      let obj = {
        start: i,
        total: out.length,
        files: out.slice(i, Math.min(i+100, out.length)),
      };
      p.push(this.#store.set(`index/${collection}/${n}`, obj));
    }
    return Promise.all(p);
  }

  async getContact(clientId, email) {
    const params = {
      email: email,
    };
    return this.sendRequest_(clientId, 'v2/sync/getContact', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(async resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      const c = resp.parts.contact;
      this.db_.contacts[''+c.userId] = c;
      await this.#store.set('contacts', this.db_.contacts);
      c.userId = ''+c.userId;
      return c;
    });
  }

  async getContacts(clientId) {
    const contacts = Object.values(this.db_.contacts).map(c => {
      c.userId = ''+c.userId;
      return c;
    });
    contacts.sort((a, b) => {
      if (a.email < b.email) return -1;
      if (a.email > b.email) return 1;
      return 0;
    });
    return contacts;
  }

  /*
   */
  async getFiles(clientId, collection, offset = 0) {
    const n = ('000000' + offset).slice(-6);
    return this.#store.get(`index/${collection}/${n}`);
  }

  /*
   */
  async getCollections(clientId) {
    return new Promise(async resolve => {
      let {url} = await this.getCover(clientId, 'gallery');
      let out = [
        {
          'collection': 'gallery',
          'name': 'gallery',
          'cover': url,
          'isOwner': true,
          'isShared': false,
          'isOffline': this.vars_.galleryOffline === true,
          'canAdd': true,
          'canCopy': true,
        },
        {
          'collection': 'trash',
          'name': 'trash',
          'cover': null,
          'isOwner': true,
          'isShared': false,
          'isOffline': false,
        },
      ];

      let albums = [];
      for (let n in this.db_.albums) {
        if (!this.db_.albums.hasOwnProperty(n)) {
          continue;
        }
        const a = this.db_.albums[n];
        let {url} = await this.getCover(clientId, a.albumId);
        albums.push({
          'collection': a.albumId,
          'name': await this.#decryptString(a.encName),
          'cover': url,
          'members': a.members.map(m => {
            if (m === this.vars_.userId) return {userId: m, email: this.vars_.email, myself: true};
            if (m in this.db_.contacts) return {userId: m, email: this.db_.contacts[m].email};
            return {userId: m, email: '#'+m};
          }).sort(),
          'isOwner': a.isOwner,
          'isShared': a.isShared,
          'isOffline': a.isOffline,
          'canAdd': a.permissions?.match(/^11../) !== null,
          'canShare': a.permissions?.match(/^1.1./) !== null,
          'canCopy': a.permissions?.match(/^1..1/) !== null,
        });
      }
      albums.sort((a, b) => {
        if (a.name < b.name) return -1;
        if (a.name > b.name) return 1;
        return 0;
      });
      out.push(...albums);
      resolve(out);
    });
  }

  async getCover(clientId, collection, opt_code) {
    let code = opt_code;
    if (code === undefined && collection in this.db_.albums) {
      code = this.db_.albums[collection].cover;
    }
    if (code === '__b__') {
      return {url:null, code:code};
    }
    let file = code || '';
    if (file === '') {
      const idx = await this.#store.get(`index/${collection}/000000`);
      if (idx?.files?.length > 0) {
        file = idx.files[0].file;
      }
    }
    if (file === '') {
      return {url:null, code:code};
    }
    const f = await this.getFile_(collection, file);
    if (!f) {
      return {url:null, code:code};
    }
    const url = await this.getDecryptUrl_(f, true);
    return {url:url, code:code};
  }

  async getDecryptUrl_(f, isThumb) {
    if (!f) {
      return null;
    }
    let collection = f.albumId;
    if (f.set === 0) collection = 'gallery';
    else if (f.set === 1) collection = 'trash';
    const fn = await this.#decryptString(f.headers[0].encFileName);
    let url = `${this.#options.pathPrefix}${this.vars_.decryptPath}/${fn}?collection=${collection}&file=${f.file}`;
    if (isThumb) {
      url += '&isThumb=1';
    }
    return url;
  }

  async getContentUrl_(f) {
    const file = await this.getFile_(f.collection, f.file);
    return this.sendRequest_(null, 'v2/sync/getUrl', {
      token: this.#token(),
      file: file.file,
      set: file.set,
      thumb: f.isThumb ? '1' : '0',
    })
    .then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.parts.url;
    });
  }

  /*
   */
  async makeParams_(obj) {
    const nonce = await so.randombytes(24);
    const m = await so.box(JSON.stringify(obj), nonce, this.#sk(), this.vars_.serverPK);
    const out = new Uint8Array(nonce.byteLength + m.byteLength);
    out.set(nonce);
    out.set(m, nonce.byteLength);
    return self.base64StdEncode(out);
  }

  /*
   */
  async decodeKeyBundle_(password, bundle) {
    const bytes = self.base64DecodeToBytes(bundle);
    if (bytes.length !== 37 && bytes.length !== 125) {
      throw new Error('bundle is too short');
    }
    // Check header.
    if (String.fromCharCode(bytes[0], bytes[1], bytes[2]) !== 'SPK') {
      throw new Error('invalid bundle header');
    }
    // Check version
    if (bytes[3] !== 1) {
      throw new Error('invalid bundle version');
    }
    // Check type
    if (bytes[4] !== 0 && bytes[4] !== 2) {
      throw new Error('unexpected bundle type');
    }
    const pk = self.base64StdEncode(new Uint8Array(bytes.slice(5, 37)));

    if (bytes[4] !== 0) {
      return {pk}; // secret key not in bundle.
    }
    const esk = new Uint8Array(bytes.slice(37, -40));
    const salt = new Uint8Array(bytes.slice(-40, -24));
    const nonce = new Uint8Array(bytes.slice(-24));

    const key = await this.passwordForEncryption_(salt, password);
    const sk = await so.secretbox_open(esk, nonce, key);
    return {pk, sk};
  }

  /*
   */
  async decryptHeader_(encHeader, albumId) {
    const bytes = self.base64DecodeToBytes(encHeader);
    if (String.fromCharCode(bytes[0], bytes[1]) !== 'SP') {
      throw new Error('invalid header');
    }
    if (bytes[2] !== 1) {
      throw new Error('unexpected header version');
    }
    //const fileId = bytes.slice(3, 35);
    let size = 0;
    for (let i = 35; i < 39; i++) {
      size = (size << 8) + bytes[i];
    }
    let pk;
    let sk;
    if (albumId === '') {
      pk = this.vars_.pk;
      sk = this.#sk();
    } else {
      pk = this.db_.albums[albumId].pk;
      sk = this.decryptAlbumSK_(albumId);
    }
    const hdr = await so.box_seal_open(bytes.slice(39, 39+size), pk, sk);
    //const version = hdr[0];
    const chunkSize = hdr[1]<<2 | hdr[2]<<16 | hdr[3]<<8 | hdr[4];
    if (chunkSize < 0 || chunkSize > 10485760) {
      throw new Error('invalid chunk size');
    }
    const dataSize = hdr[5]<<56 | hdr[6]<<48 | hdr[7]<<40 | hdr[8]<<32 | hdr[9]<<24 | hdr[10]<<16 | hdr[11]<<8 | hdr[12];
    if (dataSize < 0) {
      throw new Error('invalid data size');
    }
    const symKey = new Uint8Array(hdr.slice(13, 45));
    const fileType = hdr[45];
    const fnSize = hdr[46]<<24 | hdr[47]<<16 | hdr[48]<<8 | hdr[49];
    if (fnSize < 0 || fnSize+50 > hdr.length) {
      throw new Error('invalid filename size');
    }
    const fn = self.bytesToString(hdr.slice(50, 50+fnSize));
    const dur = hdr[50+fnSize]<<24 | hdr[51+fnSize]<<16 | hdr[52+fnSize]<<8 | hdr[53+fnSize];
    if (dur < 0) {
      throw new Error('invalid duration');
    }

    const header = {
        chunkSize: chunkSize,
        dataSize: dataSize,
        encKey: await this.#encrypt(symKey),
        fileType: fileType,
        encFileName: await this.#encryptString(fn.replace(/^ */, '')),
        duration: dur,
        headerSize: bytes.length,
    };
    return header;
  }

  async reEncryptHeader_(encHeader, pk, sk, toPK) {
    const bytes = self.base64DecodeToBytes(encHeader);
    if (String.fromCharCode(bytes[0], bytes[1]) !== 'SP') {
      throw new Error('invalid header');
    }
    if (bytes[2] !== 1) {
      throw new Error('unexpected header version');
    }
    let size = 0;
    for (let i = 35; i < 39; i++) {
      size = (size << 8) + bytes[i];
    }
    const hdr = await so.box_seal_open(bytes.slice(39, 39+size), pk, sk);
    const newEncHeader = await so.box_seal(hdr, toPK);
    if (newEncHeader.byteLength !== size) {
      console.error(`SW reEncryptHeader_ ${newEncHeader.byteLength} !== ${size}`);
      throw new Error('Re-encrypted header has unexpected size');
    }
    bytes.set(newEncHeader, 39);
    return self.base64RawUrlEncode(bytes);
  }

  async makeMetadata_(pk, name) {
    const encoded = self.bytesFromString(name);
    const md = [ 1 ];
    md.push(...self.bigEndian(encoded.byteLength, 4));
    md.push(...encoded);
    const enc = await so.box_seal(md, pk);
    return self.base64StdEncode(enc);
  }

  async renameCollection(clientId, collection, name) {
    const pk = this.db_.albums[collection].pk;
    const params = {
      albumId: collection,
      metadata: await this.makeMetadata_(pk, name),
    };
    return this.sendRequest_(clientId, 'v2/sync/renameAlbum', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.status;
    });
  }

  async setCollectionOffline(clientId, collection, isOffline) {
    if (collection === 'gallery') {
      this.vars_.galleryOffline = isOffline;
    } else if (collection in this.db_.albums) {
      this.db_.albums[collection].isOffline = isOffline;
    }
    if (this.#state.checkCachedFileState) {
      this.#state.checkCachedFileState.err = _T('canceled');
      this.#state.checkCachedFileState.cancel = true;
      await this.#state.checkCachedFilesRunning;
    }
    return this.saveVars_()
      .then(r => {
        this.checkCachedFiles_();
        return r;
      });
  }

  makePermissions(perms) {
    return '1' + (perms.canAdd ? '1' : '0') + (perms.canShare ? '1' : '0') + (perms.canCopy ? '1' : '0');
  }

  async shareCollection(clientId, collection, perms, members) {
    members.push(this.vars_.userId);
    const album = {
      albumId: collection,
      isShared: '1',
      permissions: this.makePermissions(perms),
      members: members.join(','),
    };
    const sk = await this.decryptAlbumSK_(collection);
    const sharingKeys = {};
    for (let i = 0; i < members.length; i++) {
      if (members[i] === this.vars_.userId) {
        continue;
      }
      const pk = self.base64DecodeToBytes(this.db_.contacts[''+members[i]].publicKey);
      const enc = await so.box_seal(sk, pk);
      sharingKeys[''+members[i]] = self.base64StdEncode(enc);
    }
    const params = {
      album: JSON.stringify(album),
      sharingKeys: JSON.stringify(sharingKeys),
    };
    return this.sendRequest_(clientId, 'v2/sync/share', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.status;
    });
  }

  async unshareCollection(clientId, collection) {
    const params = {
      albumId: collection,
    };
    return this.sendRequest_(clientId, 'v2/sync/unshareAlbum', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.status;
    });
  }

  async removeMembers(clientId, collection, members) {
    const album = {
      albumId: collection,
    };
    let p = [];
    for (let i = 0; i < members.length; i++) {
      const params = {
        album: JSON.stringify(album),
        memberUserId: members[i],
      };
      p.push(this.sendRequest_(clientId, 'v2/sync/removeAlbumMember', {
        token: this.#token(),
        params: this.makeParams_(params),
      }).then(resp => {
        if (resp.status !== 'ok') {
          throw resp.status;
        }
        return resp.status;
      }));
    }
    return Promise.all(p);
  }

  async updatePermissions(clientId, collection, perms) {
    const album = {
      albumId: collection,
      permissions: this.makePermissions(perms),
    };
    const params = {
      album: JSON.stringify(album),
    };
    return this.sendRequest_(clientId, 'v2/sync/editPerms', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.status;
    });
  }

  async leaveCollection(clientId, collection) {
    const params = {
      albumId: collection,
    };
    return this.sendRequest_(clientId, 'v2/sync/leaveAlbum', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.status;
    });
  }

  async createCollection(clientId, name) {
    const {sk, pk} = await so.box_keypair();
    const encSK = await so.box_seal(sk, this.vars_.pk);

    const params = {
      albumId: self.base64RawUrlEncode(await so.randombytes(32)),
      dateCreated: ''+Date.now(),
      dateModified: ''+Date.now(),
      metadata: await this.makeMetadata_(pk, name),
      encPrivateKey: self.base64StdEncode(encSK),
      publicKey: self.base64StdEncode(pk),
    };
    return this.sendRequest_(clientId, 'v2/sync/addAlbum', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(async resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      const obj = {
        'albumId': params.albumId,
        'pk': self.base64StdEncode(pk),
        'encSK': params.encPrivateKey,
        'encName': await this.#encryptString(name),
        'cover': '',
        'members': '',
        'isOwner': true,
        'isShared': false,
        'permissions': '',
        'dateModified': params.dateModified,
        'dateCreated': params.dateCreated,
      };
      this.db_.albums[obj.albumId] = obj;
      await this.#store.set('albums', this.db_.albums);
      return params.albumId;
    });
  }

  async deleteCollection(clientId, collection) {
    const prefix = `files/${collection}/`;
    const files = (await this.#store.keys()).filter(k => k.startsWith(prefix)).map(k => k.substring(prefix.length));
    if (files.length > 0) {
      await this.moveFiles(clientId, collection, 'trash', files, true);
    }

    const params = {
      albumId: collection,
    };
    return this.sendRequest_(clientId, 'v2/sync/deleteAlbum', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return resp.status;
    });
  }

  async generateOTP(clientId) {
    return this.sendRequest_(clientId, 'v2x/config/generateOTP', {
      token: this.#token(),
    }).then(resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      return {key: resp.parts.key, img: resp.parts.img};
    });
  }

  async adminUsers(clientId, changes) {
    const params = {};
    if (changes !== undefined) {
      params.changes = JSON.stringify(changes);
    }
    return this.sendRequest_(clientId, 'v2x/admin/users', {
      token: this.#token(),
      params: this.makeParams_(params),
    }).then(async resp => {
      if (resp.status !== 'ok') {
        throw resp.status;
      }
      const enc = self.base64DecodeToBytes(resp.parts.users);
      return so.box_seal_open(enc, this.vars_.pk, this.#sk());
    })
    .then(j => JSON.parse(j));
  }

  async onpush(data) {
    if (!data) {
      return;
    }
    const enc = self.base64DecodeToBytes(data);
    const m = await so.box_seal_open(enc, this.vars_.pk, this.#sk());
    const js = JSON.parse(self.bytesToString(m));
    console.log('SW onpush:', js);
    let album;
    switch (js.type) {
      case 1: // New user registration
        await this.#sw.showNotif(_T('new-user-title', js.target), {
          tag: `new-user:${js.target}:${js.id}`,
        });
        break;
      case 2: // New content in album
        await this.getUpdates('');
        album = this.db_.albums[js.target];
        if (album) {
          const name = await this.#decryptString(album.encName);
          await this.#sw.showNotif(name, {
            tag: `new-content:${js.target}`,
            body: _T('new-content-body'),
          });
        }
        break;
      case 3: // New member in album
        await this.getUpdates('');
        album = this.db_.albums[js.target];
        const name = album ? await this.#decryptString(album.encName) : _T('collection');
        const members = js.data.members;
        if (members && members.includes(this.vars_.userId)) {
          await this.#sw.showNotif(name, {
            tag: `new-collection:${js.target}`,
            body: _T('new-collection-body'),
          });
        } else if (members && members.length) {
          await this.#sw.showNotif(name, {
            tag: `new-member:${js.target}`,
            body: _T('new-members-body'),
          });
        }
        break;
      case 4: // Test notification
        await this.#sw.showNotif(_T('push-notifications-title'), {
          tag: `test-notification:${js.id}`,
          body: _T('push-notifications-body'),
        });
        break;
      case 5: // Remote MFA
        if (js.data.expires > Date.now()) {
          let tag = `remote-mfa:${js.data.session}`;
          await this.#sw.showNotif(_T('remote-mfa-title'), {
            tag: tag,
            body: _T('remote-mfa-body'),
            actions: [
              {
                action: 'approve',
                title: _T('approve'),
              },
              {
                action: 'deny',
                title: _T('deny'),
              },
            ],
            requireInteraction: true,
            vibrate: [100,50,100],
          });
          setTimeout(() => {
            self.registration.getNotifications({tag}).then(nn => nn.map(n => n.close()));
          }, 30000);
        } else {
          console.log('SW Remote MFA expired');
        }
        break;
    }
  }

  async approveRemoteMFA(session) {
    return this.sendRequest_('', 'v2x/mfa/approve', {
      token: this.#token(),
      params: this.makeParams_({session}),
    });
  }

  /*
   */
  async sendRequest_(clientId, endpoint, data) {
    //console.log('SW', this.vars_.server + endpoint);
    let enc = [];
    for (let n in data) {
      if (!data.hasOwnProperty(n)) {
        continue;
      }
      enc.push(encodeURIComponent(n) + '=' + encodeURIComponent(await Promise.resolve(data[n])));
    }
    return fetch(this.vars_.server + endpoint, {
      method: 'POST',
      mode: SAMEORIGIN ? 'same-origin' : 'cors',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'X-c2FmZQ-capabilities': this.#capabilities.join(','),
      },
      redirect: 'error',
      referrerPolicy: 'no-referrer',
      body: enc.join('&'),
    })
    .catch(err => {
      if (err instanceof TypeError) {
        throw new Error(_T('network-error'));
      }
      throw err;
    })
    .then(resp => {
      if (!resp.ok) {
        throw new Error(`${resp.status} ${resp.statusText}`);
      }
      return resp.json();
    })
    .then(resp => {
      if (resp.infos.length > 0) {
        this.#sw.sendMessage(clientId, {type: 'info', msg: resp.infos.join('\n')});
      }
      if (resp.errors.length > 0) {
        this.#sw.sendMessage(clientId, {type: 'error', msg: resp.errors.join('\n')});
      }
      if (!data.mfa && resp.status === 'nok' && resp.parts.mfa) {
        console.log(`SW got request for MFA on ${endpoint}`);
        return this.#sw.sendRPC(clientId, 'getMFA', resp.parts.mfa)
          .then(res => {
            data.mfa = JSON.stringify(res || {});
            return this.sendRequest_(clientId, endpoint, data);
          });
      }
      if (resp.parts && resp.parts.logout === "1") {
        this.vars_ = {};
        this.resetDB_();
        this.#store.clear();
        this.#sw.sendLoggedOut();
      }
      return resp;
    });
  }

  async setCachePreference(clientId, v) {
    console.log('SW setCachePreference');
    if (!['no-store','private','encrypted'].includes(v.mode)) {
      throw new Error('invalid cache option');
    }
    this.vars_.cachePref = v.mode;
    this.vars_.prefetchThumbnails = v.allthumbs === true;
    this.vars_.downloadOnMobile = v.mobile === true;
    v.maxSize = parseInt(v.maxSize);
    this.vars_.maxCacheSize = Math.max(v.maxSize, 1) * 1024 * 1024;
    this.cm_.setMaxSize(this.vars_.maxCacheSize);
    if (this.#state.checkCachedFileState) {
      this.#state.checkCachedFileState.err = _T('canceled');
      this.#state.checkCachedFileState.cancel = true;
      await this.#state.checkCachedFilesRunning;
    }
    if (v.mode !== 'encrypted') {
      await this.deleteCache_();
    }
    return this.saveVars_()
      .then(r => {
        this.checkCachedFiles_();
        return r;
      });
  }

  async cachePreference() {
    return {
      mode: this.vars_.cachePref,
      allthumbs: this.vars_.prefetchThumbnails,
      mobile: this.vars_.downloadOnMobilee,
      maxSize: Math.floor(this.vars_.maxCacheSize / 1024 / 1024),
      usage: Math.floor(this.cm_.totalSize() / 1024 / 1024),
    };
  }

  async ping() {
    console.log('SW ping');
    return true;
  }

  async deleteCache_() {
    await self.caches.delete('local');
    this.cache_ = await self.caches.open('local');
    await this.cm_.delete();
    this.cm_ = new CacheManager(this.#store, this.cache_, this.vars_.maxCacheSize);
  }

  onNetworkChange(event) {
    try {
      this.saveBandwidth_();
      this.checkCachedFiles_();
    } catch (e) {}
  }

  async checkCachedFiles_() {
    if (this.#state.checkCachedFilesRunning) {
      return this.#state.checkCachedFilesRunning;
    }
    this.#state.checkCachedFilesRunning = new Promise(async (resolve, reject) => {
      this.#state.checkCachedFileState = {};
      try {
        await this.checkCachedFilesNow_().catch(err => {
          console.log('SW checking cached files', err);
          this.#sw.sendDownloadProgress({
            count: 0,
            total: 0,
            done: true,
            err: this.#state.checkCachedFileState.err || err.message,
          });
        });
      } finally {
        this.#state.checkCachedFileState = null;
      }
      this.cm_.flush().then(() => this.cm_.selfCheck()).then(resolve, reject);
    })
    .finally(() => {
      this.#state.checkCachedFilesRunning = null;
    });
    return this.#state.checkCachedFilesRunning;
  }

  async checkCachedFilesNow_() {
    if (this.vars_.cachePref !== 'encrypted') {
      return 0;
    }
    const oc = new Set();
    if (this.vars_.galleryOffline) {
      oc.add('gallery');
    }
    for (let n in this.db_.albums) {
      if (!this.db_.albums.hasOwnProperty(n)) {
        continue;
      }
      if (this.db_.albums[n].isOffline) {
        oc.add(n);
      }
    }
    const allFiles = new Set();
    const wantfs = new Map();
    const wanttn = new Map();

    console.time('SW cache stick/unstick');
    (await this.#store.keys()).filter(k => k.startsWith('files/')).map(k => {
      const p = k.lastIndexOf('/');
      const c = k.substring(6, p);
      const f = k.substring(p+1);
      allFiles.add(f);
      if (this.vars_.prefetchThumbnails || oc.has(c)) {
        wanttn.set(f, c);
      }
      if (oc.has(c)) {
        wantfs.set(f, c);
      }
    });

    const p = [];
    (await this.cm_.keys()).forEach(key => {
      const isThumb = key.startsWith('tn/');
      const name = key.substring(key.indexOf('/')+1);
      const stick = isThumb ? wanttn.delete(name) : wantfs.delete(name);
      if (allFiles.has(name)) {
        p.push(this.cm_.update(key, stick ? {stick:true} : {unstick:true}));
      } else {
        p.push(this.cm_.update(key, {delete:true}));
      }
    });
    await Promise.all(p);
    console.timeEnd('SW cache stick/unstick');

    const queue = [];
    for (const [file, collection] of wanttn) {
      queue.push({f:file, c:collection, t:true});
    }
    for (const [file, collection] of wantfs) {
      queue.push({f:file, c:collection, t:false});
    }
    const total = queue.length;
    let count = 0;
    const id = setInterval(() => {
      this.#sw.sendDownloadProgress({count:count,total:total,done:false});
    }, 500);
    try {
      for (const it of queue) {
        if (++count % 10 === 0) {
          console.log(`SW downloading files: ${count}/${total}`);
        }
        await this.fetchCachedFile_(it.f, it.c, it.t);
      }
    } finally {
      clearInterval(id);
    }
    if (count > 0) {
      console.log(`SW downloaded files: ${count}/${total}`);
      this.#sw.sendDownloadProgress({count:count,total:total,done:true});
    }
    return count;
  }

  async fetchCachedFile_(name, collection, isThumb) {
    if (this.vars_.cachePref !== 'encrypted') {
      throw new Error(_T('caching-disabled'));
    }
    if (this.#state.checkCachedFileState.cancel) {
      throw new Error(_T('canceled'));
    }
    this.saveBandwidth_();
    const cmName = (isThumb?'tn':'fs') + '/' + name;
    if (await this.cm_.exists(cmName)) {
      return this.cm_.update(cmName, {stick:true});
    }
    const file = await this.getFile_(collection, name);
    if (!file) {
      return;
    }
    if (!this.cm_.canAdd()) {
      throw new Error(_T('cache-full'));
    }

    const headers = file.headers[isThumb?1:0];
    const startOffset = headers.headerSize;
    const symKey = this.#decrypt(headers.encKey);
    const chunkSize = headers.chunkSize;
    const encChunkSize = chunkSize+so.XCHACHA20POLY1305_OVERHEAD;
    const fileSize = headers.dataSize;
    const strategy = new ByteLengthQueuingStrategy({
      highWaterMark: 5*encChunkSize,
    });

    return this.getContentUrl_({file:name,collection:collection,isThumb:isThumb})
    .then(url => fetch(url, {
      method: 'GET',
      headers: {
        range: `bytes=${startOffset}-`,
      },
      mode: SAMEORIGIN ? 'same-origin' : 'cors',
      credentials: 'omit',
      redirect: 'error',
      referrerPolicy: 'no-referrer',
    }))
    .then(resp => {
      if (!resp.ok) {
        throw new Error(`Status: ${resp.status}`);
      }
      const rs = new ReadableStream(new CacheStream(resp.body, this.#state.checkCachedFileState));
      return this.cm_.put(cmName, new Response(rs, {status:200, statusText:'OK', headers:resp.headers}), {add:true,stick:true,size:fileSize,chunkSize:25*encChunkSize})
        .catch(err => {
          console.log('SW cache error', err);
          throw err;
        });
    });
  }

  saveBandwidth_() {
    if ('connection' in navigator) {
      const c = navigator.connection;
      if (c.saveData) {
        throw new Error(_T('data-saving-on'));
      }
      if (c.type === 'wifi') return;
      if (c.type === 'ethernet') return;
      if (this.vars_.downloadOnMobile) return;
      throw new Error(_T('no-wifi'));
    }
  }

  contentType_(ext) {
    let ctype = 'application/octet-stream';
    switch (ext) {
      case '.jpg': case '.jpeg':
        ctype = 'image/jpeg'; break;
      case '.png':
        ctype = 'image/png'; break;
      case '.gif':
        ctype = 'image/gif'; break;
      case '.webp':
        ctype = 'image/webp'; break;
      case '.avif':
        ctype = 'image/avif'; break;
      case '.mp4':
        ctype = 'video/mp4'; break;
      case '.avi':
        ctype = 'video/avi'; break;
      case '.wmv':
        ctype = 'video/x-ms-wmv'; break;
      case '.3gp':
        ctype = 'video/3gpp'; break;
      case '.m1v': case '.m2v': case '.mp2': case '.mpg': case '.mpeg':
        ctype = 'video/mpeg'; break;
      case '.qt': case '.mov': case '.moov':
        ctype = 'video/quicktime'; break;
      case '.mjpg':
        ctype = 'video/x-motion-jpeg'; break;
      case '.webm':
        ctype = 'video/webm'; break;
      case '.aac':
        ctype = 'audio/aac'; break;
      case '.mid': case '.midi':
        ctype = 'audio/midi'; break;
      case '.mp3':
        ctype = 'audio/mpeg'; break;
      case '.oga': case '.ogg': case '.ogv':
        ctype = 'audio/ogg'; break;
      case '.opus':
        ctype = 'audio/opus'; break;
      case '.wav':
        ctype = 'audio/wav'; break;
      case '.weba':
        ctype = 'audio/webm'; break;
      case '.pdf':
        ctype = 'application/pdf'; break;
      case '.txt': case '.text':
        ctype = 'text/plain'; break;
      default:
        console.log(`SW Using default content-type for ${ext}`); break;
    }
    return ctype;
  }

  /*
   */
  async handleFetchEvent(event) {
    const url = new URL(event.request.url);
    if (url.pathname.endsWith('/jsapi')) {
      const p = new Promise(resolve => {
        const params = url.searchParams;
        const func = params.get('func');
        let args = [];
        try {
          args = JSON.parse(self.base64DecodeToString(params.get('args')));
        } catch (e) {
          console.log('SW invalid args', params.get('args'));
        }
        const allowedMethods = [
          'isLoggedIn',
          'quota',
          'keyBackupEnabled',
          'logout',
          'getContact',
          'getContacts',
          'getFiles',
          'getCollections',
          'getCover',
          'getUpdates',
          'moveFiles',
          'emptyTrash',
          'deleteFiles',
          'changeCover',
          'renameCollection',
          'setCollectionOffline',
          'shareCollection',
          'unshareCollection',
          'removeMembers',
          'updatePermissions',
          'leaveCollection',
          'createCollection',
          'deleteCollection',
          'setCachePreference',
          'cachePreference',
          'enableNotifications',
          'mfaStatus',
          'ping',
        ];
        if (allowedMethods.includes(func)) {
          this[func](null, ...args)
          .then(result => {
            let headers = {};
            if (func === 'logout') {
              headers['Clear-Site-Data'] = '*';
            }
            resolve(new Response(JSON.stringify({'resolve': result}), {'status': 200, 'statusText': 'OK', 'headers': headers}));
          })
          .catch(error => {
            console.log(`SW ${func} failed`, error);
            resolve(new Response(JSON.stringify({'reject': error.toString()}), {'status': 200, 'statusText': 'OK'}));
          });
        } else {
          console.log('SW method not allowed', func);
          resolve(new Response('', {'status': 503, 'statusText': 'method not allowed'}));
        }
      });
      return p;
    }

    if (event.request.url.indexOf(`/${this.vars_.decryptPath}/`) === -1) {
      return new Response('No such endpoint', {'status': 404, 'statusText': 'Not found'});
    }

    const p = new Promise(async (resolve, reject) => {
      const ext = url.pathname.replace(/^.*(\.[^.]+)$/, '$1').toLowerCase();
      const ctype = this.contentType_(ext);
      const params = url.searchParams;
      const f = {
        collection: params.get('collection'),
        file: params.get('file'),
        isThumb: params.get('isThumb') === '1',
      };
      const file = await this.getFile_(f.collection, f.file);
      if (!file) {
        return resolve(new Response('Not found', {'status': 404, 'statusText': 'Not found'}));
      }
      const headers = file.headers[f.isThumb?1:0];
      const symKey = this.#decrypt(headers.encKey);
      const chunkSize = headers.chunkSize;
      const encChunkSize = chunkSize+so.XCHACHA20POLY1305_OVERHEAD;
      let startOffset = headers.headerSize;
      let chunkNum = 0;
      let chunkOffset = 0;
      let reqOffset = 0;
      let haveRange = false;
      if (event.request.headers.has('range')) {
        haveRange = true;
        const range = event.request.headers.get('range');
        const re = /^bytes=([0-9]+)-$/;
        const m = re.exec(range);
        reqOffset = m ? parseInt(m[1]) : 0;
        if (reqOffset > 0) {
          chunkNum = Math.floor(reqOffset / chunkSize);
          chunkOffset = reqOffset % chunkSize;
          startOffset += chunkNum * encChunkSize;
        }
      }

      const fileSize = headers.dataSize;
      if (fileSize <= 0) {
        resolve(new Response(new Blob(), {'status': 200, 'statusText': 'OK'}));
        return;
      }
      if (reqOffset > fileSize) {
        resolve(new Response(new Blob(),
          {'status': 416, 'statusText': 'Range Not Satisfiable'}));
        return;
      }
      const strategy = new ByteLengthQueuingStrategy({
        highWaterMark: 5*encChunkSize,
      });

      const cachePref = await this.cachePreference();
      const useCache = cachePref.mode === 'encrypted';
      const cmName = (f.isThumb?'tn':'fs') + '/' + f.file;

      let skipChunks = 0;
      let addToCache = false;
      let resp;
      if (useCache) {
        const pos = startOffset - headers.headerSize;
        const r = await this.cm_.match(cmName, {use:true,offset:pos});
        if (r) {
          if (r.offset % encChunkSize !== 0) {
            console.error('SW invalid cached offset', r.offset);
          } else {
            skipChunks = chunkNum - r.offset / encChunkSize;
            resp = r.response;
          }
        }
      }
      if (!resp) {
        addToCache = useCache && reqOffset === 0 && this.cm_.canAdd();
        resp = await this.getContentUrl_(f)
          .then(url => fetch(url, {
              method: 'GET',
              headers: {
                range: `bytes=${startOffset}-`,
              },
              mode: SAMEORIGIN ? 'same-origin' : 'cors',
              credentials: 'omit',
              redirect: 'error',
              referrerPolicy: 'no-referrer',
            }))
          .catch(() => new Response('', {'status': 502, 'statusText': 'network error'}));
      }
      if (!resp.ok) {
        console.log('SW fetch resp', resp.status);
        return resolve(new Response('', {'status': 502, 'statusText': 'network error'}));
      }
      let onAbort;
      let body = resp.body;
      if (addToCache) {
        const [rs1, rs2] = body.tee();
        body = rs1;
        const state = {};
        onAbort = () => {
          state.cancel = true;
        };
        this.cm_.put(cmName, new Response(new ReadableStream(new CacheStream(rs2, state)), {status:200, statusText:'OK', headers:resp.headers}), {use:true,size:fileSize,chunkSize:25*encChunkSize})
        .catch(err => {
          console.log(`SW ${cmName} not cached`);
        });
      }
      const rs = new ReadableStream(new Decrypter(body.getReader(), symKey, chunkSize, chunkNum, chunkOffset, skipChunks, onAbort), strategy);
      let h = {
        'accept-ranges': 'bytes',
        'cache-control': 'no-store, immutable',
        'content-type': ctype,
      };
      if (ctype.startsWith('application/')) {
        h['content-disposition'] = 'attachment';
        h['content-security-policy'] = 'sandbox;';
      }
      if (cachePref.mode === 'private') {
        h['cache-control'] = 'private, max-age=3600';
      }
      if (haveRange) {
        h['content-range'] = `bytes ${reqOffset}-${fileSize-1}/${fileSize}`;
      } else {
        h['content-length'] = fileSize;
      }
      resolve(new Response(rs, {
        'status': haveRange ? 206 : 200,
        'statusText': haveRange ? 'Partial Content' : 'OK',
        'headers': h,
      }));
    });
    return p;
  }

  async cancelUpload(clientId) {
    this.#state.cancelUpload.cancel = true;
    this.#state.uploadData.forEach(b => {
      b.err = 'canceled';
    });
  }

  async upload(clientId, collection, files) {
    if (files.length === 0) {
      return;
    }
    if (this.#state.cancelUpload === undefined) {
      this.#state.cancelUpload = { cancel: false };
    }
    if (this.#state.uploadData?.length > 0 && this.#state.cancelUpload.cancel) {
      return Promise.reject('canceled');
    }
    this.#state.cancelUpload.cancel = false;

    if (this.#state.streamingUploadWorks === undefined) {
      try {
        const ok = await this.testUploadStream_();
        this.#state.streamingUploadWorks = ok === true;
      } catch (e) {
        this.#state.streamingUploadWorks = false;
      }
    }
    console.log(this.#state.streamingUploadWorks ? 'SW streaming upload is supported by browser' : 'SW streaming upload is NOT supported by browser');

    for (let i = 0; i < files.length; i++) {
      files[i].uploadedBytes = 0;
      files[i].tn = self.base64DecodeToBytes(files[i].thumbnail.split(',')[1]);
      files[i].tnSize = files[i].tn.byteLength;
      delete files[i].thumbnail;
    }

    if (this.#state.uploadData) {
      return new Promise((resolve, reject) => {
        this.#state.uploadData.push({collection, files, resolve, reject});
      });
    }
    this.#state.uploadData = [];

    const p = new Promise(async (resolve, reject) => {
      this.#state.uploadData.push({collection, files, resolve, reject});

      for (let b = 0; b < this.#state.uploadData?.length; b++) {
        let batch = this.#state.uploadData[b];
        for (let i = 0; i < batch.files.length && !batch.err; i++) {
          try {
            await this.uploadFile_(clientId, batch.collection, batch.files[i]);
            delete batch.files[i].tn;
          } catch (err) {
            const name = batch.files[i].name || batch.files[i].file.name;
            console.log(`SW Upload of ${name} failed`, err);
            batch.err = err;
          }
        }
        if (batch.err) {
          batch.reject(batch.err);
        } else {
          batch.resolve();
        }
        batch.done = true;
      }
    });

    const notify = () => {
      if (!this.#state.uploadData) return;
      const state = {
        numFiles: 0,
        numBytes: 0,
        numFilesDone: 0,
        numBytesDone: 0,
      };
      let allDone = true;
      this.#state.uploadData.forEach(b => {
        if (!b.done && !b.err) allDone = false;
        b.files.forEach(f => {
          state.numFiles += 1;
          state.numBytes += f.file.size;
          state.numBytes += f.tnSize;
          if (f.done) {
            state.numFilesDone += 1;
            state.numBytesDone += f.file.size;
            state.numBytesDone += f.tnSize;
          } else {
            state.numBytesDone += f.uploadedBytes;
          }
        });
      });
      state.done = allDone;
      this.#sw.sendUploadProgress(state);
      if (allDone) {
        this.#state.uploadData = null;
      } else {
        self.setTimeout(notify, 500);
      }
    };
    notify();

    return p;
  }

  async uploadFile_(clientId, collection, file, opt_noStreaming) {
    let pk;
    if (collection === 'gallery') {
      pk = this.vars_.pk;
    } else {
      if (!(collection in this.db_.albums)) {
        throw new Error(`invalid album ${collection}`);
      }
      pk = this.db_.albums[collection].pk;
    }
    const [hdr, hdrBin, hdrBase64] = await this.makeHeaders_(pk, file);

    const boundary = Array.from(self.crypto.getRandomValues(new Uint8Array(32))).map(v => ('0'+v.toString(16)).slice(-2)).join('');
    const rs = new ReadableStream(new UploadStream(boundary, hdr, hdrBin, hdrBase64, collection, file, await this.#token(), this.#state.cancelUpload));

    if (this.#state.cancelUpload.cancel) {
      throw new Error('canceled');
    }

    let body = rs;
    if (!this.#state.streamingUploadWorks || opt_noStreaming === true) {
      // Streaming upload is supported in chrome 105+ when using http/2.
      // https://bugs.chromium.org/p/chromium/issues/detail?id=688906
      body = await self.stream2blob(rs);
    }
    const t1 = Date.now();
    return fetch(this.vars_.server + 'v2/sync/upload', {
      method: 'POST',
      mode: SAMEORIGIN ? 'same-origin' : 'cors',
      headers: {
        'Content-Type': 'multipart/form-data; boundary='+boundary,
      },
      redirect: 'error',
      referrerPolicy: 'no-referrer',
      credentials: 'omit',
      body: body,
      duplex: 'half',
    })
    .then(async resp => {
      if (!resp.ok) {
        if (!resp.body) {
          throw new Error(`${resp.status} ${resp.statusText}`);
        }
        const blob = await self.stream2blob(resp.body);
        const body = await blob.text();
        throw body;
      }
      file.done = true;
      return 'ok';
    })
    .catch(err => {
      if (this.#state.cancelUpload.cancel) {
        return Promise.reject('canceled');
      }
      const t = Date.now() - t1;
      if (err instanceof TypeError && t < 10 && this.#state.streamingUploadWorks && !opt_noStreaming) {
        console.log(`SW uploadFile TypeError (${t}ms), retrying without streaming`);
        return this.uploadFile_(clientId, collection, file, true).then(v => {
          console.log('SW uploadFile OK without streaming');
          return v;
        });
      }
      return Promise.reject(err);
    });
  }

   async testUploadStream_() {
    // https://developer.chrome.com/articles/fetch-streaming-requests/#feature-detection
    const supportsRequestStreams = (() => {
      let duplexAccessed = false;

      const hasContentType = new Request('', {
        body: new ReadableStream(),
        method: 'POST',
        get duplex() {
          duplexAccessed = true;
          return 'half';
        },
      }).headers.has('Content-Type');

      return duplexAccessed && !hasContentType;
    })();
    return supportsRequestStreams;
  }

  async makeHeaders_(pk, file) {
    const fileId = self.crypto.getRandomValues(new Uint8Array(32));
    let fileType = 1;
    if (file.file.type.startsWith('image/')) fileType = 2;
    if (file.file.type.startsWith('video/')) fileType = 3;

    const headers = [{
      version: 1,
      chunkSize: 1 << 20,
      dataSize: file.file.size,
      symmetricKey: self.crypto.getRandomValues(new Uint8Array(32)),
      fileType: fileType,
      fileName: file.name || file.file.name,
      duration: Math.floor(file.duration),
    }, {
      version: 1,
      chunkSize: 1 << 20,
      dataSize: file.tnSize,
      symmetricKey: self.crypto.getRandomValues(new Uint8Array(32)),
      fileType: fileType,
      fileName: file.name || file.file.name,
      duration: Math.floor(file.duration),
    }];

    const binHeaders = [];
    const b64Headers = [];
    for (let i = 0; i < 2; i++) {
      const encFileName = self.bytesFromString(headers[i].fileName);
      let h = [];
      h.push(headers[i].version);
      h.push(...self.bigEndian(headers[i].chunkSize, 4));
      h.push(...self.bigEndian(headers[i].dataSize, 8));
      h.push(...headers[i].symmetricKey);
      h.push(headers[i].fileType);
      h.push(...self.bigEndian(encFileName.byteLength, 4));
      h.push(...encFileName);
      h.push(...self.bigEndian(headers[i].duration, 4));
      const encHeader = await so.box_seal(h, pk);
      let out = [];
      out.push(0x53, 0x50, 0x1); // 'S', 'P', 1
      out.push(...fileId);
      out.push(...self.bigEndian(encHeader.byteLength, 4));
      out.push(...encHeader);
      binHeaders.push(new Uint8Array(out));
      b64Headers.push(self.base64RawUrlEncode(out));
    }
    return [headers, binHeaders, b64Headers.join('*')];
  }
}

/*
 * A Transformer to decrypt a stream.
 */
class Decrypter {
  constructor(reader, symKey, chunkSize, n, offset, skipChunks, onAbort) {
    this.reader_ = reader;
    this.symmetricKey_ = symKey;
    this.chunkSize_ = chunkSize;
    this.encChunkSize_ = chunkSize + so.XCHACHA20POLY1305_OVERHEAD;
    this.buf_ = new Uint8Array(0);
    this.n_ = n;
    this.offset_ = offset;
    this.skipChunks_ = skipChunks;
    this.onAbort_ = onAbort;
    this.canceled_ = false;
  }

  start(/*controller*/) {
    this.symmetricKey_ = this.symmetricKey_;
  }

  async pull(controller) {
    while (this.buf_.byteLength < this.encChunkSize_) {
      let {done, value} = await this.reader_.read();
      if (this.canceled_) {
        controller.close();
        return;
      }
      if (done) {
        if (this.skipChunks_ > 0) {
          controller.close();
          return;
        }
        return this.decryptChunk(controller).then(() => {
          controller.close();
        });
      }
      const tmp = new Uint8Array(this.buf_.byteLength + value.byteLength);
      tmp.set(this.buf_);
      tmp.set(value, this.buf_.byteLength);
      this.buf_ = tmp;

      while (this.buf_.byteLength >= this.encChunkSize_ && this.skipChunks_ > 0) {
        this.buf_ = this.buf_.slice(this.encChunkSize_);
        this.skipChunks_--;
      }
    }
    while (this.buf_.byteLength >= this.encChunkSize_) {
      if (this.canceled_) return;
      await this.decryptChunk(controller);
    }
  }

  cancel(/*reason*/) {
    this.canceled_ = true;
    this.reader_.cancel();
    if (this.onAbort_) {
      this.onAbort_();
    }
  }

  async decryptChunk(controller) {
    if (this.buf_.byteLength === 0) {
      return;
    }
    try {
      this.n_++;
      const nonce = Uint8Array.from(this.buf_.slice(0, so.AEAD_XCHACHA20POLY1305_IETF_NPUBBYTES));
      const end = this.buf_.byteLength >= this.encChunkSize_ ? this.encChunkSize_ : this.buf_.byteLength;
      const enc = this.buf_.slice(so.AEAD_XCHACHA20POLY1305_IETF_NPUBBYTES, end);
      const ck = await so.kdf_derive_from_key(32, this.n_, '__data__', this.symmetricKey_);
      let dec = new Uint8Array(await so.aead_xchacha20poly1305_ietf_decrypt(enc, nonce, ck, ''));
      this.buf_ = this.buf_.slice(end);
      if (this.offset_ > 0) {
        dec = dec.slice(this.offset_);
        this.offset_ = 0;
      }
      controller.enqueue(dec);
    } catch (e) {
      controller.error(new Error('decryption error'));
      console.error('SW decryptChunk', e);
      this.cancel();
    }
  }
}

class UploadStream {
  constructor(boundary, hdr, hdrBin, hdrBase64, collection, file, token, cancel) {
    this.boundary_ = boundary;
    this.hdr_ = hdr;
    this.hdrBin_ = hdrBin;
    this.hdrBase64_ = hdrBase64;
    this.set_ = collection === 'gallery' ? 0 : 2;
    this.albumId_ = collection === 'gallery' ? '' : collection;
    this.file_ = file;
    this.token_ = token;
    this.cancel_ = cancel;
    this.filename_ = self.base64RawUrlEncode(self.crypto.getRandomValues(new Uint8Array(32))) + '.sp';
  }

  async start(controller) {
    const fields = {
      headers: this.hdrBase64_,
      set: this.set_,
      albumId: this.albumId_,
      dateCreated: '' + (this.file_.dateCreated || this.file_.file.lastModified),
      dateModified: '' + (this.file_.dateModified || this.file_.file.lastModified),
      version: '1',
      token: this.token_,
    };
    let s = '';
    for (let k in fields) {
      if (!fields.hasOwnProperty(k)) {
        continue;
      }
      s += `--${this.boundary_}\r\n` +
        `Content-Disposition: form-data; name="${k}"\r\n` +
        `\r\n` +
        `${fields[k]}\r\n`;
    }
    controller.enqueue(self.bytesFromBinary(s));

    this.queue_ = [
      {
        name: 'file',
        key: this.hdr_[0].symmetricKey,
        hdrBin: this.hdrBin_[0],
        chunkSize: this.hdr_[0].chunkSize,
        reader: this.file_.file.stream().getReader(),
        n: 0,
      },
      {
        name: 'thumb',
        key: this.hdr_[1].symmetricKey,
        hdrBin: this.hdrBin_[1],
        chunkSize: this.hdr_[1].chunkSize,
        reader: (new Blob([this.file_.tn])).stream().getReader(),
        n: 0,
      },
    ];
  }

  checkCanceled() {
    if (this.cancel_.cancel) {
      this.cancel();
    }
    return this.cancel_.cancel;
  }

  async pull(controller) {
    if (this.queue_.length === 0) {
      controller.enqueue(self.bytesFromBinary(`--${this.boundary_}--\r\n`));
      controller.close();
      return;
    }
    if (this.checkCanceled()) return Promise.reject('canceled');

    return new Promise(async (resolve, reject) => {
      const q = this.queue_[0];
      if (q.n === 0) {
        controller.enqueue(self.bytesFromBinary(`--${this.boundary_}\r\n` +
        `Content-Disposition: form-data; name="${q.name}"; filename="${this.filename_}"\r\n` +
        `Content-Type: application/octet-stream\r\n` +
        `\r\n`));
        q.n = 1;
        q.buf = new Uint8Array(0);
        controller.enqueue(q.hdrBin);
      }
      let eof = false;
      while (q.buf.byteLength < q.chunkSize) {
        if (this.checkCanceled()) return reject('canceled');
        let {done, value} = await q.reader.read();
        if (done) {
          eof = true;
          break;
        }
        const tmp = new Uint8Array(q.buf.byteLength + value.byteLength);
        tmp.set(q.buf);
        tmp.set(value, q.buf.byteLength);
        q.buf = tmp;
      }
      while (q.buf.byteLength >= q.chunkSize) {
        if (this.checkCanceled()) return reject('canceled');
        let chunk = q.buf.slice(0, q.chunkSize);
        q.buf = q.buf.slice(q.chunkSize);
        this.file_.uploadedBytes += chunk.byteLength;
        controller.enqueue(await this.encryptChunk_(q.n, chunk, q.key));
        q.n++;
      }
      if (eof) {
        if (this.checkCanceled()) return reject('canceled');
        if (q.buf.byteLength > 0) {
          this.file_.uploadedBytes += q.buf.byteLength;
          controller.enqueue(await this.encryptChunk_(q.n, q.buf, q.key));
        }
        controller.enqueue(self.bytesFromBinary(`\r\n`));
        this.queue_.shift();
      }
      return resolve();
    });
  }

  cancel(/*reason*/) {
    for (let i = 0; i < this.queue_.length; i++) {
      if (this.queue_[i]?.reader?.close) {
        this.queue_[i].reader.close();
      }
    }
    this.queue_ = [];
  }

  async encryptChunk_(n, data, key) {
    try {
      const nonce = await so.randombytes(so.AEAD_XCHACHA20POLY1305_IETF_NPUBBYTES);
      const ck = await so.kdf_derive_from_key(32, n, '__data__', key);
      const enc = await so.aead_xchacha20poly1305_ietf_encrypt(data, nonce, ck, '');
      const out = new Uint8Array(nonce.byteLength + enc.byteLength);
      out.set(nonce, 0);
      out.set(enc, nonce.byteLength);
      return out;
    } catch (err) {
      console.log('SW encryptChunk', err);
      throw err;
    }
  }
}
