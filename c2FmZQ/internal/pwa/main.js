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

'use strict';

/* jshint -W060 */
/* jshint -W126 */
if (!('serviceWorker' in navigator)) {
  document.open();
  document.write('service workers are not supported');
  document.close();
}
/* jshint +W060 */

window.addEventListener('load', () => {
  console.log(`Version ${VERSION}`, DEVEL ? 'DEVEL' : '');
  window.main = new Main();
  window.ui = new UI();
  document.getElementById('version').textContent = VERSION + (DEVEL?' DEVEL':'');
  window.addEventListener('unhandledrejection', event => {
    ui.popupMessage(event.reason);
  });
});

function swTests() {
  if (!navigator.serviceWorker.controller) {
    console.log('No controller');
    return;
  }
  navigator.serviceWorker.controller.postMessage({type: 'run-tests'});
}

class Main {
  constructor() {
    this.salt_ = null;
    this.storeKey_ = null;
    this.rpcId_ = Math.floor(Math.random() * 1000000000);
    this.rpcWait_ = {};
    this.fixing_ = false;

    try {
      const salt = window.localStorage.getItem('salt');
      if (salt) {
        this.salt_ = this.base64DecodeToBytes(salt);
      }
    } catch (err) {
      this.salt_ = null;
    }
    if (this.salt_ === null) {
      this.salt_ = window.crypto.getRandomValues(new Uint8Array(16));
      window.localStorage.setItem('salt', this.base64RawUrlEncode(this.salt_));
      window.localStorage.setItem('resetPassphrase', 'yes');
    }
    const sh = window.localStorage.getItem('sh');
    if (sh) {
      this.serverHashValue_ = this.base64DecodeToBytes(sh);
    }
    navigator.serviceWorker.oncontrollerchange = () => {
      console.log('Controller Change');
      this.sendHello_();
    };
    let swUrl = 'service-worker.js';
    if (window.location.search.includes('tests')) {
      swUrl += '?tests';
    }
    navigator.serviceWorker.register(swUrl)
    .then(r => r.update())
    .then(() => {
      console.log('Service worker updated');
    })
    .catch(err => {
      console.error('Service worker update failed', err);
    })
    .finally(() => {
      this.sendHello_();
    });

    navigator.serviceWorker.onmessage = event => {
      switch (event.data?.type) {
        case 'fixme':
          this.resetServiceWorker();
          break;
        case 'error':
          ui.popupMessage(event.data.msg);
          break;
        case 'info':
          ui.popupMessage(event.data.msg, 'info');
          break;
        case 'loggedout':
          this.lock();
          break;
        case 'hello':
          console.log(`Received hello ${event.data.version}`);
          if (event.data.version !== VERSION) {
            console.log(`Version mismatch: ${event.data.version} != ${VERSION}`);
          }
          if (event.data.err) {
            this.storeKey_ = null;
            ui.wrongPassphrase(event.data.err);
            return;
          }
          if (!event.data.storeKey && this.storeKey_ !== null) {
            this.sendHello_();
          } else if (event.data.storeKey) {
            this.storeKey_ = event.data.storeKey;
            let v = VERSION;
            if (v !== event.data.version) {
              v = `${VERSION}/${event.data.version}`;
            }
            document.getElementById('version').textContent = v + (DEVEL?' DEVEL':'');
            window.localStorage.removeItem('resetPassphrase');
            setTimeout(ui.startUI.bind(ui));
          } else {
            setTimeout(() => {
              ui.promptForPassphrase(window.localStorage.getItem('resetPassphrase') === 'yes');
            });
          }
          break;
        case 'rpc-result':
          if (event.data.func !== 'backupPhrase') {
            //console.log('Received rpc-result', event.data);
          }
          if (event.data.id in this.rpcWait_) {
            if (event.data.reject !== undefined) {
              this.rpcWait_[event.data.id].reject(event.data.reject);
            } else {
              this.rpcWait_[event.data.id].resolve(event.data.resolve);
            }
            delete this.rpcWait_[event.data.id];
          }
          break;
        case 'upload-progress':
          ui.showUploadProgress(event.data.progress);
          navigator.serviceWorker.controller.postMessage({type: 'nop'});
          break;
        case 'download-progress':
          ui.showDownloadProgress(event.data.progress);
          navigator.serviceWorker.controller.postMessage({type: 'nop'});
          break;
        case 'keep-alive':
          navigator.serviceWorker.controller.postMessage({type: 'nop'});
          break;
        case 'jumpto':
          console.log('Received jumpto', event.data.collection);
          this.sendRPC('getUpdates').finally(() => {
            ui.switchView({collection: event.data.collection});
          });
          break;
        case 'rpc':
          console.log('Received rpc', event.data.func);
          if (!['getMFA'].includes(event.data.func)) {
            navigator.serviceWorker.controller.postMessage({id: event.data.id, type: 'rpc-result', reject: 'method not allowed'});
            break;
          }
          this[event.data.func](...event.data.args)
          .then(result => {
            navigator.serviceWorker.controller.postMessage({id: event.data.id, type: 'rpc-result', resolve: result});
          })
          .catch(err => {
            navigator.serviceWorker.controller.postMessage({id: event.data.id, type: 'rpc-result', reject: err});
          });
          break;
        default:
          console.log('Received Message', event.data);
      }
    };
  }

  async setPassphrase(p) {
    if (!p) {
      console.error('empty passphrase');
      return;
    }
    const enc = new TextEncoder();
    const km = await window.crypto.subtle.importKey('raw', enc.encode(p), 'PBKDF2', false, ['deriveBits']);
    const bits = await window.crypto.subtle.deriveBits(
      {'name': 'PBKDF2', salt: this.salt_, 'iterations': 200000, 'hash': 'SHA-256'}, km, 512);
    const a = new Uint8Array(bits);
    const k = base64.fromByteArray(a);
    this.storeKey_ = k;
    this.sendHello_();
  }

  async lock() {
    if (!window.localStorage.getItem('_')) {
      this.storeKey_ = null;
      navigator.serviceWorker.controller.postMessage({type: 'lock'});
    }
    setTimeout(() => {
      window.location.reload();
    }, 25);
  }

  resetPassphrase() {
    window.localStorage.setItem('resetPassphrase', 'yes');
  }

  async resetServiceWorker() {
    console.log('resetServiceWorker', this.fixing_);
    if (this.fixing_) {
      return;
    }
    this.fixing_ = true;
    return navigator.serviceWorker.ready
      .then(r => r.unregister())
      .then(() => {
        window.localStorage.clear();
        window.indexedDB.databases().then(list => Promise.all(list.map(item => window.indexedDB.deleteDatabase(item.name))));
        return new Promise((resolve, reject) => {
          window.setTimeout(resolve, 5000);
        })
        .then(() => window.location.reload());
      });
  }

  setServerFingerPrint(elemId) {
    const elem = document.querySelector(elemId);
    const fp = this.fingerPrint(this.serverHashValue_);
    elem.textContent = fp;
  }

  async calcServerFingerPrint(n, elemId, commit) {
    const elem = document.querySelector(elemId);
    try { 
      n = new URL(n).toString();
    } catch (err) {
      if (elem) {
        elem.textContent = '';
      }
      return Promise.resolve('');
    }
    const data = new TextEncoder().encode(n);
    return window.crypto.subtle.digest('SHA-256', data)
      .then(b => {
        const a = new Uint8Array(b);
        const fp = this.fingerPrint(a);
        if (elem) {
          elem.textContent = fp;
        }
        if (commit) {
          this.serverHashValue_ = a;
          window.localStorage.setItem('sh', this.base64RawUrlEncode(a));
        }
        return fp;
      });
  }

  fingerPrint(hash) {
    const map = ['0','1','2','3','4','5','6','7','8','9','A','B','C','D','E','F'];
    const h = [];
    for (let i = 0; i < 6; i++) {
      h.push(map[(hash[i]>>4)&0xf]+map[hash[i]&0xf]);
    }
    return h.slice(0, 2).join('') + '-' + h.slice(2, 4).join('') + '-' + h.slice(4, 6).join('');
  }

  checkKeyId_(keyId) {
    const b64 = this.base64RawUrlEncode(keyId);
    const sh = window.localStorage.getItem('sh');
    const saved = window.localStorage.getItem(b64);
    if (saved !== null && saved !== sh) {
      throw new Error('keyId belongs to another server');
    }
    if (saved === null) {
      window.localStorage.setItem(b64, sh);
    }
  }

  async addSecurityKey(password, usePassKey) {
    if (!('PublicKeyCredential' in window)) {
      throw new Error('Browser doesn\'t support WebAuthn');
    }
    return this.sendRPC('addSecurityKey', {password, usePassKey})
      .then(options => {
        options.challenge = this.base64DecodeToBytes(options.challenge);
        // Set user.id to the concatenation of server hash and user id from the server.
        const uid = this.base64DecodeToBytes(options.user.id);
        const id = new Uint8Array(this.serverHashValue_.byteLength + uid.byteLength);
        id.set(this.serverHashValue_);
        id.set(uid, this.serverHashValue_.byteLength);
        options.user.id = id;
        if (!SAMEORIGIN) {
          options.user.name = ui.serverUrl_ + '; ' + options.user.name;
        }
        if (options.excludeCredentials) {
          for (let i = 0; i < options.excludeCredentials.length; i++) {
            options.excludeCredentials[i].id = this.base64DecodeToBytes(options.excludeCredentials[i].id);
          }
        }
        const done = ui.freeze({message: _T('select-security-key')});
        return navigator.credentials.create({publicKey: options}).finally(done);
      })
      .then(async pkc => {
        if (pkc.type !== 'public-key' || !(pkc.response instanceof window.AuthenticatorAttestationResponse)) {
          console.log('Invalid credentials.create response', pkc);
          throw new Error('invalid credentials.create response');
        }
        this.checkKeyId_(new Uint8Array(pkc.rawId));
        let keyName = await ui.prompt({
          message: _T('enter-security-key-name'),
          getValue: true,
          defaultValue: pkc.id,
        });
        if (keyName === '') {
          keyName = pkc.id;
        }
        const args = {
          password: password,
          keyName: keyName,
          discoverable: usePassKey,
          clientDataJSON: new Uint8Array(pkc.response.clientDataJSON),
          attestationObject: new Uint8Array(pkc.response.attestationObject),
          transports: pkc.response.getTransports(),
        };
        return this.sendRPC('addSecurityKey', args);
      });
  }

  async getMFA(mfa) {
    const getCode = () => {
      return ui.prompt({
        message: _T('enter-otp'),
        getValue: true,
      })
      .then(v => {
        return {otp: v.trim()};
      });
    };
    const options = mfa.webauthn;
    if (!options || !('PublicKeyCredential' in window)) {
      return getCode();
    }
    options.challenge = this.base64DecodeToBytes(options.challenge);
    if (options.allowCredentials) {
      for (let i = 0; i < options.allowCredentials.length; i++) {
        options.allowCredentials[i].id = this.base64DecodeToBytes(options.allowCredentials[i].id);
        this.checkKeyId_(options.allowCredentials[i].id);
      }
    }
    const done = ui.freeze({message: _T('verify-identity')});
    return navigator.credentials.get({publicKey: options})
      .finally(done)
      .then(pkc => {
        if (pkc.type !== 'public-key' || !(pkc.response instanceof window.AuthenticatorAssertionResponse)) {
          throw new Error('invalid PublicKeyCredential value');
        }
        let userHandle = pkc.response.userHandle;
        if (userHandle) {
          let uh = new Uint8Array(userHandle);
          if (uh.byteLength < this.serverHashValue_.byteLength) {
            throw new Error("invalid userHandle");
          }
          for (let i = 0; i < this.serverHashValue_.byteLength; i++) {
            if (uh[i] !== this.serverHashValue_[i]) {
              throw new Error("invalid userHandle");
            }
          }
          userHandle = uh.slice(this.serverHashValue_.byteLength);
        }
        return {
          webauthn: {
            id: pkc.id,
            clientDataJSON: this.base64RawUrlEncode(new Uint8Array(pkc.response.clientDataJSON)),
            authenticatorData: this.base64RawUrlEncode(new Uint8Array(pkc.response.authenticatorData)),
            signature: this.base64RawUrlEncode(new Uint8Array(pkc.response.signature)),
            userHandle: userHandle ? this.base64RawUrlEncode(userHandle) : undefined,
          },
        };
      })
      .catch(err => {
        console.log('getMFA', err);
        return getCode();
      });
  }

  setHash(key, value) {
    let obj = {};
    try {
      obj = JSON.parse(atob(location.hash.replace(/^#/, '')));
    } catch (e) {}
    obj[key] = value;
    location.hash = btoa(JSON.stringify(obj));
  }

  getHash(field, defaultValue) {
    let obj = {};
    try {
      obj = JSON.parse(atob(location.hash.replace(/^#/, '')));
    } catch (e) {}
    if (obj[field] !== undefined) {
      return obj[field];
    }
    return defaultValue;
  }

  base64DecodeToBytes(v) {
    if (typeof v !== 'string') {
      throw new Error('base64DecodeToBytes arg not string');
    }
    v = v.replaceAll('-', '+').replaceAll('_', '/');
    while (v.length % 4 !== 0) {
      v += '=';
    }
    return base64.toByteArray(v);
  }

  base64RawUrlEncode(v) {
    if (!(v instanceof Uint8Array)) {
      throw new Error('base64RawUrlEncode arg not Uint8Array');
    }
    return base64.fromByteArray(v).replaceAll('+', '-').replaceAll('/', '_').replaceAll('=', '');
  }

  sendHello_() {
    console.log(`Sending hello ${VERSION}`);
    if (!navigator.serviceWorker.controller) {
      console.log('No controller');
      return;
    }
    const capabilities = [''];
    if ('PublicKeyCredential' in window) {
      capabilities.push('mfa');
    }
    if (this.storeKey_ === null) {
      navigator.serviceWorker.controller.postMessage({
        type: 'hello',
        version: VERSION,
        lang: Lang.current,
        capabilities: capabilities,
      });
    } else {
      navigator.serviceWorker.controller.postMessage({
        type: 'hello',
        storeKey: this.storeKey_,
        version: VERSION,
        lang: Lang.current,
        capabilities: capabilities,
        reset: window.localStorage.getItem('resetPassphrase') === 'yes',
      });
    }
  }

  async sendRPC(f, ...args) {
    const sensitiveMethods = [
      'login',
      'createAccount',
      'recoverAccount',
      'upload',
      'cancelUpload',
      'backupPhrase',
      'changeKeyBackup',
      'restoreSecretKey',
      'updateProfile',
      'deleteAccount',
      'generateOTP',
      'addSecurityKey',
      'listSecurityKeys',
      'adminUsers',
      'mfaCheck',
    ];
    document.body.classList.add('waiting');
    let p;
    if (sensitiveMethods.includes(f)) {
      p = this.sendMessageRPC_(f, ...args);
    } else {
      p = this.sendWebRPC_(f, ...args);
    }
    return p.finally(() => {
      document.body.classList.remove('waiting');
    });
  }

  async sendMessageRPC_(f, ...args) {
    const send = () => {
      if (!navigator.serviceWorker.controller || navigator.serviceWorker.controller.state !== 'activated') {
        setTimeout(send, 100);
        return;
      }
      navigator.serviceWorker.controller.postMessage({
        type: 'rpc',
        id: id,
        func: f,
        args: args,
      });
    };
    const id = this.rpcId_++;
    return new Promise(async (resolve, reject) => {
      this.rpcWait_[id] = {'resolve': resolve, 'reject': reject};
      // Keep serviceworker awake.
      if (!this.keepAliveRunning_) {
        this.keepAliveRunning_ = true;
        const keepAlive = async () => {
          if (Object.keys(this.rpcWait_).length === 0) {
            this.keepAliveRunning_ = false;
            return;
          }
          return this.sendWebRPC_('ping').finally(() => {
            setTimeout(keepAlive, 1000);
          });
        };
        await keepAlive();
      }
      send();
    });
  }

  async sendWebRPC_(f, ...args) {
    return new Promise((resolve, reject) => {
      const send = () => {
        if (!navigator.serviceWorker.controller || navigator.serviceWorker.controller.state !== 'activated') {
          setTimeout(send, 100);
          return;
        }
        fetch('jsapi?func='+f+'&args='+base64.fromByteArray(new TextEncoder().encode(JSON.stringify(args))))
        .then(resp => {
          if (!resp.ok) {
            reject(''+resp.status+' '+resp.statusText);
          } else {
            resp.json().then(resp => {
              if (resp.reject !== undefined) reject(resp.reject);
              else resolve(resp.resolve);
            });
          }
        });
      };
      send();
    });
  }
}
