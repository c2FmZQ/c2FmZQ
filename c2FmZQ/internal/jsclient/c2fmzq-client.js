/*
 * Copyright 2021-2022 Robin Thellend
 *
 * This file is part of c2FmZQ.
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

/**
 * c2FmZQ / Stingle client.
 *
 * @class
 */
class c2FmZQClient {
  constructor(options) {
    options.pathPrefix = options.pathPrefix || '/';
    this.options_ = options;
    this.vars_ = {};
    this.resetDB_();
  }

  /*
   * Initialize / restore saved data.
   */
  async init() {
    return Promise.all([
      this.loadVars_(),
      store.get('albums').then(v => {
        this.db_.albums = v || {};
      }),
      store.get('contacts').then(v => {
        this.db_.contacts = v || {};
      }),
    ]);
  }

  /*
   */
  async saveVars_() {
    return store.set('vars', this.vars_);
  }

  /*
   */
  async loadVars_() {
    this.vars_ = await store.get('vars') || {};
    for (let v of ['albumsTimeStamp', 'galleryTimeStamp', 'trashTimeStamp', 'albumFilesTimeStamp', 'contactsTimeStamp', 'deletesTimeStamp']) {
      if (this.vars_[v] === undefined) {
        this.vars_[v] = 0;
      }
    }
  }

  /*
   */
  resetDB_() {
    this.db_ = {
      albums: {},
      files: {
        'gallery': {},
        'trash': {},
      },
      contacts: {},
    };
  }

  /*
   */
  async isLoggedIn() {
    return Promise.resolve(typeof this.vars_.token === "string" && this.vars_.token.length > 0 ? this.vars_.email : '');
  }

  /*
   * Perform the login sequence:
   * - hash the password
   * - send login request
   * - decode / decrypt the keybundle
   */
  async login(clientId, email, password) {
    console.log(`SW login ${email}`);

    return this.sendRequest_(clientId, 'v2/login/preLogin', {'email': email})
      .then(async resp => {
        console.log('SW hashing password');
        const salt = await SodiumUtil.toBuffer(await sodium.sodium_hex2bin(resp.parts.salt));
        let hashed = await sodium.crypto_pwhash(64, password, salt,
          sodium.CRYPTO_PWHASH_OPSLIMIT_MODERATE,
          sodium.CRYPTO_PWHASH_MEMLIMIT_MODERATE,
          sodium.CRYPTO_PWHASH_ALG_ARGON2ID13);
        hashed = hashed.toString('hex').toUpperCase();
        return this.sendRequest_(clientId, 'v2/login/login', {'email': email, 'password': hashed});
      })
      .then(async resp => {
        if (resp.status !== 'ok') {
          throw resp.status;
        }
        this.vars_.token = resp.parts.token;
        this.vars_.serverPK = resp.parts.serverPublicKey;
        console.log('SW decrypting secret key');
        const keys = await this.decodeKeyBundle_(password, resp.parts.keyBundle);
        this.vars_.pk = keys.pk;
        this.vars_.sk = keys.sk;
        console.log('SW logged in');
        this.vars_.email = email;
        this.vars_.userId = resp.parts.userId;
        await this.saveVars_();
        return email;
      });
  }

  /*
   * Logout and clear all saved data.
   */
  async logout(clientId) {
    console.log('SW logout');
    return this.sendRequest_(clientId, 'v2/login/logout', {'token': this.vars_.token})
      .then(() => {
        console.log('SW logged out');
      })
      .catch(console.error)
      .finally(async () => {
        this.vars_ = {};
        this.resetDB_();
        await store.clear();
        this.loadVars_();
        console.log('SW internal data cleared');
      });
  }

  /*
   * Send a getUpdates request, and process the response.
   */
  async getUpdates(clientId) {
    const data = {
      'token': this.vars_.token,
      'filesST': this.vars_.galleryTimeStamp,
      'trashST': this.vars_.trashTimeStamp,
      'albumsST': this.vars_.albumsTimeStamp,
      'albumFilesST': this.vars_.albumFilesTimeStamp,
      'cntST': this.vars_.contactsTimeStamp,
      'delST': this.vars_.deletesTimeStamp,
    };
    return this.sendRequest_(clientId, 'v2/sync/getUpdates', data)
      .then(async resp => {
        /* contacts */
        for (let c of resp.parts.contacts) {
          this.db_.contacts[''+c.userId] = c;
          if (c.dateModified > this.vars_.contactsTimeStamp) {
            this.vars_.contactsTimeStamp = c.dateModified;
          }
        }

        /*  albums */
        const pk = await sodiumPublicKey(this.vars_.pk);
        const sk = await sodiumSecretKey(this.vars_.sk);
        for (let a of resp.parts.albums) {
          try {
            const apk = base64DecodeIntoArray(a.publicKey);
            const ask = await sodium.crypto_box_seal_open(await SodiumUtil.toBuffer(base64DecodeIntoArray(a.encPrivateKey)), pk, sk);

            const md = await Promise.all([
              SodiumUtil.toBuffer(base64DecodeIntoArray(a.metadata)),
              sodiumPublicKey(apk),
              sodiumSecretKey(ask),
            ]).then(v => sodium.crypto_box_seal_open(...v));
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
            const name = md.slice(5, 5+size).toString();
            let members = [];
            if (typeof a.members === 'string') {
              members = a.members.split(',').filter(m => m !== '');
            }
            const obj = {
              'albumId': a.albumId,
              'pk': apk,
              'encSK': a.encPrivateKey,
              'encName': await this.encrypt_(name),
              'cover': a.cover,
              'members': members,
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
                if (f?.[d.file]?.dateModified < d.date) {
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
          store.set('albums', this.db_.albums),
          store.set('contacts', this.db_.contacts),
          Promise.all(Object.keys(changed).map(collection => this.indexCollection_(collection))),
        ];
        await Promise.all(p);
      });
  }

  async encrypt_(data) {
    const pk = await sodiumPublicKey(this.vars_.pk);
    return sodium.crypto_box_seal(await SodiumUtil.toBuffer(data), pk);
  }

  async decrypt_(data) {
    const pk = await sodiumPublicKey(this.vars_.pk);
    const sk = await sodiumSecretKey(this.vars_.sk);
    if (typeof data === 'object' && data.type === 'Buffer') {
      data = data.data;
    }
    try {
      return sodium.crypto_box_seal_open(await SodiumUtil.toBuffer(new Uint8Array(data)), pk, sk);
    } catch (error) {
      console.error('SW decrypt_', error);
      throw error;
    }
  }

  async decryptString_(data) {
    return this.decrypt_(data).then(r => String.fromCharCode(...r));
  }

  async decryptAlbumSK_(albumId) {
    const pk = await sodiumPublicKey(this.vars_.pk);
    const sk = await sodiumSecretKey(this.vars_.sk);
    if (!(albumId in this.db_.albums)) {
      throw new Error('invalid albumId');
    }
    const a = this.db_.albums[albumId];
    return sodiumSecretKey(sodium.crypto_box_seal_open(await SodiumUtil.toBuffer(base64DecodeIntoArray(a.encSK)), pk, sk));
  }

  async insertFile_(collection, file, obj) {
    return store.set(`files/${collection}/${file}`, obj);
  }

  async deleteFile_(collection, file) {
    return store.del(`files/${collection}/${file}`);
  }

  async getFile_(collection, file) {
    return store.get(`files/${collection}/${file}`);
  }

  async deletePrefix_(prefix) {
    return store.keys()
      .then(keys => keys.filter(k => k.startsWith(prefix)))
      .then(keys => Promise.all(keys.map(k => store.del(k))));
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
      'dateCreated': up.dateCreated,
      'dateModified': up.dateModified,
    };
  }

  async indexCollection_(collection) {
    await this.deletePrefix_(`index/${collection}`);

    const prefix = `files/${collection}/`;
    const keys = (await store.keys()).filter(k => k.startsWith(prefix));
    let out = [];
    for (let k of keys) {
      const file = k.substring(prefix.length);
      const f = await this.getFile_(collection, file);
      const obj = {
        'collection': collection,
        'file': f.file,
        'isImage': f.headers[0].fileType === 2,
        'isVideo': f.headers[0].fileType === 3,
        'fileName': await this.decryptString_(f.headers[0].encFileName),
        'dateCreated': f.dateCreated,
        'dateModified': f.dateModified,
      };
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
      p.push(store.set(`index/${collection}/${n}`, obj));
    }
    return Promise.all(p);
  }

  /*
   */
  async getFiles(clientId, collection, offset = 0) {
    const n = ('000000' + offset).slice(-6);
    return store.get(`index/${collection}/${n}`);
  }

  /*
   */
  async getCollections(clientId) {
    return new Promise(async (resolve, reject) => {
      let out = [];
      let cover = await this.getCover_('gallery');
      out.push({'collection': 'gallery', 'name': 'gallery', 'cover': cover});
      out.push({'collection': 'trash', 'name': 'trash', 'cover': null});

      let albums = [];
      for (let n in this.db_.albums) {
        if (!this.db_.albums.hasOwnProperty(n)) {
          continue;
        }
        const a = this.db_.albums[n];
        albums.push({
          'collection': a.albumId,
          'name': await this.decryptString_(a.encName),
          'cover': await this.getCover_(a.albumId),
          'members': a.members.map(m => {
            if (m === this.vars_.userId) return this.vars_.email;
            if (m in this.db_.contacts) return this.db_.contacts[m].email;
            return m;
          }).sort(),
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

  async getCover_(collection) {
    let cover = '';
    if (collection in this.db_.albums) {
      cover = this.db_.albums[collection].cover;
      if (cover === '__b__') {
        return null;
      }
    }
    if (cover === '') {
      const idx = await store.get(`index/${collection}/000000`);
      if (idx?.files?.length > 0) {
        cover = idx.files[0].file;
      }
    }
    if (cover === '') {
      return null;
    }
    const f = await this.getFile_(collection, cover);
    if (!f) {
      return null;
    }
    return this.getDecryptUrl_(f, true);
  }

  async getDecryptUrl_(f, isThumb) {
    if (!f) {
      return null;
    }
    let collection = f.albumId;
    if (f.set === 0) collection = 'gallery';
    else if (f.set === 1) collection = 'trash';
    const fn = await this.decryptString_(f.headers[0].encFileName);
    let url = `${this.options_.pathPrefix}jsdecrypt/${fn}?collection=${collection}&file=${f.file}`;
    if (isThumb) {
      url += '&isThumb=1';
    }
    return url;
  }

  async getContentUrl_(f) {
    const file = await this.getFile_(f.collection, f.file);
    return this.sendRequest_(null, 'v2/sync/getUrl', {
      'token': this.vars_.token,
      'file': file.file,
      'set': file.set,
      'thumb': f.isThumb ? '1' : '0',
    })
    .then(resp => resp.parts.url);
  }

  /*
   */
  async makeParams_(obj) {
    return Promise.all([
      sodium.randombytes_buf(24),
      sodiumSecretKey(this.vars_.sk),
      sodiumPublicKey(this.vars_.serverPK),
    ]).then(v => sodium.crypto_box(JSON.stringify(obj), ...v));
  }

  /*
   */
  async decodeKeyBundle_(password, bundle) {
    const binary = base64Decode(bundle);
    let bytes = [];
    for (let i = 0; i < binary.length; i++) {
      bytes.push(binary.charCodeAt(i));
    }
    if (bytes.length !== 125) {
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
    if (bytes[4] !== 0) {
      throw new Error('secret key not in bundle');
    }
    const pk = await SodiumUtil.toBuffer(new Uint8Array(bytes.slice(5, 37)));
    const esk = await SodiumUtil.toBuffer(new Uint8Array(bytes.slice(37, -40)));
    const salt = await SodiumUtil.toBuffer(new Uint8Array(bytes.slice(-40, -24)));
    const nonce = await SodiumUtil.toBuffer(new Uint8Array(bytes.slice(-24)));

    const key = await sodium.crypto_pwhash(32, password, salt,
              sodium.CRYPTO_PWHASH_OPSLIMIT_MODERATE,
              sodium.CRYPTO_PWHASH_MEMLIMIT_MODERATE,
              sodium.CRYPTO_PWHASH_ALG_ARGON2ID13);
    const sk = await sodium.crypto_secretbox_open(esk, nonce, key);
    return {'pk': pk, 'sk': sk};
  }

  /*
   */
  async decryptHeader_(encHeader, albumId) {
    const bytes = base64DecodeIntoArray(encHeader);
    if (String.fromCharCode(bytes[0], bytes[1]) !== 'SP') {
      throw new Error('invalid header');
    }
    if (bytes[2] !== 1) {
      throw new Error('unexpected header version');
    }
    const fileId = bytes.slice(3, 35);
    let size = 0;
    for (let i = 35; i < 39; i++) {
      size = (size << 8) + bytes[i];
    }
    let pk = this.vars_.pk;
    let sk = this.vars_.sk;
    if (albumId !== '') {
      pk = this.db_.albums[albumId].pk;
      sk = this.decryptAlbumSK_(albumId);
    }
    const hdr = await Promise.all([sodiumPublicKey(pk),sodiumSecretKey(sk)])
      .then(v => sodium.crypto_box_seal_open(new Uint8Array(bytes.slice(39, 39+size)), ...v));
    const version = hdr[0];
    const chunkSize = hdr[1]<<2 | hdr[2]<<16 | hdr[3]<<8 | hdr[4];
    if (chunkSize < 0 || chunkSize > 10485760) {
      throw new Error('invalid chunk size');
    }
    const dataSize = hdr[5]<<56 | hdr[6]<<48 | hdr[7]<<40 | hdr[8]<<32 | hdr[9]<<24 | hdr[10]<<16 | hdr[11]<<8 | hdr[12];
    if (dataSize < 0) {
      throw new Error('invalid data size');
    }
    const symKey = await SodiumUtil.toBuffer(new Uint8Array(hdr.slice(13, 45)));
    const fileType = hdr[45];
    const fnSize = hdr[46]<<24 | hdr[47]<<16 | hdr[48]<<8 | hdr[49];
    if (fnSize < 0 || fnSize+50 > hdr.length) {
      throw new Error('invalid filename size');
    }
    const fn = hdr.slice(50, 50+fnSize);
    const dur = hdr[50+fnSize]<<24 | hdr[51+fnSize]<<16 | hdr[52+fnSize]<<8 | hdr[53+fnSize];
    if (dur < 0) {
      throw new Error('invalid duration');
    }

    const header = {
        chunkSize: chunkSize,
        dataSize: dataSize,
        encKey: await this.encrypt_(symKey),
        fileType: fileType,
        encFileName: await this.encrypt_(String.fromCharCode(...fn).replace(/^ */, '')),
        duration: dur,
        headerSize: bytes.length,
    };
    return header;
  }

  /*
   */
  async sendRequest_(clientId, endpoint, data) {
    //console.log('SW', this.options_.pathPrefix + endpoint);
    let enc = [];
    for (let n in data) {
      if (!data.hasOwnProperty(n)) {
        continue;
      }
      enc.push(encodeURIComponent(n) + '=' + encodeURIComponent(data[n]));
    }
    return fetch(this.options_.pathPrefix + endpoint, {
      method: 'POST',
      mode: 'same-origin',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      redirect: 'error',
      referrerPolicy: 'no-referrer',
      body: enc.join('&'),
    })
    .then(resp => {
      if (!resp.ok) {
        throw new Error(`${resp.status} ${resp.statusText}`);
      }
      return resp.json();
    })
    .then(resp => {
      if (resp.infos.length > 0) {
        self.sendMessage(clientId, {type: 'info', msg: resp.infos.join('\n')});
      }
      if (resp.errors.length > 0) {
        self.sendMessage(clientId, {type: 'error', msg: resp.errors.join('\n')});
      }
      if (resp.parts && resp.parts.logout === "1") {
        this.vars_ = {};
        this.resetDB_();
        store.clear();
        sendLoggedOut();
      }
      if (resp.status === 'ok') {
        //console.log(`SW ${endpoint} response`, resp);
        return resp;
      }
      throw resp.status;
    });
  }

  async ping() {
    return true;
  }

  /*
   */
  async handleFetchEvent(event) {
    const url = new URL(event.request.url);
    if (url.pathname.endsWith('/jsapi')) {
      const p = new Promise((resolve, reject) => {
        const params = url.searchParams;
        const func = params.get('func');
        let args = [];
        try {
          args = JSON.parse(base64Decode(params.get('args')));
        } catch (e) {
          console.log('SW invalid args', params.get('args'));
        }
        if (['isLoggedIn', 'logout', 'getCollections', 'getFiles', 'getUpdates', 'ping'].includes(func)) {
          this[func](null, ...args)
          .then(result => {
            resolve(new Response(JSON.stringify({'resolve': result}), {'status': 200, 'statusText': 'OK'}));
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

    if (event.request.url.indexOf('/jsdecrypt/') === -1) {
      return new Response('No such endpoint', {'status': 404, 'statusText': 'Not found'});
    }

    const p = new Promise(async (resolve, reject) => {
      const params = url.searchParams;
      const f = {
        collection: params.get('collection'),
        file: params.get('file'),
        isThumb: params.get('isThumb'),
      };
      const file = await this.getFile_(f.collection, f.file);
      let startOffset = file.headers[0].headerSize;
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
          chunkNum = Math.floor(reqOffset / file.headers[0].chunkSize);
          chunkOffset = reqOffset % file.headers[0].chunkSize;
          startOffset += chunkNum * (file.headers[0].chunkSize+40);
          //console.info('FETCH RANGE HEADER', range, reqOffset);
        }
      }

      const strategy = new ByteLengthQueuingStrategy({
        highWaterMark: 5*(file.headers[0].chunkSize+40),
      });
      const fileSize = file.headers[f.isThumb?1:0].dataSize;
      if (fileSize <= 0) {
        resolve(new Response(new Blob(), {'status': 200, 'statusText': 'OK'}));
        return;
      }
      if (reqOffset > fileSize) {
        resolve(new Response(new Blob(),
          {'status': 416, 'statusText': 'Range Not Satisfiable'}));
        return;
      }
      const symKey = await sodiumKey(this.decrypt_(file.headers[f.isThumb?1:0].encKey));
      const chunkSize = file.headers[f.isThumb?1:0].chunkSize;

      this.getContentUrl_(f)
      .then(url => {
        return fetch(url, {
          method: 'GET',
          headers: {
            range: `bytes=${startOffset}-`,
          },
          mode: 'same-origin',
          credentials: 'omit',
          redirect: 'error',
          referrerPolicy: 'no-referrer',
        });
      })
      .then(resp => resp.body)
      .then(rs => {
        const reader = rs.getReader();
        const decrypter = new Decrypter(symKey, chunkSize, chunkNum, chunkOffset);
        let canceled = false;
        return new ReadableStream({
          start(controller) {
            function work() {
              reader.read().then(({done, value}) => {
                if (canceled) {
                  return;
                }
                if (done) {
                  decrypter.flush(controller).then(() => controller.close());
                  return;
                }
                decrypter.transform(value, controller).then(work);
              });
            }
            decrypter.start(controller).then(work);
          },
          cancel() {
            canceled = true;
          }
        }, strategy);
      })
      .then(rs => {
        let h = {
          'accept-ranges': 'bytes',
          //'cache-control': 'no-store',
          'cache-control': 'private, max-age=3600',
        };
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
      })
      .catch(e => {
        console.log('SW Error', e);
        const url = new URL(event.request.url);
        self.sendMessage('', {type: 'error', msg: url.pathname + '\n' + e.message});
        reject(e);
      });
    });
    return p;
  }
}

/*
 * A Transformer to decrypt a stream.
 */
class Decrypter {
  constructor(symKey, chunkSize, n, offset) {
    this.symmetricKey_ = symKey;
    this.chunkSize_ = chunkSize;
    this.encChunkSize_ = chunkSize + 40;
    this.buf_ = [];
    this.n_ = n;
    this.offset_ = offset;
  }

  async start(controller) {
    this.symmetricKey_ = await sodiumKey(this.symmetricKey_);
  }

  async transform(chunk, controller) {
    for (let b of chunk) {
      this.buf_.push(b);
    }
    while (this.buf_.length >= this.encChunkSize_) {
      await this.decryptChunk(controller);
    }
  }

  async flush(controller) {
    return this.decryptChunk(controller);
  }

  async decryptChunk(controller) {
    if (this.buf_.length === 0) {
      return;
    }
    try {
      this.n_++;
      const nonce = await SodiumUtil.toBuffer(Uint8Array.from(this.buf_.slice(0, 24)));
      const end = this.buf_.length >= this.encChunkSize_ ? this.encChunkSize_ : this.buf_.length;
      const enc = Uint8Array.from(this.buf_.slice(24, end));
      const ck = await sodium.crypto_kdf_derive_from_key(32, this.n_, '__data__', this.symmetricKey_);
      let dec = new Uint8Array(await sodium.crypto_aead_xchacha20poly1305_ietf_decrypt(enc, nonce, ck, ''));
      this.buf_ = this.buf_.slice(end);
      if (this.offset_ > 0) {
        dec = dec.slice(this.offset_);
        this.offset_ = 0;
      }
      controller.enqueue(dec);
    } catch (e) {
      console.error('SW decryptChunk', e);
    }
  }
}
