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

/* jshint -W079 */
/* jshint -W097 */
'use strict';

self.importScripts('version.js');
console.log(`SW Version ${VERSION}`, DEVEL ? 'DEVEL' : '');

let MANIFEST = [
  'c2fmzq.webmanifest',
  'c2fmzq-client.js',
  'c2.png',
  'c2-bg.png',
  'c2-144x144.png',
  'cache-manager.js',
  'clear.png',
  'index.html',
  'lang.js',
  'main.js',
  'store2.js',
  'style.css',
  'ui.js',
  'utils.js',
  'version.js',
  'thirdparty/browser-libs.js',
  'thirdparty/filerobot-image-editor.min.js',
  'thirdparty/libs.js',
];
if (self.location.search.includes('tests')) {
  MANIFEST.push('sw-tests.js');
  MANIFEST.push('sw-tests.html');
  self.importScripts('sw-tests.js');
}

self.importScripts('thirdparty/libs.js');
self.importScripts('lang.js');
self.importScripts('store2.js');
self.importScripts('utils.js');
self.importScripts('c2fmzq-client.js');
self.importScripts('cache-manager.js');

self._T = Lang.text;

class ServiceWorker {
  #app;
  #state;
  #store;
  #notifs;
  constructor() {
    this.#state = {};
    this.#state.initp = null;
    this.#state.rpcId = Math.floor(Math.random() * 1000000000);
    this.#state.rpcWait = {};
    this.#store = new Store2();
    this.#notifs = new Store2('notifications');
    this.#notifs.setPassphrase('notifications');
  }

  static start() {
    const sw = new ServiceWorker();
    self.addEventListener('install', event => sw.#oninstall(event));
    self.addEventListener('activate', event => sw.#onactivate(event));
    self.addEventListener('freeze', event => sw.#onfreeze(event));
    self.addEventListener('resume', event => sw.#onresume(event));
    self.addEventListener('statechange', event => sw.#onstatechange(event));
    self.addEventListener('unhandledrejection', event => sw.#onunhandledrejection(event));
    self.addEventListener('message', event => sw.#onmessage(event));
    self.addEventListener('notificationclick', event => sw.#onnotificationclick(event));
    self.addEventListener('push', event => sw.#onpush(event));
    self.addEventListener('pushsubscriptionchange', event => sw.#onpushsubscriptionchange(event));
    self.addEventListener('fetch', event => sw.#onfetch(event));
    if ('connection' in navigator) {
      sw.#state.currentNetworkType = navigator.connection.type;
      navigator.connection.addEventListener('change', event => {
        if (sw.#state.currentNetworkType !== navigator.connection.type) {
          console.log(`SW network changed ${sw.currentNetworkType_} -> ${navigator.connection.type}`);
          sw.#state.currentNetworkType = navigator.connection.type;
          if (sw.#app) {
            sw.#app.onNetworkChange(event);
          }
        }
      });
    }
    sw.#sendHello();
  }

  async #initApp(storeKey, storeName, capabilities, reset) {
    const p = new Promise(async (resolve, reject) => {
      const dbList = await self.indexedDB.databases().then(list => list.filter(item => item.name !== 'notifications').map(item => item.name));
      console.log(`SW storeName ${storeName} ${JSON.stringify(dbList)}`);
      try {
        if (!reset && dbList.length && !dbList.includes(storeName)) {
          console.log('SW Wrong passphrase (new storeName)');
          this.#sendHello('Wrong passphrase');
          return resolve();
        }
        this.#store.setName(storeName);
        this.#store.setPassphrase(storeKey);
        await this.#store.open();
        if (!await this.#store.check()) {
          console.log('SW Wrong passphrase');
          this.#sendHello('Wrong passphrase');
          return resolve();
        }
      } catch (err) {
        return reject(err);
      }
      if (this.#state.appInitialized) {
        return resolve();
      }
      this.#state.appInitialized = true;
      dbList.filter(item => item !== storeName).forEach(name => {
        console.log(`SW Delete database ${name}`);
        try {
          self.indexedDB.deleteDatabase(name);
        } catch (err) {
          console.log(`SW deleteDatabase(${name}):`, err);
        }
      });
      const app = new c2FmZQClient({
        store: this.#store,
        sw: this,
        capabilities: capabilities,
        pathPrefix: self.location.href.replace(/^(.*\/)[^\/]*/, '$1'),
      });
      await app.init();
      this.#app = app;
      console.log('SW app ready');
      this.#sendHello();
      await this.#store.release();
      setTimeout(this.#checkNotifications.bind(this), 500);
      setTimeout(this.#checkPushsubscriptionchanges.bind(this), 500);
      return resolve();
    })
    .finally(() => {
      this.#state.initp = null;
    });
    if (this.#state.initp) {
      console.log('SW initApp called concurrently');
      return this.#state.initp.then(() => p);
    }
    this.#state.initp = p;
    return p;
  }

  #checkNotifications() {
    if (this.#state.checkingNotifications) {
      setTimeout(this.#checkNotifications.bind(this), 500);
      return;
    }
    this.#state.checkingNotifications = true;
    this.#notifs.keys()
    .then(keys => keys.filter(k => k.startsWith("notifs/")))
    .then(keys => {
      if (keys.length === 0) {
        return;
      }
      if (!this.#app) {
        self.showNotif(_T('notification-encrypted-title', keys.length), {
          tag: 'encrypted',
          body: _T('notification-encrypted-body'),
          requireInteraction: true,
        });
        return;
      }
      self.registration.getNotifications({tag:'encrypted'}).then(nn => nn.forEach(n => n.close()));
      keys.forEach(k => {
        this.#notifs.get(k)
          .then(v => this.#app.onpush(v))
          .finally(() => this.#notifs.del(k));
      });
    })
    .finally(() => {
      this.#state.checkingNotifications = false;
    });
  }

  #checkPushsubscriptionchanges() {
    if (this.#state.checkingPushsubscriptionchanges) {
      setTimeout(this.#checkPushsubscriptionchanges.bind(this), 500);
      return;
    }
    this.#state.checkingPushsubscriptionchanges = true;
    this.#notifs.keys()
    .then(keys => keys.filter(k => k.startsWith("pushsubscriptionchange/")))
    .then(keys => {
      if (keys.length === 0 || !this.#app) {
        return;
      }
      keys.forEach(k => {
        this.#notifs.get(k)
          .then(v => this.#app.enableNotifications('', true))
          .then(() => this.#notifs.del(k));
      });
    })
    .finally(() => {
      this.#state.checkingPushsubscriptionchanges = false;
    });
  }

  #sendHello(err) {
    const key = this.#store.passphrase();
    console.log(`SW Sending hello ${VERSION}`);
    let msg = {
      type: 'hello',
      storeKey: key,
      version: VERSION,
    };
    if (err) {
      msg.err = err;
    }
    this.sendMessage('', msg);
  }

  async sendRPC(clientId, f, ...args) {
    const id = this.#state.rpcId++;
    return new Promise((resolve, reject) => {
      this.#state.rpcWait[id] = {resolve, reject};
      return this.sendMessage(clientId, {
        type: 'rpc',
        id: id,
        func: f,
        args: args,
      });
    });
  }

  sendLoggedOut() {
    console.log('SW Sending logged out');
    this.sendMessage('', {type: 'loggedout'});
  }

  sendUploadProgress(p) {
    this.sendMessage('', {type: 'upload-progress', progress: p});
  }

  sendDownloadProgress(p) {
    this.sendMessage('', {type: 'download-progress', progress: p});
  }

  async sendMessage(id, m) {
    const clients = await self.clients.matchAll({type: 'window'});
    if (clients.length === 0) {
      console.log(`SW no clients ${VERSION}`);
      return;
    }
    for (let c of clients) {
      if (id === '' || c.id === id) {
        try {
          c.postMessage(m);
        } catch (err) {
          console.log('SW sendMessage failed', m, err);
        }
      }
    }
  }

  async showNotif(title, options) {
    options.badge = './c2.png';
    return self.registration.showNotification(title, options);
  }

  async #handleRpcRequest(clientId, event) {
    if (!this.#app) {
      console.log('SW handleRpcRequest not ready');
      setTimeout(() => {
        self.#handleRpcRequest(clientId, event);
      }, 100);
      return;
    }
    const methods = [
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
    if (!methods.includes(event.data.func)) {
      console.log('SW RPC method not allowed', event.data.func);
      this.sendMessage(clientId, {type: 'rpc-result', id: event.data.id, func: event.data.func, reject: 'method not allowed'});
      return;
    }
    await this.#store.open();
    this.#app[event.data.func](clientId, ...event.data.args)
    .then(e => {
      this.sendMessage(clientId, {type: 'rpc-result', id: event.data.id, func: event.data.func, resolve: e});
    })
    .catch(e => {
      this.sendMessage(clientId, {type: 'rpc-result', id: event.data.id, func: event.data.func, reject: e});
    })
    .finally(() => this.#store.release());
  }

  async #handleRpcResult(clientId, event) {
    if (event.data.id in this.#state.rpcWait) {
      if (event.data.reject !== undefined) {
        this.#state.rpcWait[event.data.id].reject(event.data.reject);
      } else {
        this.#state.rpcWait[event.data.id].resolve(event.data.resolve);
      }
      delete this.#state.rpcWait[event.data.id];
    }
  }

  #oninstall(event) {
    if (DEVEL) {
      console.log(`SW install ${VERSION} DEVEL`);
      event.waitUntil(self.skipWaiting());
      return;
    }
    console.log(`SW install ${VERSION}`);
    event.waitUntil(
      self.caches.open(VERSION).then(c => c.addAll(MANIFEST))
    );
  }

  #onactivate(event) {
    if (DEVEL) {
      console.log(`SW activate ${VERSION} DEVEL`);
      event.waitUntil(
        self.caches.keys()
        .then(keys => keys.filter(k => k !== 'local').map(k => self.caches.delete(k)))
        .then(p => Promise.all(p))
        .then(r => console.log('SW cache deletes', r))
        .then(() => self.clients.claim())
      );
      return;
    }
    console.log(`SW activate ${VERSION}`);
    event.waitUntil(
      self.caches.keys()
      .then(keys => keys.filter(k => k !== VERSION && k !== 'local').map(k => self.caches.delete(k)))
      .then(p => Promise.all(p))
      .then(r => console.log('SW cache deletes', r))
      .then(() => self.clients.claim())
    );
  }

  #onfreeze(event) {
    console.log('SW freeze', event);
  }

  #onresume(event) {
    console.log('SW resume', event);
  }

  #onstatechange(event) {
    console.log('SW state change', event);
  }

  #onunhandledrejection(event) {
    this.sendMessage('', {type: 'error', msg: event.reason});
  }

  async #onmessage(event) {
    const clientId = event.source.id;
    switch(event.data?.type) {
      case 'nop':
        break;
      case 'hello':
        console.log(`SW Received hello ${event.data.version}`);
        if (event.data.version !== VERSION) {
          console.log(`SW Version mismatch: ${event.data.version} != ${VERSION}`);
        }
        Lang.current = event.data.lang || 'en-US';
        if (!event.data.storeKey) {
          this.#sendHello();
        } else {
          this.#initApp(event.data.storeKey, event.data.storeName, event.data.capabilities, event.data.reset);
        }
        break;
      case 'lock':
        console.log('SW Received lock');
        this.#store.lock();
        this.#app = null;
        this.#state.appInitialized = false;
        this.#store = new Store2();
        break;
      case 'rpc':
        this.#handleRpcRequest(clientId, event);
        break;
      case 'rpc-result':
        this.#handleRpcResult(clientId, event);
        break;
      case 'run-tests':
        self.runTests()
        .then(results => {
          this.sendMessage(clientId, {type: 'test-results', results: results});
        });
        break;
      default:
        console.log('SW Received unexpected message', event.data);
        break;
    }
  }

  #onnotificationclick(event) {
    const tag = event.notification.tag;
    let jumpTo;
    if (tag.startsWith('new-content:') || tag.startsWith('new-member:') || tag.startsWith('new-collection:')) {
      jumpTo = tag.substring(tag.indexOf(':')+1);
    }
    event.notification.close();
    event.waitUntil(
      self.clients.matchAll({type: "window"})
      .then(list => {
        if (tag.startsWith('remote-mfa:')) {
          if (event.action && event.action === 'deny') {
            console.log('SW Remove MFA denied');
            return;
          }
          if (!this.#app) {
            throw new Error('app is not ready');
          }
          return this.#app.approveRemoteMFA(tag.substring(tag.indexOf(':')+1));
        }

        let url = self.registration.scope;
        if (jumpTo) {
          url += '#' + btoa(JSON.stringify({collection: jumpTo}));
        }
        let client;
        for (const c of list) {
          if (c.url.startsWith(self.registration.scope) && c.focus) {
            client = c;
            break;
          }
        }
        if (!client) {
          return self.clients.openWindow(url);
        }
        return client.focus()
          .catch(err => console.log('SW focus:', err))
          .then(() => new Promise((resolve) => setTimeout(resolve, 500)))
          .then(() => {
            if (jumpTo) {
              return client.navigate(url).catch(err => console.log('SW navigate:', err));
            }
          })
          .then(() => {
            if (jumpTo) {
              return this.sendMessage(client.id, {type: 'jumpto', collection: jumpTo});
            }
          });
      })
    );
  }

  #onpush(event) {
    const data = event.data ? event.data.text() : null;
    event.waitUntil(
      this.#notifs.set('notifs/' + new Date().getTime(), data)
      .then(() => this.#checkNotifications())
    );
  }

  #onpushsubscriptionchange(event) {
    console.log('SW pushsubscriptionchange', event);
    event.waitUntil(
      this.#notifs.set('pushsubscriptionchange/' + new Date().getTime(), event.oldSubscription.options)
      .then(() => this.#checkPushsubscriptionchanges())
    );
  }

  #onfetch(event) {
    const reqUrl = event.request.url.replace(/#.*$/, '');
    if (!reqUrl.startsWith(self.registration.scope)) {
      console.error('SW fetch req out of scope', reqUrl, self.registration.scope);
      event.respondWith(new Response('request out of scope', {'status': 403, 'statusText': 'Permission denied'}));
      return;
    }
    const url = new URL(reqUrl);
    const scope = new URL(self.registration.scope);
    let rel = url.pathname.slice(scope.pathname.length);
    if (rel === '') {
      rel = 'index.html';
    }
    if (MANIFEST.includes(rel)) {
      event.respondWith(
        self.caches.match(rel).then(resp => {
          if (resp) return resp;
          console.log(`SW fetch ${rel}, no cache`);
          return fetch(event.request);
        })
      );
      return;
    }
    let count = 0;
    event.respondWith(new Promise(async (resolve, reject) => {
      const handler = async () => {
        if (this.#app) {
          await this.#store.open();
          return resolve(this.#app.handleFetchEvent(event).finally(() => this.#store.release()));
        }
        if (count++ > 100) {
          if (!this.#store.locked()) {
            this.fixme();
            console.log(event.request);
          }
          return resolve(new Response('Service Unavailable', {'status': 502, 'statusText': 'Service Unavailable'}));
        }
        setTimeout(handler, 100);
      };
      handler();
    }));
  }

  fixme() {
    this.sendMessage('', {type: 'fixme'});
    setInterval(() => {
      this.sendMessage('', {type: 'fixme'});
    }, 5000);
  }
}

ServiceWorker.start();
console.log(`SW loaded ${VERSION}`);
