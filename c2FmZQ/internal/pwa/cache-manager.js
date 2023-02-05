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

class CacheManager {
  constructor(store, cache, maxSize) {
    this.store_ = store;
    this.cache_ = cache;
    this.maxSize_ = maxSize;
    this.storePrefix_ = 'cachedata/';
    this.cachePrefix_ = 'local/';
    this.summaryKey_ = this.storePrefix_ + 'summary';
    this.cacheSummary_ = {
      totalSize: 0,
      numEvictable: 0,
    };
  }

  canAdd() {
    return this.cacheSummary_.numEvictable > 0 || this.cacheSummary_.totalSize < this.maxSize_ * 0.95;
  }

  setMaxSize(v) {
    this.maxSize_ = v;
  }

  totalSize() {
    return this.cacheSummary_.totalSize;
  }

  async delete() {
    this.populateCacheDataPromise_ = null;
    this.maxSize_ = 0;
    return this.store_.del(Store2.prefix(this.storePrefix_));
  }

  async selfCheck() {
    return this.populateCacheData_()
      .then(() => this.store_.keys(Store2.prefix(this.storePrefix_)))
      .then(keys => Promise.all(keys.map(k => this.store_.get(k).then(v => ({key: k, value: v})))))
      .then(cachedFiles => {
        let totalSize = 0;
        let numEvictable = 0;
        let summary = {totalSize:0,numEvictable:0};
        cachedFiles.forEach(({key, value}) => {
          if (!value) return;
          if (key === this.summaryKey_) {
            summary = value;
          } else {
            totalSize += value.size;
            if (!value.sticky) {
              numEvictable++;
            }
          }
        });
        if (this.cacheSummary_.totalSize !== totalSize || this.cacheSummary_.numEvictable !== numEvictable ||
            this.cacheSummary_.totalSize !== summary.totalSize || this.cacheSummary_.numEvictable !== summary.numEvictable) {
          console.log(`SW cache self-check counters off: ${this.cacheSummary_.totalSize} ${totalSize} ${this.cacheSummary_.numEvictable} ${numEvictable}`, summary);
          this.cacheSummary_.totalSize = totalSize;
          this.cacheSummary_.numEvictable = numEvictable;
          this.cacheSummary_.changed = true;
          this.delayedSave();
        } else {
          console.log(`SW cache self-check OK ${totalSize} ${numEvictable}`);
        }
      });
  }

  async put(name, resp, opt) {
    opt = opt || {};
    opt.add = true;
    const reader = resp.body.getReader();
    let buf = new Uint8Array(0);
    let leftInChunk = 0;
    let readDone = false;
    let writeDone = false;
    const stream = {
      async pull(controller) {
        if (!readDone) {
          const {done, value} = await reader.read();
          if (done) {
            readDone = true;
          }
          if (value) {
            const tmp = new Uint8Array(buf.byteLength + value.byteLength);
            tmp.set(buf);
            tmp.set(value, buf.byteLength);
            buf = tmp;
          }
        }
        if (readDone && buf.byteLength === 0) {
          controller.close();
          writeDone = true;
        } else {
          const sz = Math.min(buf.byteLength, leftInChunk);
          controller.enqueue(buf.slice(0, sz));
          buf = buf.slice(sz);
          leftInChunk -= sz;
          if (leftInChunk === 0) {
            controller.close();
          }
        }
      },
    };
    const n = i => this.cachePrefix_ + name + '/' + i;
    let count = 0;
    while (!writeDone) {
      leftInChunk = opt.chunkSize;
      try {
        await this.cache_.put(n(count), new Response(new ReadableStream(stream), {status:resp.status, statusText:resp.statusText, headers:resp.headers}));
      } catch (err) {
        for (let i = 0; i < count; i++) {
          await this.cache_.delete(n(i)).catch(err => console.log(`SW undo put ${n(i)} failed`, err));
        }
        throw err;
      }
      count++;
    }
    opt.numChunks = count;
    return this.update(name, opt);
  }

  async exists(name) {
    return this.store_.get(this.storePrefix_ + name)
      .then(result => result !== undefined);
  }

  async match(name, opt) {
    const item = await this.store_.get(this.storePrefix_ + name);
    if (!item) {
      return;
    }
    await this.update(name, opt).catch(err => console.log('SW cache error', err));

    const startChunk = opt.offset ? Math.floor(opt.offset/item.chunkSize) : 0;
    const numChunks = item.numChunks;
    let count = 0;
    let canceled = false;
    const m = async i => this.cache_.match(this.cachePrefix_ + name + '/' + i);
    let resp = await m(startChunk);
    const stream = {
      async start(controller) {
        for (let i = startChunk; i < numChunks; i++) {
          if (i > startChunk) {
            resp = await m(i);
          }
          const reader = resp.body.getReader();
          while (true) {
            const {done, value} = await reader.read();
            if (canceled) {
              controller.error(new Error('canceled'));
              return;
            }
            if (value) {
              controller.enqueue(value);
            }
            if (done) {
              break;
            }
          }
        }
        controller.close();
      },
      cancel() {
        canceled = true;
      },
    };
    return {
      response: new Response(new ReadableStream(stream), {status:resp.status, statusText:resp.statusText, headers:resp.headers}),
      offset: startChunk * item.chunkSize,
    };
  }

  async keys() {
    return this.store_.keys(Store2.prefix(this.storePrefix_))
      .then(keys => keys.filter(k => k !== this.summaryKey_))
      .then(keys => keys.map(k => k.substring(this.storePrefix_.length)));
  }

  async update(name, opt) {
    await this.populateCacheData_();
    if (!this.queues_) {
      this.queues_ = new Map();
    }
    const p = new Promise(async (resolve, reject) => {
      let queue = this.queues_.get(name);
      if (!queue) {
        queue = [];
        this.queues_.set(name, queue);
      }
      if (queue.push({resolve, reject, name, opt}) !== 1) {
        return;
      }
      while(queue.length) {
        const it = queue[0];
        await this.updateInternal_(it.name, it.opt).then(it.resolve, it.reject);
        queue.shift();
      }
      this.queues_.delete(name);
      this.delayedSave();
    });
    return p;
  }

  async populateCacheData_() {
    if (!this.populateCacheDataPromise_) {
      this.populateCacheDataPromise_ = this.store_.get(this.summaryKey_)
        .then(v => {
          if (v) {
            this.cacheSummary_ = v;
          }
        });
    }
    return this.populateCacheDataPromise_;
  }

  async flush() {
    if (this.saveCacheDataPromise_) {
      return this.saveCacheDataPromise_.then(() => this.save());
    }
    return this.save();
  }

  delayedSave() {
    if (!this.cacheSummary_.changed) {
      return;
    }
    if (this.saveTimeoutId_) {
      clearTimeout(this.saveTimeoutId_);
      this.saveTimeoutId_ = null;
    }
    this.saveTimeoutId_ = setTimeout(() => {
      this.saveTimeoutId_ = null;
      this.save().catch(err => console.log('SW cache save', err));
    }, 500);
  }

  async save() {
    if (!this.saveCacheDataPromise_) {
      this.saveCacheDataPromise_ = new Promise((resolve, reject) => {
        if (this.saveTimeoutId_) {
          clearTimeout(this.saveTimeoutId_);
          this.saveTimeoutId_ = null;
        }
        delete this.cacheSummary_.changed;
        this.store_.set(this.summaryKey_, this.cacheSummary_).then(resolve, reject);
      }).finally(() => {
        this.saveCacheDataPromise_ = null;
      });
    }
    return this.saveCacheDataPromise_;
  }

  async updateInternal_(name, opt) {
    opt = opt || {};

    const cacheKey = this.cachePrefix_ + name;
    const storeKey = this.storePrefix_ + name;

    let item = await this.store_.get(storeKey);
    if (opt.delete) {
      if (item) {
        this.cacheSummary_.totalSize -= item.size;
        if (!item.sticky) {
          this.cacheSummary_.numEvictable--;
          this.cacheSummary_.changed = true;
        }
        const p = [];
        p.push(this.store_.del(storeKey).catch(err => console.log(`SW deleting ${storeKey} failed`, err)));
        for (let i = 0; i < item.numChunks; i++) {
          p.push(this.cache_.delete(cacheKey + '/' + i).catch(err => console.log(`SW deleting ${cacheKey}/${i} failed`, err)));
        }
        await Promise.all(p);
      }
    } else {
      if (!item && (opt.add || opt.use)) {
        item = {
          size: opt.size || 0,
          sticky: false,
          lastSeen: 0,
          numChunks: opt.numChunks || 1,
          chunkSize: opt.chunkSize,
          changed: true,
        };
        this.cacheSummary_.totalSize += item.size;
        this.cacheSummary_.numEvictable++;
        this.cacheSummary_.changed = true;
      }
      if (item) {
        if (opt.stick && !item.sticky) {
          this.cacheSummary_.numEvictable--;
          this.cacheSummary_.changed = true;
          item.sticky = true;
          item.changed = true;
        }
        if (opt.unstick && item.sticky) {
          this.cacheSummary_.numEvictable++;
          this.cacheSummary_.changed = true;
          item.sticky = false;
          item.changed = true;
        }
        if (opt.use) {
          item.lastSeen = Date.now();
          item.changed = true;
        }
        if (item.changed) {
          delete item.changed;
          await this.store_.set(storeKey, item);
        }
      }
    }
    if (this.cacheSummary_.numEvictable > 0 && this.cacheSummary_.totalSize > this.maxSize_ * 0.95) {
      return this.reclaimCache_();
    }
  }

  async reclaimCache_() {
    if (!this.reclaimingCache_) {
      this.reclaimingCache_ = this.store_.keys()
        .then(keys => keys.filter(k => k.startsWith(this.storePrefix_) && k !== this.summaryKey_))
        .then(keys => Promise.all(keys.map(k => this.store_.get(k).then(v => ({key: k, value: v})))))
        .then(cachedFiles => {
          let nonSticky = [];
          cachedFiles.forEach(({value, key}) => {
            if (!value.sticky) {
              nonSticky.push({
                key: key.substring(this.storePrefix_.length),
                value: value,
              });
            }
          });
          nonSticky.sort((a, b) => a.value.lastSeen - b.value.lastSeen);

          const p = [];
          for (const it of nonSticky) {
            if (this.cacheSummary_.totalSize <= this.maxSize_ * 0.9) {
              break;
            }
            const storeKey = this.storePrefix_ + it.key;
            const cacheKey = this.cachePrefix_ + it.key;
            p.push(this.store_.del(storeKey).catch(err => console.log(`SW deleting ${storeKey} failed`, err)));
            for (let i = 0; i < it.value.numChunks; i++) {
              p.push(this.cache_.delete(cacheKey + '/' + i).catch(err => console.log(`SW deleting ${cacheKey}/${i} failed`, err)));
            }
            this.cacheSummary_.totalSize -= it.value.size;
            this.cacheSummary_.numEvictable--;
            console.log(`SW evicted ${it.key} size ${it.value.size}`);
          }
          p.push(this.save());
          return Promise.all(p);
        })
        .catch(err => console.log('SW reclaimCache', err))
        .finally(() => {
          this.reclaimingCache_ = null;
        });
    }
    return this.reclaimingCache_;
  }
}

class CacheStream {
  constructor(rs, state) {
    this.reader_ = rs.getReader();
    this.state_ = state;
  }
  async pull(controller) {
    if (this.state_.cancel) {
      this.reader_.cancel();
      controller.error(new Error('canceled stream'));
      return;
    }
    const {value, done} = await this.reader_.read();
    if (value) {
      controller.enqueue(value);
    }
    if (done) {
      controller.close();
    }
  }
}
