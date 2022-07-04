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

/* jshint -W097 */
'use strict';

let DEBUG = false;

/*
 * Wrapper around SecureStore.Store to open DB connections when needed and
 * close them as soon as they are no longer used.
 */
class Store {
  constructor() {
    if (DEBUG) console.log(`SW Store constructor`);
    this.refs = 0;
    this.store = null;
    this.storep = null;
    this.passphrase = null;
  }

  async open(storeKey) {
    if (DEBUG) console.log(`SW store.open()`);
    this.refs++;
    if (this.store) {
      if (DEBUG) console.log(`SW Store refs++ ${this.refs} exists`);
      return this.store;
    }
    if (this.storep) {
      if (DEBUG) console.log(`SW Store refs++ ${this.refs} pending`);
      return this.storep;
    }
    if (DEBUG) console.log(`SW Store refs++ ${this.refs} start`);
    this.storep = new Promise(async (resolve, reject) => {
      let store;
      try {
        const key = storeKey || this.passphrase;
        if (!key) {
          this.storep = null;
          return reject(false);
        }
        store = new SecureStore.Store('c2FmZQ', key);
        if (DEBUG) console.log(`SW store init`);
        await store.init();
        this.passphrase = key;
        if (DEBUG) console.log(`SW store init done`);
      } catch (err) {
        console.info('SW store.init failed', err);
        this.storep = null;
        if (err.message !== 'Wrong passphrase') {
          // XXX fixme();
          if (store) {
            store.destroy();
          }
        }
        return reject(err);
      }
      this.store = store;
      if (DEBUG) console.log('SW store opened');
      this.storep = null;
      return resolve(true);
    });
    return this.storep;
  }

  async release() {
    this.refs--;
    if (DEBUG) console.log(`SW Store refs-- ${this.refs}`);
    if (this.refs < 0) {
      console.error('SW this.refs is negative!');
      this.refs = 0;
    }
    if (this.store && this.refs === 0) {
      let s = this.store;
      this.store = null;
      await s.close();
      if (DEBUG) console.log('SW store closed');
    }
  }

  async get(key) {
    //if (DEBUG) console.log(`SW Store.get(${key})`);
    return this.open()
      .then(() => this.store.get(key))
      .finally(() => this.release());
  }

  async set(key, value) {
    //if (DEBUG) console.log(`SW Store.set(${key}, ...)`);
    return this.open()
      .then(() => this.store.set(key, value))
      .finally(() => this.release());
  }

  async del(key) {
    //if (DEBUG) console.log(`SW Store.del(${key})`);
    return this.open()
      .then(() => this.store.del(key))
      .finally(() => this.release());
  }

  async clear() {
    if (DEBUG) console.log(`SW Store.clear()`);
    return this.open()
      .then(() => this.store.clear())
      .finally(() => this.release());
  }

  async destroy() {
    if (DEBUG) console.log(`SW Store.destroy()`);
    return this.open()
      .then(() => this.store.destroy())
      .finally(() => this.release());
  }

  async keys() {
    if (DEBUG) console.log(`SW Store.keys()`);
    return this.open()
      .then(() => this.store.keys())
      .finally(() => this.release());
  }
}
