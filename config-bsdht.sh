#!/bin/sh

# Default values (can be overridden by environment variables)
IPFS=cmd/ipfs/ipfs
REPO_DIR=$HOME/.ipfs
AUTH_PUB_KEY=BABBEIIDVUCM6VGALPTCTHLCIK2GA6FQNVCJFGQODVSO6MISXBZXQ2CQIXDA
CERT_FILE=pcert
BOOTSTRAP=/ip4/127.0.0.1/udp/4001/quic-v1/p2p/12D3KooWJ32FL9UhS6PFtBnuUPJrivuns5EtP7GCx2HBCfTjQSkh

# Override with environment variables if they are set
IPFS=${SET_IPFS:-$IPFS}
REPO_DIR=${SET_REPO_DIR:-$REPO_DIR}
AUTH_PUB_KEY=${SET_AUTH_PUB_KEY:-$AUTH_PUB_KEY}
CERT_FILE=${SET_CERT_FILE:-$CERT_FILE}
BOOTSTRAP=${SET_BOOTSTRAP:-$BOOTSTRAP}

# Display current settings
echo "=== Current Configuration Settings ==="
echo "IPFS: $IPFS"
echo "REPO_DIR: $REPO_DIR"
echo "AUTH_PUB_KEY: $AUTH_PUB_KEY"
echo "CERT_FILE: $CERT_FILE"
echo "BOOTSTRAP: $BOOTSTRAP"
echo "======================================"
echo

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

# Following settings are optional. Disable NAT and WebTransport.
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

echo "setting web ui access"
$IPFS --repo-dir $REPO_DIR config --json API.HTTPHeaders.Access-Control-Allow-Origin '["http://localhost:3000", "http://127.0.0.1:5001", "https://webui.ipfs.io"]'
$IPFS --repo-dir $REPO_DIR config --json API.HTTPHeaders.Access-Control-Allow-Methods '["PUT", "POST"]'

echo "Use this PeerID value to get your pcert file."
grep PeerID $REPO_DIR/config
