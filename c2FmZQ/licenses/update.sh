#!/bin/bash
#
# Use this script to update the licenses.
#
# ./licenses/update.sh

for p in $(grep -E '^[[:space:]]*".+[.].*/.*"' -r --include="*.go" | tr -d '"' | awk '{ print $2 }' | sort -u); do
  echo "$p"
  ~/go/bin/go-licenses save "$p" --force --save_path="licenses/embed/$(echo "$p" | tr '/' '-')"
done
find licenses/embed -type d -exec chmod 755 {} \;
find licenses/embed/ -name .gitignore -exec rm -f {} \;
