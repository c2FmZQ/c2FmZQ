#!/bin/bash

cd "$(dirname $0)"

tmp="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp}"
}
trap cleanup EXIT

cp internal/pwa/.jshintrc internal/pwa/*.js "${tmp}"
for f in "${tmp}"/*.js; do
  sed -r \
    -e 's:#([a-zA-Z]+)\(:_\1(:g' \
    -e 's:this.#([a-zA-Z]+):this._\1:g' \
    -e 's:sw.#([a-zA-Z]+):sw._\1:g' \
    -e 's:#([a-zA-Z]+);://#\1:g' \
    -i "${f}"
done
jshint "${tmp}"/*.js |& sed -re "s:${tmp}/::g"
