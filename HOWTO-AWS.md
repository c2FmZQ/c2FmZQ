# HOW TO RUN THE C2FMZQ SERVER ON AWS

The server can easily run on a EC2 micro instance (1 vCPU, 1 GB RAM), which may
qualify for the [AWS Free Tier](https://aws.amazon.com/free).

It is your responsibility to check free tier eligibility and/or pay for the
resources you use.

## The first step is to create a AWS Account, and then create an EC2 instance.

* Select `t2.micro` as instance type.
* Select `ubuntu 20.04` as operating system.
* Select 30 GB of EBS storage.
* Under security, open TCP ports 22 (SSH), 80 (HTTP), and 443 (HTTPS).

Once the instance is running, make sure you can connect to it with SSH.

```
ssh -i <keyfile> ubuntu@<instance-IP>
```

## Then create a DNS entry for the instance's IP address.

You can't use the amazonaws.com hostname because letsencrypt.org won't issue
certificates for that domain. You need to use your own domain, or ask a friend
to create a DNS entry for you.

The c2FmZQ server will only respond to requests that use the hostname that you
specify.

## Install docker.

Login to the instance with ssh.

Install docker:

```
sudo apt-get update
sudo apt-get install docker.io
```

## Build the docker image.

Pull the source code from github and build the docker image.

```
mkdir -p ~/src
cd ~/src
git clone https://github.com/c2FmZQ/c2FmZQ.git
sudo docker build -t c2fmzq-server ~/src/c2FmZQ/
```

## Create passphrase file.

Create a file that contains the passphrase used to encrypt the server's master
key. It's best to keep it in a in-memory filesystem.

```
mkdir -m 700 /dev/shm/c2fmzq
vi /dev/shm/c2fmzq/passphrase
```

Note that if the VM reboots, you will need to recreate the passphrase file.

## Start the docker container.

```
DOMAIN="<YOUR DOMAIN OR HOST NAME>"

sudo docker run \
  --detach \
  --env=C2FMZQ_DOMAIN="$DOMAIN" \
  --name=c2fmzq-server \
  --network=host \
  --volume=$HOME/data:/data \
  --volume=/dev/shm/c2fmzq:/secrets:ro c2fmzq-server
```

## Check the server logs.

```
sudo docker logs -f c2fmzq-server
```

## Show users, set quotas, etc.

The `inspect` command can be used inside the docker container to perform administrative tasks.

```
sudo docker exec -it c2fmzq-server inspect
sudo docker exec -it c2fmzq-server inspect users
sudo docker exec -it c2fmzq-server inspect quotas
```
