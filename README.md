`echo -n safe | base64`

# c2FmZQ

* [Overview](#overview)
* [Notes about security and privacy](#security)
* [c2FmZQ Server](#c2FmZQ-server)
  * [Connecting the Stingle Photos app to this server](#stingle)
  * [Scale and performance](#scale)
  * [How to run the server](#run-server)
* [c2FmZQ Client](#c2FmZQ-client)
  * [Mount as fuse filesystem](#fuse)
  * [Connect to stingle.org account](#connect-to-stingle)

# <a name="overview"></a>Overview

This repo contains an application that can securely encrypt, store, and share
files, including but not limited to pictures and videos.

There is a command-line client application, and a server application.

The server is the central repository where all the encrypted data can be stored.
It has no way to access to the plaintext data.

The command-line client is used to import, export, organize, and share files.

They use an API that's compatible with the Stingle Photos app
(https://github.com/stingle/stingle-photos-android) published by [stingle.org](https://stingle.org).

This project is **NOT** associated with stingle.org. This is not the code used
by stingle.org. We have no knowledge of how their server is actually implemented.
The code in this repo was developed by studying the client app's code and
reverse engineering the API.

---

# <a name="security"></a>Notes about security and privacy

**This software has not been reviewed for security.** Review, comments, and
contributions are welcome.

The server has no way to decrypt the files that are uploaded by the clients.
It knows how many files you have, how big they are, and who they're
shared with.

The client has to trust the server when sharing albums. The server provides
the contact search feature (/v2/sync/getContact), which returns a User ID and 
a public key for the contact. Then, the album is shared with that User ID and
public key (via /v2/sync/share).

A malicious server _could_ replace the contact's User ID and public key with
someone else's, and make the user think they're sharing with their friend while
actually sharing with an attacker. The client application lets the user verify
the contact's public key before sharing.

When viewing a shared album, the client has to trust that the shared content is
"safe". Since the server can't decrypt the content, it has no way to sanitize it
either. A malicious user _could_ share content that aims to exploit some unpatched
vulnerability in the client's code.

Once an album is shared, there is really no way to completely _unshare_ it. The
permissions on the album can be changed, but it is impossible to control what
happens to the files that were previously shared. They could have been downloaded,
exported, published to the New York Times, etc.

Since c2FmZQ is compatible with the Stingle Photos API, it uses the
[same cryptographic algorithms](https://stingle.org/security/) for authentication,
client-server communication, and file encryption, namely:

* [Argon2](https://en.wikipedia.org/wiki/Argon2) for password key derivation on the
client side; [bcrypt](https://en.wikipedia.org/wiki/Bcrypt) on the server side,
* [NaCl](https://en.wikipedia.org/wiki/NaCl_(software)) (Curve25519/XSalsa20/Poly1305)
for client-server authentication and encryption,
* [Chacha20+Poly1305](https://ieeexplore.ieee.org/document/7927078) and
[Blake2b](https://en.wikipedia.org/wiki/BLAKE_(hash_function)#BLAKE2) for file
encryption and key derivation.

Additionally, it uses [AES256-GCM](https://en.wikipedia.org/wiki/Galois/Counter_Mode) and
[AES256-CBC](https://en.wikipedia.org/wiki/Block_cipher_mode_of_operation#CBC) with
[HMAC-SHA256](https://en.wikipedia.org/wiki/HMAC) to encrypt its own metadata, and
[PBKDF2](https://en.wikipedia.org/wiki/PBKDF2) for the passphrase key derivation.

---

# <a name="c2FmZQ-server"></a>c2FmZQ Server

c2FmZQ-server is an API server with a relatively small footprint. It can run
just about anywhere, as long as it has access to a lot of storage space, and a modern
CPU. It must be reachable by the client(s) via HTTPS.

The server needs at least two pieces of information: the name of the directory where
its data will be stored, and a passphrase to protect the data. The passphrase
can be read from a file or retrieved with an external command, otherwise the server
will prompt for it when it starts.

For TLS, the server also needs the TLS key, and certificates. They can be read from
files, or directly from letsencrypt.org.

---

## <a name="stingle"></a>Connecting the Stingle Photos app to this server

For the Stingle Photos app to connect to this server, it has to the recompiled to allow `api_server_url`
to point to this server.
See the [Stingle Photos App for Self-Hosted Server](https://github.com/c2FmZQ/stingle-photos-for-self-hosted-server) ([diff](https://github.com/stingle/stingle-photos-android/compare/master...c2FmZQ:master?diff=split)).

Note: build the F-Droid version with: `gradlew installFdroidRelease`

---

## <a name="scale"></a>Scale and performance

The server was designed for personal use, not for large scale deployment or speed.
On a modern CPU and SSD, it scales to 10+ concurrent users with tens of thousands of
files per album, while maintaining a response time well under a second (excluding
network I/O).

On a small device, e.g. a raspberry pi, it scales to a handful of concurrent
users with a few thousand files per album, and still maintain an acceptable response time.

---

## <a name="run-server"></a>How to run the server

The server is self-contained. It doesn't depend on any external resources. It
stores all its data on a local filesystem.

It can run on AWS [Howto](HOWTO-AWS.md) or any other cloud providers. It can run
in a docker container. It can run on Linux, MacOS, Windows. It can run on a
raspberry pi, or on a NAS. It can run pretty much on anything that has at least
1 GB of RAM.
--- 

### Build it, and run it locally

```bash
cd c2FmZQ/c2FmZQ-server
go build
./c2FmZQ-server help
```
```txt
NAME:
   c2FmZQ-server - Run the c2FmZQ server

USAGE:
   c2FmZQ-server command [command options]  

COMMANDS:

OPTIONS:
   --database DIR, --db DIR       Use the database in DIR (default: "$HOME/c2FmZQ-server/data") [$C2FMZQ_DATABASE]
   --address value, --addr value  The local address to use. (default: "127.0.0.1:8080")
   --path-prefix value            The API endpoints are <path-prefix>/v2/...
   --base-url value               The base URL of the generated download links. If empty, the links will generated using the Host headers of the incoming requests, i.e. https://HOST/.
   --redirect-404 value           Requests to unknown endpoints are redirected to this URL.
   --tlscert FILE                 The name of the FILE containing the TLS cert to use.
   --tlskey FILE                  The name of the FILE containing the TLS private key to use.
   --autocert-domain domain       Use autocert (letsencrypt.org) to get TLS credentials for this domain. The special value 'any' means accept any domain. The credentials are saved in the database. [$C2FMZQ_DOMAIN]
   --autocert-address value       The autocert http server will listen on this address. It must be reachable externally on port 80. (default: ":http")
   --allow-new-accounts           Allow new account registrations. (default: true)
   --verbose value, -v value      The level of logging verbosity: 1:Error 2:Info 3:Debug (default: 2 (info))
   --encrypt-metadata             Encrypt the server metadata (strongly recommended). (default: true)
   --passphrase-command COMMAND   Read the database passphrase from the standard output of COMMAND. [$C2FMZQ_PASSPHRASE_CMD]
   --passphrase-file FILE         Read the database passphrase from FILE. [$C2FMZQ_PASSPHRASE_FILE]
   --htdigest-file FILE           The name of the htdigest FILE to use for basic auth for some endpoints, e.g. /metrics [$C2FMZQ_HTDIGEST_FILE]
   --licenses                     Show the software licenses. (default: false)
```

---

### Or, build a docker image

```bash
docker build -t c2fmzq-server .
docker run -u ${USER} -e C2FMZQ_DOMAIN=${DOMAIN} -v ${DATABASEDIR}:/data -v ${SECRETSDIR}:/secrets:ro c2fmzq-server
```
With the default Dockerfile, TLS credentials are fetched from [letsencrypt.org](https://letsencrypt.org)
automatically.

`${DATABASEDIR}` is where all the encrypted data will be stored, `${SECRETSDIR}`
is where the database encryption passphrase is stored (`${SECRETSDIR}/passphrase`),
and `${DOMAIN}` is the domain or hostname to use.

The domain or hostname must resolve to the IP address where the server will be running,
and firwall and/or port forwarding rules must be in place to allow TCP connections to
ports 80 and 443. A dynamic hostname is fine (DDNS, Dynamic DNS, ...). The clients will
connect to `https://${DOMAIN}/`.

---

### Or, build a binary for another platform, e.g. windows, raspberry pi, or a NAS

```bash
cd c2FmZQ/c2FmZQ-server
GOOS=windows GOARCH=amd64 go build -o c2FmZQ-server.exe
GOOS=linux GOARCH=arm go build -o c2FmZQ-server-arm
GOOS=darwin GOARCH=arm64 go build -o c2FmZQ-server-darwin
```

---

# <a name="c2FmZQ-client"></a>c2FmZQ Client

The c2FmZQ client can be used by itself, or with a remote ("cloud") server very
similarly.

Sharing only works when content is synced with a remote server.

To connect to a remote server, the user will need to provide the URL of the
server when _create-account_, _login_, or _recover-account_ is used.

To run it:

```bash
cd c2FmZQ/c2FmZQ-client
go build
./c2FmZQ-client
```
```txt
NAME:
   c2FmZQ - Keep your files away from prying eyes.

USAGE:
   c2FmZQ-client [global options] command [command options] [arguments...]

COMMANDS:
   Account:
     backup-phrase    Show the backup phrase for the current account. The backup phrase must be kept secret.
     change-password  Change the user's password.
     create-account   Create an account.
     delete-account   Delete the account and wipe all data.
     login            Login to an account.
     logout           Logout.
     recover-account  Recover an account with backup phrase.
     set-key-backup   Enable or disable secret key backup.
     status           Show the client's status.
     wipe-account     Wipe all local files associated with the current account.
   Albums:
     create-album, mkdir  Create new directory (album).
     delete-album, rmdir  Remove a directory (album).
     rename               Rename a directory (album).
   Files:
     cat, show           Decrypt files and send their content to standard output.
     copy, cp            Copy files to a different directory.
     delete, rm, remove  Delete files (move them to trash, or delete them from trash).
     list, ls            List files and directories.
     move, mv            Move files to a different directory, or rename a directory.
   Import/Export:
     export  Decrypt and export files.
     import  Encrypt and import files.
   Misc:
     licenses  Show the software licenses.
   Mode:
     mount  Mount as a fuse filesystem.
     shell  Run in shell mode.
   Share:
     change-permissions, chmod  Change the permissions on a shared directory (album).
     contacts                   List contacts.
     leave                      Remove a directory (album) that is shared with us.
     remove-member              Remove members from a directory (album).
     share                      Share a directory (album) with other people.
     unshare                    Stop sharing a directory (album).
   Sync:
     download, pull   Download a local copy of encrypted files.
     free             Remove the local copy of encrypted files that are backed up.
     sync             Upload changes to remote server.
     updates, update  Pull metadata updates from remote server.

GLOBAL OPTIONS:
   --data-dir DIR, -d DIR     Save the data in DIR (default: "$HOME/.config/.c2FmZQ") [$C2FMZQ_DATADIR]
   --verbose value, -v value  The level of logging verbosity: 1:Error 2:Info 3:Debug (default: 2 (info))
   --passphrase-file FILE     Read the database passphrase from FILE. [$C2FMZQ_PASSPHRASE_FILE]
   --server value             The API server base URL. [$C2FMZQ_API_SERVER]
   --auto-update              Automatically fetch metadata updates from the remote server before each command. (default: true)
```

---

## <a name="fuse"></a>Mount as fuse filesystem

The c2FmZQ client can mount itself as a fuse filesystem. It supports read and
write operations with some caveats.

* Files can only be opened for writing when they are created, and all writes must
  append. The file content is encrypted as it is written.
* Once a new file is closed, it is read-only (regardless of file permissions).
  The only way to modify a file after that is to delete it or replace it. Renames
  are OK.
* While the fuse filesystem is mounted, data isn't automatically uploaded to the
  cloud/remote server, but remote content will be streamed for reading if a local
  copy doesn't exist.

```bash
mkdir -p 0700 /dev/shm/$USER
echo -n "<INSERT DATABASE PASSPHRASE HERE>" > /dev/shm/$USER/.c2fmzq-passphrase
export C2FMZQ_PASSPHRASE_FILE=/dev/shm/$USER/.c2fmzq-passphrase
```
```bash
mkdir $HOME/mnt
./c2FmZQ-client mount $HOME/mnt
```
```txt
I0604 144921.460 fuse/fuse.go:43] Mounted $HOME/mnt
```

Open a different terminal. You can now access all your files. They will be decrypted on demand as they are read.
```bash
ls -a $HOME/mnt
```
```txt
gallery  .trash
```
Bulk copy in and out of the fuse filesystem should work as expected with:

* cp, cp -r, mv
* tar
* rsync, with --no-times

When you're done, hit `CTRL-C` where the `mount` command is running to close and unmount the fuse filesystem.

---

## <a name="connect-to-stingle"></a>Connect to stingle.org account

To connect to your stingle.org account, `--server=https://api.stingle.org/` with _login_ or _recover-account_.

```bash
mkdir -p 0700 /dev/shm/$USER
echo -n "<INSERT DATABASE PASSPHRASE HERE>" > /dev/shm/$USER/.c2fmzq-passphrase
export C2FMZQ_PASSPHRASE_FILE=/dev/shm/$USER/.c2fmzq-passphrase
```
```bash
./c2FmZQ-client --server=https://api.stingle.org/ login <email>
```
```txt
Enter password: 
Logged in successfully.
```
```bash
./c2FmZQ-client ls -a
```
```txt
.trash/
gallery/
```
