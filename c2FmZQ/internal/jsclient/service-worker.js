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

/* jshint -W079 */
/* jshint -W097 */
'use strict';

console.log('SW loading');

const window = self;
self.importScripts('sodium-plus.min.js');
self.importScripts('secure-webstore.js');
self.importScripts('store.js');
self.importScripts('c2fmzq-client.js');

self.store = new Store();
self.initp = null;
async function initApp(storeKey) {
  const p = new Promise(async (resolve, reject) => {
    if (self.store.passphrase || !storeKey) {
      sendHello();
      return resolve();
    }
    try {
      await store.open(storeKey);
    } catch (err) {
      if (err.message === 'Wrong passphrase') {
        console.log('SW Wrong passphrase');
        sendHello(err.message);
        return resolve();
      }
      return reject(err);
    }
    self.sodium = await self.SodiumPlus.auto();
    const app = new c2FmZQClient({
      pathPrefix: self.location.href.replace(/^(.*\/)[^\/]*/, '$1'),
    });
    await app.init();
    self.app = app;
    console.log('SW app ready');
    sendHello();
    await self.store.release();
    return resolve();
  })
  .finally(() => {
    self.initp = null;
  });
  if (self.initp) {
    console.log('SW initApp called concurrently');
    return self.initp.then(() => p);
  }
  self.initp = p;
  return p;
}

function fixme() {
  self.sendMessage('', {type: 'fixme'});
  setInterval(() => {
    self.sendMessage('', {type: 'fixme'});
  }, 5000);
}

function bin2array(bin) {
  let array = [];
  for (let i = 0; i < bin.length; i++) {
    array.push(bin.charCodeAt(i));
  }
  return new Uint8Array(array);
}

function base64Decode(v) {
  let s = v.replace(/-/g, '+').replace(/_/g, '/');
  const pad = (4 - s.length%4)%4;
  for (let i = 0; i < pad; i++) {
    s += '=';
  }
  try {
    return atob(s);
  } catch (error) {
    console.error('SW base64Decode invalid input', v, error);
    throw error;
  }
}

function base64DecodeIntoArray(v) {
  return bin2array(base64Decode(v));
}

async function sodiumPublicKey(k) {
  return sodiumKey(k, 'public');
}

async function sodiumSecretKey(k) {
  return sodiumKey(k, 'secret');
}

async function sodiumKey(k, type) {
  k = await Promise.resolve(k);
  if (typeof k === 'object') {
    if (k instanceof X25519SecretKey) return k;
    if (k instanceof X25519PublicKey) return k;
    if (k instanceof CryptographyKey) return k;
    if (k.type === 'Buffer') {
      k = new Uint8Array(k.data);
    } else if (k[0] !== undefined && k[31] !== undefined) {
      const kk = new Array(32);
      for (let i = 0; i < 32; i++) {
        kk[i] = k[i];
      }
      k = kk;
    }
  }
  if (typeof k === 'string') {
    k = base64DecodeIntoArray(k);
  }
  if (Array.isArray(k)) {
    k = new Uint8Array(k);
  }
  if (k instanceof Uint8Array) {
    k = await SodiumUtil.toBuffer(k);
  }
  try {
    switch (type) {
      case 'public': return new X25519PublicKey(k);
      case 'secret': return new X25519SecretKey(k);
      default: return new CryptographyKey(k);
    }
  } catch (e) {
    console.error('SW sodiumKey', k, e);
    return null;
  }
}

function sendHello(err) {
  const key = self.store.passphrase;
  console.log('SW Sending hello');
  let msg = {
    type: 'hello',
    storeKey: key,
  };
  if (err) {
    msg.err = err;
  }
  self.sendMessage('', msg);
}

function sendLoggedOut() {
  console.log('SW Sending logged out');
  self.sendMessage('', {type: 'loggedout'});
}

async function sendMessage(id, m) {
  const clients = await self.clients.matchAll({type: 'window'});
  for (let c of clients) {
    if (id === '' || c.id === id) {
      c.postMessage(m);
    }
  }
}

self.addEventListener('install', event => {
  event.waitUntil(self.skipWaiting());
});

self.addEventListener('activate', event => {
  self.clients.claim();
});

self.addEventListener('statechange', event => {
  console.log('SW state change', event);
});

self.addEventListener('resume', event => {
  console.log('SW resume', event);
});

self.addEventListener('message', async event => {
  const clientId = event.source.id;
  switch(event.data?.type) {
    case 'hello':
      console.log('SW Received hello');
      initApp(event.data.storeKey);
      break;
    case 'rpc':
      if (!self.app) {
        self.sendMessage(clientId, {type: 'rpc-result', id: event.data.id, func: event.data.func, reject: 'not ready'});
        return;
      }
      if (!['login'].includes(event.data.func)) {
        console.log('SW RPC method not allowed', event.data.func);
        self.sendMessage(clientId, {type: 'rpc-result', id: event.data.id, func: event.data.func, reject: 'method not allowed'});
        return;
      }
      await store.open();
      self.app[event.data.func](clientId, ...event.data.args)
      .then(e => {
        self.sendMessage(clientId, {type: 'rpc-result', id: event.data.id, func: event.data.func, resolve: e});
      })
      .catch(e => {
        self.sendMessage(clientId, {type: 'rpc-result', id: event.data.id, func: event.data.func, reject: e});
      })
      .finally(() => store.release());
      break;
    default:
      console.log('SW Received unexpected message', event.data);
      break;
  }
});

self.addEventListener('fetch', event => {
  const url = new URL(event.request.url);
  const scope = new URL(self.registration.scope);
  const rel = url.pathname.slice(scope.pathname.length);
  if (!rel.startsWith('jsapi') && !rel.startsWith('jsdecrypt')) {
    event.respondWith(fetch(event.request));
    return;
  }

  let count = 0;
  const p = new Promise(async (resolve, reject) => {
    const handler = async () => {
      if (self.app) {
        await store.open();
        return resolve(self.app.handleFetchEvent(event));
      }
      if (count++ > 100) {
        fixme();
        console.log(event.request);
        return reject(new Error('timeout'));
      }
      setTimeout(handler, 100);
    };
    handler();
  })
  .finally(() => store.release());
  event.respondWith(p);
});

initApp(null);
console.log('SW loaded');
