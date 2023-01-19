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

console.log('SW loading tests');

async function testSodium() {
  const kp = await sodium.crypto_box_keypair();
  const obj = {
    sk: await sodium.crypto_box_secretkey(kp),
    pk: await sodium.crypto_box_publickey(kp),
  };
  const enc = sodium.crypto_box_seal(self.bytesFromString('Hello!'), obj.pk);

  obj.sk = obj.sk.getBuffer();
  obj.pk = obj.pk.getBuffer();

  const ser = JSON.stringify(obj);
  const obj2 = JSON.parse(ser);

  const pk = await self.sodiumPublicKey(obj2.pk);
  const sk = await self.sodiumSecretKey(obj2.sk);

  const dec = await sodium.crypto_box_seal_open(enc, pk, sk);
  if (self.bytesToString(dec) !== 'Hello!') {
    throw 'Unexpected decrypted message';
  }
}

class MockStore {
  constructor() {
    this.store = new Map();
  }
  async get(key) {
    //console.log(`store get(${JSON.stringify(key)})`);
    if (!this.store.has(key)) return null;
    return this.store.get(key);
  }
  async set(key, value) {
    console.log(`store set(${JSON.stringify(key)}, ${JSON.stringify(value)})`);
    this.store.set(key, value);
    return new Promise(resolve => {
      setTimeout(resolve, 10);
    });
  }
  async del(key) {
    console.log(`store del(${JSON.stringify(key)})`);
    return this.store.delete(key);
  }
  async keys() {
    //console.log(`store keys()`);
    const out = [];
    this.store.forEach((v, k) => out.push(k));
    return out;
  }
}

class MockCache {
  constructor() {
    this.cache = new Set();
  }
  add(key) {
    return this.cache.add(key);
  }
  has(key) {
    return this.cache.has(key);
  }
  async keys() {
    console.log('cache keys()');
    const out = [];
    this.cache.forEach(v => out.push({url:v}));
    return out;
  }
  async delete(key) {
    console.log(`cache delete(${JSON.stringify(key)})`);
    return this.cache.delete(key);
  }
}

async function testCache() {
  const store = new MockStore();
  const cache = new MockCache();
  const sw = new ServiceWorker();
  sw.sendMessage = (id, m) => console.log(`SW sendMessage(${id}, ${JSON.stringify(m)})`);
  const app = new c2FmZQClient({store, sw});
  app.cache_ = cache;
  app.db_ = {
    albums: {
      'FOO': {
        isOffline: false,
      },
    },
  };
  app.vars_ = {
    maxCacheSize: 1000,
    cachePref: 'encrypted',
  };
  app.cm_ = new CacheManager(store, cache, app.vars_.maxCacheSize);
  app.fetchCachedFile_ = async (name, collection, isThumb) => {
    cache.add(`local/${isThumb?'tn':'fs'}/${name}/0`);
    app.cm_.update(`${isThumb?'tn':'fs'}/${name}`, {add:true,stick:true,size:100});
  };

  const update = app.cm_.update.bind(app.cm_);
  const check = app.checkCachedFiles_.bind(app);

  const expectCacheData = async want => {
    const keys = (await store.keys()).filter(k => k.startsWith('cachedata/') && k !== 'cachedata/summary');
    const cachedFiles = await Promise.all(keys.map(k => store.get(k).then(v => ({key: k, value: v}))));
    const items = [];
    cachedFiles.forEach(({value, key}) => {
      const name = key.substring(10);
      items.push({
        name: value.sticky ? name.toUpperCase() : name,
        time: value.lastSeen,
        sticky: value.sticky,
      });
    });
    items.sort((a, b) => {
      if (a.time === b.time) {
        if (a.name < b.name) return -1;
        if (a.name > b.name) return 1;
        return 0;
      }
      return a.time - b.time;
    });
    const got = items.map(it => it.name).join(',');
    if (want !== got) {
      throw `Unexpected cachedata: ${got} want ${want}`;
    }
  };
  const expectCacheSet = async want => {
    const items = [];
    cache.cache.forEach(key => items.push(key.substring(6)));
    items.sort();
    const got = items.join(',');
    if (want !== got) {
      throw `Unexpected cacheset: ${got} want ${want}`;
    }
  };

  for (let i = 0; i < 20; i++) {
    await store.set(`files/gallery/file${i}`, `${i}`);
    if (i < 4) {
      await store.set(`files/FOO/file${i}`, `${i}`);
    }
  }
  await expectCacheData('');
  await expectCacheSet('');

  for (let i = 0; i < 9; i++) {
    cache.add(`local/tn/file${i}/0`);
    await update(`tn/file${i}`, {use:true, size:100});
  }
  await expectCacheData('tn/file0,tn/file1,tn/file2,tn/file3,tn/file4,tn/file5,tn/file6,tn/file7,tn/file8');
  await expectCacheSet('tn/file0/0,tn/file1/0,tn/file2/0,tn/file3/0,tn/file4/0,tn/file5/0,tn/file6/0,tn/file7/0,tn/file8/0');

  await update('tn/file0', {use:true, size:100});
  cache.add(`local/tn/file9/0`);
  await update('tn/file9', {use:true, size:100});

  await expectCacheData('tn/file2,tn/file3,tn/file4,tn/file5,tn/file6,tn/file7,tn/file8,tn/file0,tn/file9');
  await expectCacheSet('tn/file0/0,tn/file2/0,tn/file3/0,tn/file4/0,tn/file5/0,tn/file6/0,tn/file7/0,tn/file8/0,tn/file9/0');

  await update('tn/file5', {delete:true});
  await expectCacheData('tn/file2,tn/file3,tn/file4,tn/file6,tn/file7,tn/file8,tn/file0,tn/file9');
  await expectCacheSet('tn/file0/0,tn/file2/0,tn/file3/0,tn/file4/0,tn/file6/0,tn/file7/0,tn/file8/0,tn/file9/0');

  await update('tn/file2', {stick:true, size:100});
  cache.add(`local/tn/file11/0`);
  await update('tn/file11', {add:true, stick:true, size:100});
  cache.add(`local/tn/file12/0`);
  await update('tn/file12', {use:true, size:100});
  cache.add(`local/tn/file13/0`);
  await update('tn/file13', {use:true, size:100});

  await expectCacheData('TN/FILE11,TN/FILE2,tn/file6,tn/file7,tn/file8,tn/file0,tn/file9,tn/file12,tn/file13');
  await expectCacheSet('tn/file0/0,tn/file11/0,tn/file12/0,tn/file13/0,tn/file2/0,tn/file6/0,tn/file7/0,tn/file8/0,tn/file9/0');

  await check();
  await expectCacheData('tn/file11,tn/file2,tn/file6,tn/file7,tn/file8,tn/file0,tn/file9,tn/file12,tn/file13');
  await expectCacheSet('tn/file0/0,tn/file11/0,tn/file12/0,tn/file13/0,tn/file2/0,tn/file6/0,tn/file7/0,tn/file8/0,tn/file9/0');

  app.db_.albums.FOO.isOffline = true;
  // Expect files0-3 to be added to the cache (or updated) as sticky.
  await check();
  await expectCacheData('FS/FILE0,FS/FILE1,FS/FILE2,FS/FILE3,TN/FILE1,TN/FILE3,TN/FILE2,TN/FILE0,tn/file13');
  await expectCacheSet('fs/file0/0,fs/file1/0,fs/file2/0,fs/file3/0,tn/file0/0,tn/file1/0,tn/file13/0,tn/file2/0,tn/file3/0');

  await app.cm_.delete();
}
  
async function runTests() {
  const tests = Object.keys(self).filter(k => k.startsWith('test') && typeof self[k] === 'function');
  const results = [];
  for (const t of tests) {
    console.log(`SW running test: ${t}`);
    try {
      await self[t]();
      console.log(`${t} PASS`);
      results.push({test:t, result:'PASS'});
    } catch (err) {
      console.error(`${t} FAIL`, err);
      results.push({test:t, result:'FAIL', err:err});
    }
  }
  let pass = true;
  console.log('===== TEST SUMMARY =====');
  for (const r of results) {
    if (r.result !== 'PASS') pass = false;
    console.log(`${r.test} ${r.result} ${r.err||''}`);
  }
  console.log(pass ? 'PASS' : 'FAIL');
  return results;
}
console.log('SW tests loaded');
