#!/bin/bash

cd "$(dirname $0)"

npm install bip39 secure-webstore sodium-plus

browserify browser.js \
  --exclude=./wordlists/japanese.json  \
  --exclude=./wordlists/spanish.json  \
  --exclude=./wordlists/italian.json \
  --exclude=./wordlists/french.json \
  --exclude=./wordlists/korean.json \
  --exclude=./wordlists/czech.json \
  --exclude=./wordlists/portuguese.json \
  --exclude=./wordlists/chinese_simplified.json \
  --exclude=./wordlists/chinese_traditional.json  > libs.js
