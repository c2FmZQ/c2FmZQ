window = self;
const {
  CryptographyKey,
  SodiumPlus,
  X25519PublicKey,
  X25519SecretKey
} = require('sodium-plus');
self.CryptographyKey = CryptographyKey;
self.SodiumPlus = SodiumPlus;
self.X25519PublicKey = X25519PublicKey;
self.X25519SecretKey = X25519SecretKey;
self.bip39 = require('bip39');
self.SecureStore = require('secure-webstore');
