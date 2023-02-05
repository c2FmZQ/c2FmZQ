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

/* jshint -W097 */
'use strict';

const DEBUG = false;
const STORENAME = 'store';
const STOREDBVERSION = 2;

class Store2 {
  #key;
  #so;
  #refs;
  #db;
  #openp;
  #pp;
  #name;

  constructor(name) {
    if (DEBUG) console.log(`SW Store constructor`);
    this.#refs = 0;
    this.#db = null;
    this.#openp = null;
    this.#pp = null;
    this.#name = name || 'c2FmZQ';
  }

  lock() {
    this.#key = () => null;
    this.#pp = () => null;
  }

  locked() {
    return !this.#pp || !this.#pp();
  }

  passphrase() {
    return this.#pp ? this.#pp() : this.#pp;
  }

  setPassphrase(pp) {
    if (!pp) {
      this.lock();
      return;
    }
    this.#pp = () => pp;
    this.#setKey();
  }

  setName(name) {
    this.#name = name;
    this.#setKey();
  }

  #setKey() {
    if (!this.#pp || !this.#pp() || !this.#name) return;
    const keyp = self.crypto.subtle.digest('SHA-256', new TextEncoder('utf-8').encode(this.#name + '##' + this.#pp()));
    this.#key = () => keyp;
  }

  async open() {
    if (DEBUG) console.log(`SW store.open()`);
    this.#refs++;
    if (this.#db) {
      if (DEBUG) console.log(`SW Store refs++ ${this.#refs} exists`);
      return;
    }
    if (this.#openp) {
      if (DEBUG) console.log(`SW Store refs++ ${this.#refs} pending`);
      return this.#openp;
    }
    if (DEBUG) console.log(`SW Store refs++ ${this.#refs} start`);
    this.#openp = new Promise(async (resolve, reject) => {
      if (!this.#key()) {
        this.#openp = null;
        this.release();
        return reject(new Error('locked'));
      }
      if (!this.#so) {
        const sodium = new SodiumWrapper();
        await sodium.init();
        this.#so = sodium;
      }
      const req = self.indexedDB.open(this.#name, STOREDBVERSION);
      req.onupgradeneeded = () => {
        const db = req.result;
        if (!db.objectStoreNames.contains(STORENAME)) {
          db.createObjectStore(STORENAME);
        }
      };
      req.onsuccess = () => {
        this.#db = req.result;
        this.#db.onerror = event => {
          console.log('SW Store database error', event);
        };
        this.#db.onabort = event => {
          console.log('SW Store database abort', event);
        };
        this.#db.onversionchange = event => {
          console.log('SW Store database version change', event);
        };
        this.#openp = null;
        resolve();
      };
      req.onerror = event => {
        console.log('SW Store open failed', event);
        this.#openp = null;
        this.release();
        reject();
      };
    });
    return this.#openp;
  }

  async release() {
    this.#refs--;
    if (DEBUG) console.log(`SW Store refs-- ${this.#refs}`);
    if (this.#refs < 0) {
      console.error('SW this.#refs is negative!');
      this.#refs = 0;
    }
    if (this.#db && this.#refs === 0) {
      const db = this.#db;
      this.#db = null;
      db.onerror = null;
      db.onabort = null;
      db.onversionchange = null;
      db.close();
      if (DEBUG) console.log('SW store closed');
    }
  }

  async check() {
    return this.get('__sentinel')
      .then(v => {
        if (v) return;
        return this.#so.randombytes(24)
          .then(nonce => this.set('__sentinel', nonce.toString('hex')));
      })
      .then(() => true, () => false);
  }

  async #encrypt(data) {
    const payload = self.bytesFromString(JSON.stringify(data));
    const nonce = await this.#so.randombytes(24);
    const ct = new Uint8Array(await this.#so.secretbox(payload, nonce, await this.#key()));
    const res = new Uint8Array(nonce.byteLength + ct.byteLength);
    res.set(nonce);
    res.set(ct, nonce.byteLength);
    return res;
  }

  async #decrypt(data) {
    try {
      const v = await this.#so.secretbox_open(data.slice(24), data.slice(0, 24), await this.#key());
      return JSON.parse(v);
    } catch (error) {
      console.error('SW #decrypt', error);
      throw error;
    }
  }

  static prefix(p) {
    if (!p) return null;
    const s = Array.from(p).map(c => c.codePointAt(0));
    s[s.length-1] = s[s.length-1] + 1;
    return IDBKeyRange.bound(p, String.fromCodePoint(...s), false, true);
  }

  async get(key) {
    //if (DEBUG) console.log(`SW Store.get(${key})`);
    await this.open();
    const req = this.#db.transaction(STORENAME, 'readonly').objectStore(STORENAME).get(key);
    return new Promise((resolve, reject) => {
      req.onsuccess = () => {
        if (req.result === undefined) {
          resolve();
        } else {
          resolve(this.#decrypt(req.result));
        }
      };
      req.onerror = reject;
    })
    .finally(() => this.release());
  }

  async set(key, value) {
    //if (DEBUG) console.log(`SW Store.set(${key}, ...)`);
    await this.open();
    const t = this.#db.transaction(STORENAME, 'readwrite');
    return this.#encrypt(value)
      .then(enc => new Promise((resolve, reject) => {
        const req = t.objectStore(STORENAME).put(enc, key);
        req.onsuccess = () => resolve();
        req.onerror = reject;
      }))
      .then(() => new Promise((resolve, reject) => {
        t.oncomplete = () => resolve();
        t.onerror = reject;
        t.commit();
      }))
      .finally(() => this.release());
  }

  async del(key) {
    //if (DEBUG) console.log(`SW Store.del(${key})`);
    await this.open();
    const t = this.#db.transaction(STORENAME, 'readwrite');
    const req = t.objectStore(STORENAME).delete(key);
    return new Promise((resolve, reject) => {
      req.onsuccess = () => resolve();
      req.onerror = reject;
    })
    .then(() => new Promise((resolve, reject) => {
      t.oncomplete = () => resolve();
      t.onerror = reject;
      t.commit();
    }))
    .finally(() => this.release());
  }

  async clear() {
    if (DEBUG) console.log(`SW Store.clear()`);
    await this.open();
    const t = this.#db.transaction(STORENAME, 'readwrite');
    const req = t.objectStore(STORENAME).clear();
    return new Promise((resolve, reject) => {
      req.onsuccess = () => resolve();
      req.onerror = reject;
    })
    .then(() => new Promise((resolve, reject) => {
      t.oncomplete = () => resolve();
      t.onerror = reject;
      t.commit();
    }))
    .finally(() => this.release());
  }

  async keys(range) {
    if (DEBUG) console.log(`SW Store.keys()`);
    await this.open();
    const req = this.#db.transaction(STORENAME, 'readonly').objectStore(STORENAME).getAllKeys(range);
    return new Promise((resolve, reject) => {
      req.onsuccess = () => resolve(req.result);
      req.onerror = reject;
    })
    .finally(() => this.release());
  }
}
