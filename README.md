# Kringle Server

Kringle-server is an API server that's compatible with the Stingle Photos app
(https://github.com/stingle/stingle-photos-android) published by [stingle.org](https://stingle.org).

This project is **not** associated with stingle.org. This is not the code used
by stingle.org. We have no knowledge of how their server is actually implemented.
The code in this repo was developed by studying the client app's code and
reverse engineeing the API.

## Connecting the app to this server

For the app to connect to this server, it has to the recompiled with api_server_url
set to point to this server.
See https://github.com/rthellend/stingle-photos-android/commit/87ffc310b9fd02d4aba52a2fb4d12cb392208904
for an example.

## Notes about security and privacy

The server has no way to decrypt the photos and videos that are uploaded by
the app. The server knows how many files you have, how big they are, and who
you're sharing them with.

The app has to trust the server when sharing albums. The server provides
the contact search feature (/v2/sync/getContact), which returns a User ID and 
a public key for the contact. Then the album is shared with that User ID and
public key (via /v2/sync/share).

A malicious server _could_ replace the contact's User ID and public key with
someone else's, and make the user think they're sharing with their friend while
actually sharing with an attacker.

## How to run the server

The server doesn't depend on an external database. It stores all its data on a local filesystem.

Simply build it, and run it.

```bash
$ cd kringle-server
$ go build
$ ./kringle-server -h
```

Or, build a docker image.

```bash
$ docker build -t kringle-server .
$ docker run -v /data:${DATABASEDIR} -v /secrets:${SECRETSDIR} kringle-server
```
${DATABASEDIR} is where all the data will be stored, and ${SECRETSDIR} is where the
database encryption passphrase, the TLS key, and TLS cert are stored.
