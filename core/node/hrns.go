package node

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core/node/libp2p"
	"github.com/ipfs/kubo/repo"
	madns "github.com/multiformats/go-multiaddr-dns"
	apb "github.com/piax/go-byzskip/ayame/p2p/pb"
	bsdht "github.com/piax/go-byzskip/dht"
	"go.uber.org/fx"
	"google.golang.org/protobuf/proto"
)

// Human-Readable Name System
// /hrns/<domain-name>:<peer-id>
const VALIDITY_PERIOD = 24 * time.Hour
const INITIAL_REPUBLISH_DELAY = 1 * time.Minute
const DEFAULT_REPUBLISH_PERIOD = 1 * time.Minute //1 * time.Hour

// HRNSRepublisher runs a service to republish HRNS records at specified intervals
func HRNSRepublisher(repubPeriod time.Duration) func(lc fx.Lifecycle, dht *bsdht.BSDHT, repo repo.Repo) error {
	return func(lc fx.Lifecycle, dht *bsdht.BSDHT, repo repo.Repo) error {
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				go func() {

					timer := time.NewTimer(INITIAL_REPUBLISH_DELAY)
					defer timer.Stop()
					if repubPeriod < INITIAL_REPUBLISH_DELAY {
						timer.Reset(repubPeriod)
					}

					for {
						select {
						case <-timer.C:
							timer.Reset(repubPeriod)
							err := RepublishHRNSRecords(ctx, repo.Datastore(), dht)
							if err != nil {
								logger.Errorf("failed to republish records: %v", err)
							}
						case <-ctx.Done():
							return
						}
					}
				}()
				return nil
			},
			OnStop: func(ctx context.Context) error {
				// closethe repo and remove repo.lock file
				if closer, ok := repo.(interface{ Close() error }); ok {
					return closer.Close()
				}
				return nil
			},
		})
		return nil
	}
}

func getRecordWithPrefixFromDatastore(ctx context.Context, datastore ds.Datastore, key string) ([]*apb.Record, error) {
	q := query.Query{Filters: []query.Filter{query.FilterKeyPrefix{Prefix: ds.NewKey(key).String()}}}
	results, err := datastore.Query(ctx, q)
	//results, err := dht.datastore.Get(ctx, ds.NewKey(key))
	if err != nil {
		return nil, err
	}
	es, _ := results.Rest()
	logger.Debugf("getRecordWithPrefixFromDatastore %s, length=%d", key, len(es))

	ret := []*apb.Record{}
	for _, e := range es {
		rec := new(apb.Record)
		err = proto.Unmarshal(e.Value, rec)
		if err != nil {
			// Bad data in datastore, log it but don't return an error, we'll just overwrite it
			return nil, err
		}
		// will be validated by dht.PutNamedValue
		//err = dht.RecordValidator.Validate(string(rec.GetKey()), rec.GetValue())
		//if err != nil {
		//	return nil, err
		//}
		ret = append(ret, rec)
	}
	return ret, nil
}

func RepublishHRNSRecords(ctx context.Context, datastore ds.Datastore, dht *bsdht.BSDHT) error {
	// Query all keys with prefix /hrns/
	entries, err := getRecordWithPrefixFromDatastore(ctx, datastore, "/hrns")
	if err != nil {
		return fmt.Errorf("error processing result: %v", err)
	}
	logger.Infof("republish hrns %d records", len(entries))
	for _, r := range entries {

		//var record pb.Record
		//if err := proto.Unmarshal(r.Value, &record); err != nil {
		//	log.Errorf("failed to unmarshal record: %v", err)
		//		continue
		//	}

		namedValue, err := bsdht.CheckHRNSRecord(r.Value)
		if err != nil {
			logger.Infof("record %s is not valid: %s", r.Key, err)
			return err
		}

		str, err := url.QueryUnescape(string(r.Key))
		if err != nil {
			str = string(r.Key)
		}

		key, err := bsdht.ParseName(str)
		name := string(key.Value()[:bytes.IndexByte(key.Value(), 0)])

		if name != dht.Node.Name() {
			logger.Debugf("registered name (%s) is not mine(%s)", name, dht.Node.Name())
			continue
		}

		value := namedValue.Value

		if err == nil {
			logger.Infof("republishing record %s", str)
			err := dht.PutNamedValue(ctx, name, value)
			if err != nil {
				logger.Errorf("failed to republish record %s: %v", str, err)
			}
		} else {
			logger.Infof("record %s is not valid: %s", str, err)
			err := DeleteHRNSRecord(ctx, datastore, str)
			if err != nil {
				logger.Errorf("failed to delete record %s: %v", str, err)
			}
		}
	}
	return nil
}

func DeleteHRNSRecord(ctx context.Context, datastore ds.Datastore, key string) error {
	if err := datastore.Delete(ctx, ds.NewKey(key)); err != nil {
		return fmt.Errorf("failed to delete record: %v", err)
	}
	return nil
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
