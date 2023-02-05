#!/bin/bash

cd "$(dirname $0)"

npm install bip39 sodium-plus exifreader

browserify worker.js \
  --exclude=./wordlists/japanese.json  \
  --exclude=./wordlists/spanish.json  \
  --exclude=./wordlists/italian.json \
  --exclude=./wordlists/french.json \
  --exclude=./wordlists/korean.json \
  --exclude=./wordlists/czech.json \
  --exclude=./wordlists/portuguese.json \
  --exclude=./wordlists/chinese_simplified.json \
  --exclude=./wordlists/chinese_traditional.json  > libs.js

browserify browser.js > browser-libs.js
