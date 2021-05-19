#!/bin/bash
#
# Use this script to update the licenses.
#
# ./licenses/update.sh

for p in $(grep -E '^[[:space:]]*".+[.].*/.*"' -r | tr -d '"' | awk '{ print $2 }' | sort -u); do
  echo "$p"
  ~/go/bin/go-licenses save "$p" --force --save_path="licenses/$(echo "$p" | tr '/' '-')"
done
