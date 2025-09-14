package node

import (
	"context"
	"math"
	"net"
	"time"

	"github.com/ipfs/boxo/gateway"
	config "github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core/node/libp2p"
	doh "github.com/libp2p/go-doh-resolver"
	madns "github.com/multiformats/go-multiaddr-dns"
)

func DNSResolver(cfg *config.Config) (*madns.Resolver, error) {
	var dohOpts []doh.Option
	if !cfg.DNS.MaxCacheTTL.IsDefault() {
		dohOpts = append(dohOpts, doh.WithMaxCacheTTL(cfg.DNS.MaxCacheTTL.WithDefault(time.Duration(math.MaxUint32)*time.Second)))
	}

	// Replace "auto" DNS resolver placeholders with autoconf values
	resolvers := cfg.DNSResolversWithAutoConf()

	return gateway.NewDNSResolver(resolvers, dohOpts...)
}

// switchingBasicResolver routes BasicResolver calls to BSDHT when available, otherwise to fallback
// It implements madns.BasicResolver fully (LookupIPAddr and LookupTXT).
type switchingBasicResolver struct {
	fallback madns.BasicResolver
}

func (s *switchingBasicResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	// Check if BSDHT is available at runtime
	bs := libp2p.GetGlobalBSDHT()
	if bs != nil {
		return bs.(madns.BasicResolver).LookupIPAddr(ctx, host)
	}

	return s.fallback.LookupIPAddr(ctx, host)
}

func (s *switchingBasicResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	// Check if BSDHT is available at runtime
	bs := libp2p.GetGlobalBSDHT()
	if bs != nil {
		return bs.(madns.BasicResolver).LookupTXT(ctx, name)
	}
	return s.fallback.LookupTXT(ctx, name)
}

// NewBSDHTDNSResolver creates a *madns.Resolver that uses BSDHT for BasicResolver methods once available.
func NewBSDHTDNSResolver(cfg *config.Config) (*madns.Resolver, error) {
	// If HRNS is disabled, use the standard DNS resolver
	if !cfg.Experimental.HRNSEnabled {
		return DNSResolver(cfg)
	}

	// Build the fallback resolver using standard gateway configuration (DoH etc.).
	fallbackRes, err := DNSResolver(cfg)
	if err != nil {
		return nil, err
	}

	// Create a BasicResolver that checks BSDHT availability at runtime
	switcher := &switchingBasicResolver{fallback: fallbackRes}

	// Create a new Resolver that uses the switcher as the default BasicResolver.
	res, err := madns.NewResolver(madns.WithDefaultResolver(switcher))
	if err != nil {
		return nil, err
	}

	return res, nil
}

// BSDHTDNSResolverProvider provides the resolver instance for Fx.
func BSDHTDNSResolverProvider(cfg *config.Config) (*madns.Resolver, error) {
	return NewBSDHTDNSResolver(cfg)
}
