#!/bin/bash
# This script runs the browser tests with selenium in docker containers.

cd $(dirname $0)

export GOCACHE=$(go env GOCACHE)
export GOPATH=$(go env GOPATH)
export USERID=$(id -u)
export SRCDIR=$(realpath ..)
export TESTS="$1"
docker-compose -f docker-compose-browser-tests.yaml up \
  --abort-on-container-exit \
  --exit-code-from=devtest
RES=$?
docker-compose -f docker-compose-browser-tests.yaml rm -f

if [[ $RES == 0 ]]; then
  echo PASS
else
  echo FAIL
fi
