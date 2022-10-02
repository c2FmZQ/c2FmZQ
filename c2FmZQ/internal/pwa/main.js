/*
 * Copyright 2021-2022 TTBT Enterprises LLC
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

class Main {
  constructor() {
    this.salt_ = null;
    this.storeKey_ = null;
    this.rpcId_ = Math.floor(Math.random() * 1000000000);
    this.rpcWait_ = {};
    this.fixing_ = false;

    const salt = localStorage.getItem('salt');
    if (!salt) {
      this.salt_ = window.crypto.getRandomValues(new Uint8Array(16));
      localStorage.setItem('salt', this.salt_.join(','));
    } else {
      this.salt_ = new Uint8Array(salt.split(',').map(v => parseInt(v)));
    }
    navigator.serviceWorker.oncontrollerchange = () => {
      console.log('Controller Change');
      this.sendHello_();
    };
    navigator.serviceWorker.register('service-worker.js')
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
          window.location.reload();
          break;
        case 'hello':
          console.log(`Received hello ${event.data.version}`);
          if (event.data.version !== VERSION) {
            console.log(`Version mismatch: ${event.data.version} != ${VERSION}`);
          }
          if (event.data.err) {
            this.storeKey_ = null;
            ui.wrongPassphrase(event.data.err);
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
            setTimeout(ui.startUI.bind(ui));
          } else {
            setTimeout(ui.promptForPassphrase.bind(ui));
          }
          break;
        case 'rpc-result':
          if (event.data.func !== 'backupPhrase') {
            console.log('Received rpc-result', event.data);
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
      {'name': 'PBKDF2', salt: this.salt_, 'iterations': 200000, 'hash': 'SHA-256'}, km, 256);
    const a = new Uint8Array(bits);
    const k = btoa(String.fromCharCode(...a));
    this.storeKey_ = k;
    this.sendHello_();
  }

  resetServiceWorker() {
    console.log('resetServiceWorker', this.fixing_);
    if (this.fixing_) {
      return;
    }
    this.fixing_ = true;
    navigator.serviceWorker.ready
    .then(r => r.unregister())
    .then(() => {
      window.localStorage.clear();
      let req = window.indexedDB.deleteDatabase('c2FmZQ');
      req.onsuccess = () => console.log('DB deleted');
      req.onerror = () => console.error('DB deletion failed');
      window.requestAnimationFrame(() => {
        window.location.reload();
      });
    });
  }

  async calcServerFingerPrint(n) {
    try { 
      n = new URL(n).toString();
    } catch (err) {
      return Promise.resolve('');
    }
    const data = new TextEncoder().encode(n);
    return window.crypto.subtle.digest('SHA-256', data)
      .then(b => {
        const a = new Uint8Array(b);
        const map = 'BCDFHJMNPQRSTVWZbcdfhjmnpqrstvwz';
        const h = [
          (a[0]) & 0x1f,                  // bits 4, 3, 2, 1, 0
          (a[1] << 3 | a[0] >> 5) & 0x1f, // bits 1, 0, 7, 6, 5
          (a[1] >> 2) & 0x1f,             // bits 6, 5, 4, 3, 2
          (a[2] << 1 | a[1] >> 7) & 0x1f, // bits 3, 2, 1, 0, 7
          (a[3] << 4 | a[2] >> 4) & 0x1f, // bits 0, 7, 6, 5, 4
          (a[3] >> 1) & 0x1f,             // bits 5, 4, 3, 2, 1
          (a[4] << 2 | a[3] >> 6) & 0x1f, // bits 2, 1, 0, 7, 6
          (a[4] >> 3) & 0x1f,             // bits 7, 6, 5, 4, 3
        ].map(c => map.substr(c, 1));
        return h.slice(0, 4).join('') + '-' + h.slice(4, 8).join('');
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

  sendHello_() {
    console.log(`Sending hello ${VERSION}`);
    if (!navigator.serviceWorker.controller) {
      console.log('No controller');
      return;
    }
    navigator.serviceWorker.controller.postMessage({
      type: 'hello',
      storeKey: this.storeKey_,
      version: VERSION,
    });
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
      'adminUsers',
    ];
    const body = document.querySelector('body');
    body.classList.add('waiting');
    let p;
    if (sensitiveMethods.includes(f)) {
      p = this.sendMessageRPC_(f, ...args);
    } else {
      p = this.sendWebRPC_(f, ...args);
    }
    return p.finally(v => {
      body.classList.remove('waiting');
      return v;
    });
  }

  async sendMessageRPC_(f, ...args) {
    // Wake up serviceworker.
    await this.sendWebRPC_('ping');
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
    return new Promise((resolve, reject) => {
      this.rpcWait_[id] = {'resolve': resolve, 'reject': reject};
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
        fetch('jsapi?func='+f+'&args='+btoa(JSON.stringify(args)))
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
