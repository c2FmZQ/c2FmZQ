# Kringle

This repo contains an application that can securely encrypt, store, and share
files, including but not limited to pictures and videos.

There is a command-line client application, and a server application.

The server is the central repository where all the encrypted data can be stored.
It has no access to the plaintext data.

The command-line client is used to import, export, and organize files.

They use an API that's compatible with the Stingle Photos app
(https://github.com/stingle/stingle-photos-android) published by [stingle.org](https://stingle.org).

This project is **not** associated with stingle.org. This is not the code used
by stingle.org. We have no knowledge of how their server is actually implemented.
The code in this repo was developed by studying the client app's code and
reverse engineering the API.

## Notes about security and privacy

**This code has not been reviewed for security.** Review, comments, and
contributions are welcome.

The server has no way to decrypt the files that are uploaded by the clients.
The server knows how many files you have, how big they are, and who they're
shared with.

The client has to trust the server when sharing albums. The server provides
the contact search feature (/v2/sync/getContact), which returns a User ID and 
a public key for the contact. Then the album is shared with that User ID and
public key (via /v2/sync/share).

A malicious server _could_ replace the contact's User ID and public key with
someone else's, and make the user think they're sharing with their friend while
actually sharing with an attacker.

When viewing a shared album, the app / user has to trust that the shared content is
"safe". Since the server can't decrypt the content, it has no way to sanitize it
either. A malicious user _could_ share content that aims to exploit some unpatched
vulnerability in the client's code.

Once an album is shared, there is really no way to completely _unshare_ it. The
permissions on the album can be changed, but it is impossible to control what
happens to the files that were previously shared. They could have been
downloaded, exported, published to the New York Times, etc.

## Kringle Server

Kringle-server is an API server with a relatively small footprint. It can run
just about anywhere, as long as it has access to lots of disk space, a modern
CPU. It must be reachable by the client(s) via HTTPS.

The server needs at two pieces of information: the name of the directory where
its data will be stored, and a passphrase to protect the data. The passphrase
_can_ be stored in a file, otherwise the server will prompt for it when it
starts.

For TLS, the server also needs the TLS key, and certificates.

### Connecting the Stingle app to this server

For the app to connect to this server, it has to the recompiled with api_server_url
set to point to this server.
See [this commit](https://github.com/rthellend/stingle-photos-android/commit/c6758758513f7b9d3cdf755085e4b57945f2494f) for an example.

Note: build the F-Droid version with: gradlew installFdroidRelease

### Scale and performance

The server was designed for personal use, not for large scale or speed. On a
modern CPU and SSD, it scales to 10+ concurrent users with tens of thousands of
files per album, while maintaining a response time well under a second (excluding
network I/O).

On a small device, e.g. a raspberry pi, it scales to a handful of concurrent
users with a few thousand files each if metadata encryption is turned off, and
still maintain a response time ~ 5 seconds depending on storage type.

### How to run the server

The server is self-contained. It doesn't depend on any external resources. It
stores all its data on a local filesystem.

Simply build it, and run it.

```bash
$ cd kringle/kringle-server
$ go build
$ ./kringle-server -h
```

Or, build a docker image.

```bash
$ docker build -t kringle-server .
$ docker run -u ${USER} -v ${DATABASEDIR}:/data -v ${SECRETSDIR}:/secrets:ro kringle-server
```
${DATABASEDIR} is where all the data will be stored, and ${SECRETSDIR} is where the
database encryption passphrase, the TLS key, and TLS cert are stored.

With the default Dockerfile, the server expects the following files in ${SECRETDDIR}:

- **passphrase** contains the passphrase used to encrypt the metadata.
- **privkey.pem** contains the TLS private key in PEM format.
- **fullchain.pem** contains the TLS certificates in PEM format.

Or, build a binary for arm and run the server on a NAS, raspberry pi, etc.

```bash
$ cd kringle/kringle-server
$ GOOS=linux GOARCH=arm go build -o kringle-server-arm
$ scp kringle-server-arm root@NAS:.
```


## Kringle Client

The Kringle client connects to the server. The first time _create-account_ or
_login_ is called, it will prompt the user for the URL of the API server.

Like the server, the client needs to know where to store its data, and it needs
a passphrase to encrypt the data.

To run it:

```bash
$ cd kringle/kringle-client
$ go build
$ ./kringle-client -h
```


