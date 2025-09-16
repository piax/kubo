# kubo with ByzSkip-DHT extension

## Build

```bash
$ pushd cmd/ipfs
$ go build
$ popd
```

## Edit config-bsdht.sh

At least you have to edit following variables.

`IPFS`: the location of ipfs binary (default is cmd/ipfs/ipfs)
`REPO_DIR`: the repository directory (default is $HOME/.ipfs)
`AUTH_PUB_KEY`: the public key string of the authority
`CERT_FILE`: the name of the cert file (default is $REPO_DIR/pcert)
`BOOTSTRAP`: the multiaddr of the bootstrap node (only one bootstrap can be specified)

## Run config-bsdht.sh

```bash
$ sh config-bsdht.sh
```

## Get participation certificate.

Access authority and get your own certificate data. At this time, you need the PeerID, which is printed by executing `config-bsdht.sh`.
You need to save the certificate data in the file `CERT_FILE`.


