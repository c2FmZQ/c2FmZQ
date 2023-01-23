
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

function bytesFromString(s) {
  return new TextEncoder('utf-8').encode(s);
}

function bytesToString(a) {
  return new TextDecoder('utf-8').decode(a);
}

function bytesFromBinary(bin) {
  let array = [];
  for (let i = 0; i < bin.length; i++) {
    array.push(bin.charCodeAt(i));
  }
  return new Uint8Array(array);
}

function bigEndian(n, size) {
  let a = [];
  while (size-- > 0) {
    a.unshift(n & 0xff);
    n >>= 8;
  }
  return new Uint8Array(a);
}

function base64RawUrlEncode(v) {
  return base64StdEncode(v).replaceAll('+', '-').replaceAll('/', '_').replaceAll('=', '');
}

function base64StdEncode(v) {
  if (Array.isArray(v)) {
    v = new Uint8Array(v);
  }
  return base64.fromByteArray(v);
}

function base64DecodeToString(s) {
  return self.bytesToString(base64DecodeToBytes(s));
}

function base64DecodeToBytes(v) {
  v = v.replaceAll('-', '+').replaceAll('_', '/');
  while (v.length % 4 !== 0) {
    v += '=';
  }
  return base64.toByteArray(v);
}

async function stream2blob(rs) {
  const reader = rs.getReader();
  const buf = [];
  while (true) {
    let {done, value} = await reader.read();
    if (done) break;
    buf.push(value);
  }
  return new Blob(buf);
}

async function sodiumPublicKey(k) {
  return sodiumKey(k, 'public');
}

async function sodiumSecretKey(k) {
  return sodiumKey(k, 'secret');
}

async function sodiumKey(k, type) {
  if (k === undefined) return k;
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
  if (typeof k === 'string' && k.length !== 32) {
    k = base64DecodeToBytes(k);
  }
  if (Array.isArray(k)) {
    k = new Uint8Array(k);
  }
  try {
    switch (type) {
      case 'public': return X25519PublicKey.from(k);
      case 'secret': return X25519SecretKey.from(k);
      default: return CryptographyKey.from(k);
    }
  } catch (e) {
    console.error('SW sodiumKey', k, e);
    return null;
  }
}

class SodiumWrapper {
  async init() {
    const sodium = await self.SodiumPlus.auto();
    this.PWHASH_OPSLIMIT_MODERATE = sodium.CRYPTO_PWHASH_OPSLIMIT_MODERATE;
    this.PWHASH_OPSLIMIT_INTERACTIVE = sodium.CRYPTO_PWHASH_OPSLIMIT_INTERACTIVE;
    this.PWHASH_MEMLIMIT_MODERATE = sodium.CRYPTO_PWHASH_MEMLIMIT_MODERATE;
    this.PWHASH_OPSLIMIT_MIN = 1; // For tests only
    this.PWHASH_MEMLIMIT_MIN = 8192; // For tests only
    this.PWHASH_ALG_ARGON2ID13 = sodium.CRYPTO_PWHASH_ALG_ARGON2ID13;
    this.AEAD_XCHACHA20POLY1305_IETF_NPUBBYTES = sodium.CRYPTO_AEAD_XCHACHA20POLY1305_IETF_NPUBBYTES;
    this.XCHACHA20POLY1305_OVERHEAD = sodium.CRYPTO_AEAD_XCHACHA20POLY1305_IETF_NPUBBYTES + sodium.CRYPTO_AEAD_XCHACHA20POLY1305_IETF_ABYTES;

    this.secretbox_keygen = async () => sodium.crypto_secretbox_keygen().then(k => k.getBuffer());
    this.randombytes = async (n) => sodium.randombytes_buf(n);
    this.secretbox = async (data, nonce, key) => sodium.crypto_secretbox(new Uint8Array(data), nonce, await sodiumKey(key));
    this.secretbox_open = async (data, nonce, key) => sodium.crypto_secretbox_open(data, nonce, await sodiumKey(key));
    this.pwhash = sodium.crypto_pwhash.bind(sodium);
    this.hex2bin = async (hex) => sodium.sodium_hex2bin(hex);
    this.box_keypair = async () => {
      const kp = await sodium.crypto_box_keypair();
      return {
        sk: (await sodium.crypto_box_secretkey(kp)).getBuffer(),
        pk: (await sodium.crypto_box_publickey(kp)).getBuffer(),
      };
    };
    this.box_publickey_from_secretkey = async (sk) => sodium.crypto_box_publickey_from_secretkey(await sodiumSecretKey(sk)).then(k => k.getBuffer());
    this.box_seal = async (data, pk) => sodium.crypto_box_seal(new Uint8Array(data), await sodiumPublicKey(pk));
    this.box_seal_open = async (data, pk, sk) => sodium.crypto_box_seal_open(new Uint8Array(data), await sodiumPublicKey(pk), await sodiumSecretKey(sk));
    this.box = async (data, nonce, sk, pk) => sodium.crypto_box(data, nonce, await sodiumSecretKey(sk), await sodiumPublicKey(pk));
    this.box_open = async (data, nonce, sk, pk) => sodium.crypto_box_open(data, nonce, await sodiumSecretKey(sk), await sodiumPublicKey(pk));

    this.kdf_derive_from_key = async (sz, id, ctx, key) => sodium.crypto_kdf_derive_from_key(sz, id, ctx, await sodiumKey(key));
    this.aead_xchacha20poly1305_ietf_decrypt = sodium.crypto_aead_xchacha20poly1305_ietf_decrypt.bind(sodium);
    this.aead_xchacha20poly1305_ietf_encrypt = sodium.crypto_aead_xchacha20poly1305_ietf_encrypt.bind(sodium);
  }
}
