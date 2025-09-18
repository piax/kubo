#!/bin/sh

IPFS=cmd/ipfs/ipfs
REPO_DIR=$HOME/.ipfs
AUTH_PUB_KEY=BABBEIIDVUCM6VGALPTCTHLCIK2GA6FQNVCJFGQODVSO6MISXBZXQ2CQIXDA
CERT_FILE=pcert
BOOTSTRAP=/ip4/127.0.0.1/udp/4001/quic-v1/p2p/12D3KooWJ32FL9UhS6PFtBnuUPJrivuns5EtP7GCx2HBCfTjQSkh

# run initialization
echo "running initialization"
$IPFS --repo-dir $REPO_DIR init

# setup bsdht
echo "setting bsdht"
$IPFS --repo-dir $REPO_DIR config --json Routing "{
    \"Type\": \"custom\",
    \"Routers\": {
      \"bsdht-router\": {
        \"Type\": \"bsdht\",
        \"Parameters\": {
          \"AuthPubKey\": \"$AUTH_PUB_KEY\",
          \"CertFile\": \"$CERT_FILE\"
        }
      }
    },
    \"Methods\": {
      \"find-peers\": {
        \"RouterName\": \"bsdht-router\"
      },
      \"find-providers\": {
        \"RouterName\": \"bsdht-router\"
      },
      \"get-ipns\": {
        \"RouterName\": \"bsdht-router\"
      },
      \"provide\": {
        \"RouterName\": \"bsdht-router\"
      },
      \"put-ipns\": {
        \"RouterName\": \"bsdht-router\"
      }
    }
  }"

# setup bootstrap
echo "setting bootstrap"
$IPFS --repo-dir $REPO_DIR config --json Bootstrap "[\"$BOOTSTRAP\"]"

# BSDHT setting
echo "setting BSDHT"
$IPFS --repo-dir $REPO_DIR config --json Experimental.HRNSEnabled 'true'

# Disable NAT traversal
echo "setting NAT traversal settings"
$IPFS --repo-dir $REPO_DIR config --json Swarm.EnableHolePunching false
$IPFS --repo-dir $REPO_DIR config --json Swarm.RelayClient.Enabled false
$IPFS --repo-dir $REPO_DIR config --json Swarm.RelayService.Enabled false
$IPFS --repo-dir $REPO_DIR config --json AutoNAT.ServiceMode '"disabled"'

# Disable Web Transport and WebRTC
echo "disabling Web Transport and WebRTC"
$IPFS --repo-dir $REPO_DIR config --json Swarm.Transports.Network.WebTransport false
$IPFS --repo-dir $REPO_DIR config --json Swarm.Transports.Network.WebRTCDirect false
$IPFS --repo-dir $REPO_DIR config --json Swarm.Transports.Network.Websocket false
$IPFS --repo-dir $REPO_DIR config --json AutoTLS.Enabled false

echo "disabling NAT port mapping"
$IPFS --repo-dir $REPO_DIR config --json Swarm.DisableNatPortMap true

echo "setting MultiAddress"
$IPFS --repo-dir $REPO_DIR config --json Addresses.Swarm "[\"/ip4/0.0.0.0/tcp/4001\", \"/ip6/::/tcp/4001\", \"/ip4/0.0.0.0/udp/4001/quic-v1\", \"/ip6/::/udp/4001/quic-v1\"]"

echo "Use this PeerID value to get your pcert file."
grep PeerID $REPO_DIR/config
