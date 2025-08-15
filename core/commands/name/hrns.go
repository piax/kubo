package name

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/ipfs/boxo/path"
	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/kubo/client/rpc"
	"github.com/ipfs/kubo/core/commands/cmdenv"
	"github.com/ipfs/kubo/repo/fsrepo"
	"github.com/libp2p/go-libp2p/core/peer"
	madns "github.com/multiformats/go-multiaddr-dns"
	"github.com/piax/go-byzskip/authority"
	ayame "github.com/piax/go-byzskip/ayame/p2p"
)

var HrnsCmd = &cmds.Command{
	Status: cmds.Experimental,
	Helptext: cmds.HelpText{
		Tagline: "Human-Readable Name System management commands",
		ShortDescription: `
Manage and inspect the state of the HRNS.
Note: this command is experimental and subject to change.
`,
	},

	Subcommands: map[string]*cmds.Command{
		"show":     ShowCmd,
		"search":   QueryCmd,
		"register": RegisterNameCmd,
		"peers":    PeersCmd,
		"trust":    TrustCmd,
	},
}

type BsnsEntry struct {
	Name        string
	Value       string
	Error       string
	ValidBefore int64
}

type BsnsEntries struct {
	Entries []BsnsEntry
	Env     cmds.Environment
}

type BsnsShowEntry struct {
	Name        string
	Value       string
	Addr        string
	Key         string
	ValidAfter  int64
	ValidBefore int64
}

// Define an interface for the API client to avoid direct dependency
type ApiClient interface {
	Request(command string, args ...string) interface{}
}

// Define an interface for the request builder
type RequestBuilder interface {
	Option(key string, value interface{})
	Exec(ctx context.Context, result interface{}) error
}

func executeBsnsCommand(ctx context.Context, api *rpc.HttpApi, command string, args ...string) ([]string, error) {
	// Build the request
	req := api.Request(command, args...)

	// Execute the request and process the response
	var result struct {
		Entries []struct {
			Name        string `json:"Name"`
			Value       string `json:"Value"`
			Error       string `json:"Error,omitempty"`
			ValidBefore int64  `json:"ValidBefore"`
		} `json:"Entries"`
	}

	err := req.Exec(ctx, &result)
	if err != nil {
		fmt.Printf("Error executing command: %v\n", err)
		return nil, err
	}

	entries := make([]string, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = e.Name
	}

	return entries, nil
}

// getRepoPath returns the repository path from the command options or the best known path
func getRepoPath(req *cmds.Request) (string, error) {
	if req != nil {
		if repoOpt, found := req.Options["repo-dir"].(string); found && repoOpt != "" {
			return repoOpt, nil
		}
	}
	return fsrepo.BestKnownPath()
}

//func getRepoPath(req *cmds.Request) (string, error) {
//	return cmdutils.GetRepoPath(req)
//}

func runCommand(api *rpc.HttpApi, command string, args ...string) ([]string, error) {
	ctx := context.Background()
	entries, err := executeBsnsCommand(ctx, api, command, args...)
	if err != nil {
		fmt.Printf("Error executing command: %v\n", err)
		return nil, err
	}
	return entries, nil
}

var ShowCmd = &cmds.Command{
	Status: cmds.Experimental,
	Helptext: cmds.HelpText{
		Tagline:          "Show the state of the HRNS.",
		ShortDescription: "Show the state of the HRNS.",
	},
	Options: []cmds.Option{
		cmds.BoolOption("verbose", "v", "Print extra information."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		nd, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}
		name := nd.BSDHT.Node.Name()
		id := nd.BSDHT.Node.Id()
		addr := nd.PeerHost.Addrs()
		key := nd.BSDHT.Node.Key()
		cbytes := nd.BSDHT.Node.Parent.(*ayame.P2PNode).Cert
		_, va, vb, err := authority.ExtractCert(cbytes)

		cmds.EmitOnce(res, &BsnsShowEntry{
			Name:        name,
			Value:       id.String(),
			Addr:        addr[0].String(),
			Key:         key.String(),
			ValidAfter:  va,
			ValidBefore: vb,
		})

		return err
	},

	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, ie *BsnsShowEntry) error {
			_, err := fmt.Fprintf(w, "Name: %s\nID: %s\nAddr: %s\nKey: %s\nValidAfter: %s\nValidBefore: %s\n",
				cmdenv.EscNonPrint(ie.Name), cmdenv.EscNonPrint(ie.Value),
				cmdenv.EscNonPrint(ie.Addr), cmdenv.EscNonPrint(ie.Key),
				time.Unix(ie.ValidAfter, 0).Format(time.RFC3339),
				time.Unix(ie.ValidBefore, 0).Format(time.RFC3339))
			return err
		}),
	},
	Type: BsnsShowEntry{},
}

var RegisterNameCmd = &cmds.Command{
	Status: cmds.Experimental,
	Helptext: cmds.HelpText{
		Tagline:          "Register names.",
		ShortDescription: "Register arbitrary string as a name for the specified ipfs path.",
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("value", true, false, "The ipfs path of object to be registered."),
	},
	Options: []cmds.Option{
		cmds.BoolOption("verbose", "v", "Print extra information."),
		cmds.BoolOption("dryrun", "n", "Dry run."),
		cmds.StringOption("lifetime", "t",
			`Time duration that the record will be valid for. <<default>>
This accepts durations such as "300s", "1.5h" or "2h45m". Valid time units are
"ns", "us" (or "µs"), "ms", "s", "m", "h".`).WithDefault("24h"),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		nd, err := cmdenv.GetNode(env)
		if err != nil {
			return cmds.Errorf(cmds.ErrClient, "Error getting node: %s", err)
		}

		if nd.BSDHT == nil {
			return cmds.Errorf(cmds.ErrClient, "routing service is not a RQDHT")
		}

		madns.DefaultResolver = &nd.BSDHT.Resolver
		/* XXX Not implemented yet.
		validTimeOpt, _ := req.Options["lifetime"].(string)
		validTime, err := time.ParseDuration(validTimeOpt)
		if err != nil {
			return cmds.Errorf(cmds.ErrClient, "error parsing lifetime option: %s", err)
		}

		opts := []options.NamePublishOption{
			options.Name.ValidTime(validTime),
		}
		*/
		p, err := path.NewPath(req.Arguments[0])
		if err != nil {
			return cmds.Errorf(cmds.ErrClient, "Error parsing path: %s", err)
		}

		ctx, cancel := context.WithCancel(req.Context)
		defer cancel()

		d := nd.BSDHT
		errCh := make(chan error, 1)

		go func() {
			defer close(errCh)
			defer cancel()

			//key := fmt.Sprintf("%s:%s", name, d.Node.Id())
			err := d.PutNamedValue(ctx, d.Node.Name(), []byte("dnslink="+p.String()))
			if err != nil {
				errCh <- cmds.Errorf(cmds.ErrClient, "Error putting named value: %s", err)
				return
			}
		}()
		if err := <-errCh; err != nil {
			return err
		}

		ret := BsnsEntry{
			Name:  d.Node.Name(),
			Value: p.String(),
			Error: "",
		}

		cmds.EmitOnce(res, &ret)

		return nil
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, ie *BsnsEntry) error {
			var err error
			_, err = fmt.Fprintf(w, "Registered to %s: %s\n", cmdenv.EscNonPrint(ie.Name), cmdenv.EscNonPrint(ie.Value))
			return err
		}),
	},
	Type: BsnsEntry{},
}

var QueryCmd = &cmds.Command{
	Status: cmds.Experimental,
	Helptext: cmds.HelpText{
		Tagline:          "Find the CID to a given name",
		ShortDescription: "Outputs a list of newline-delimited pairs of Name and CID.",
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("name", true, true, "The name to run the query against."),
	},
	Options: []cmds.Option{
		cmds.BoolOption("verbose", "v", "Print extra information."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		var start time.Time
		verbose, _ := req.Options["verbose"].(bool)
		if verbose {
			start = time.Now()
		}
		nd, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		if nd.BSDHT == nil {
			return errors.New("routing service is not a NSDHT")
		}

		name := req.Arguments[0]

		ctx, cancel := context.WithCancel(req.Context)
		defer cancel()

		d := nd.BSDHT
		keys, vals, err := d.LookupNames(ctx, name, true)
		if err != nil {
			fmt.Printf("err: %s\n", err)
			return err
		}
		if verbose {
			fmt.Printf("time= %d\n", (time.Now().UnixMilli() - start.UnixMilli()))
		}
		entries := make([]BsnsEntry, len(keys))
		for i, key := range keys {
			entries[i] = BsnsEntry{
				Name:  key,
				Value: vals[i],
			}
		}
		res.Emit(BsnsEntries{
			Entries: entries,
		})
		return nil
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *BsnsEntries) error {
			for _, o := range out.Entries {
				_, err := fmt.Fprintf(w, "%s\t%s\n", cmdenv.EscNonPrint(o.Name), cmdenv.EscNonPrint(o.Value))
				if err != nil {
					return err
				}
			}
			return nil
		}),
	},
	Type: BsnsEntries{},
}

// GetRepoPath gets the IPFS repository path
func GetRepoPath() (string, error) {
	repoPath, err := fsrepo.BestKnownPath()
	if err != nil {
		return "", err
	}
	return repoPath, nil
}

// yOrN prompts the user for a yes or no answer
func yOrN(prompt string) bool {
	var scanner = bufio.NewScanner(os.Stdin)

	fmt.Print(prompt)

	for scanner.Scan() {
		answer := strings.TrimSpace(scanner.Text())
		if answer == "y" {
			return true
		} else if answer == "n" {
			return false
		} else {
			fmt.Print("Please enter 'y' or 'n': ")
		}
	}

	// If we get here, there was an error with the scanner
	return false
}

// parse str
// format: xxxx:yyyy
// xxxx might include ":".
// pick yyyy by searching ":" from tail of the string.
// then decode it to peer.ID. If succeeded, return it.
// return name(xxxx part) and peer.ID.
func parse(str string) (string, peer.ID) {
	// Find the last colon in the string
	lastColonIndex := strings.LastIndex(str, ":")
	if lastColonIndex == -1 {
		return str, ""
	}

	// Extract the name part (before the last colon)
	name := str[:lastColonIndex]

	// Extract the peer ID part (after the last colon)
	peerIDStr := str[lastColonIndex+1:]
	if peerIDStr == "" {
		return name, ""
	}

	// Decode the peer ID
	peerID, err := peer.Decode(peerIDStr)
	if err != nil {
		return name, ""
	}

	return name, peerID
}

var TrustCmd = &cmds.Command{
	Status: cmds.Experimental,
	Helptext: cmds.HelpText{
		Tagline:          "trust a name",
		ShortDescription: "Trust a peer with completion.",
	},

	Arguments: []cmds.Argument{},

	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		return nil
	},

	PostRun: cmds.PostRunMap{
		cmds.CLI: func(res cmds.Response, re cmds.ResponseEmitter) error {
			repoPath, err := getRepoPath(res.Request())
			if err != nil {
				return err
			}
			api, err := rpc.NewPathApi(repoPath)
			if err != nil {
				return err
			}

			completer := readline.NewPrefixCompleter(
				readline.PcItemDynamic(func(prefix string) []string {
					entries, err := runCommand(api, "name/hrns/search", prefix)
					if err != nil {
						return nil
					}
					return entries
				}),
			)

			rl, err := readline.NewEx(&readline.Config{
				Prompt:       "name: ",
				AutoComplete: completer,
			})

			/*
				rl2, err := readline.NewEx(&readline.Config{
					Prompt: "trust: ",
					AutoComplete: &readline.PrefixCompleter{
						Dynamic: true,
						Callback: func(prefix string) []string {
							entries, err := runCommand(api, "name/hrns/trust", prefix)
							if err != nil {
								return nil
							}
							return entries
						},
					},
				})
			*/
			if err != nil {
				return err
			}
			defer rl.Close()

			prefix, err := rl.Readline()

			if err != nil {
				return err
			}
			prefix = strings.TrimSpace(prefix)

			name, peerID := parse(prefix)
			if len(peerID) > 0 {
				prompt := fmt.Sprintf("Trust %s for %s? (y/n): ", cmdenv.EscNonPrint(peerID.String()), cmdenv.EscNonPrint(name))
				if yOrN(prompt) {
					fmt.Printf("Trusted %s\n", cmdenv.EscNonPrint(peerID.String()))
				}
				return nil
			}

			entries, err := runCommand(api, "name/hrns/search", name)
			if err != nil {
				return err
			}
			if len(entries) == 1 {
				fmt.Printf("sole name: %v\n", entries[0])
				name, peerID := parse(entries[0])
				if len(peerID) > 0 {
					prompt := fmt.Sprintf("Trust %s for %s? (y/n): ", cmdenv.EscNonPrint(peerID.String()), cmdenv.EscNonPrint(name))
					if yOrN(prompt) {
						fmt.Printf("Trusted %s\n", cmdenv.EscNonPrint(peerID.String()))
					}
					return nil
				}
			}
			fmt.Printf("Not a sole name: %d names\n", len(entries))
			for _, e := range entries {
				fmt.Printf("%s\n", cmdenv.EscNonPrint(e))
			}
			return nil

		},
	},
	Encoders: cmds.EncoderMap{},
}

var PeersCmd = &cmds.Command{
	Status: cmds.Experimental,
	Helptext: cmds.HelpText{
		Tagline:          "Query the peer IDs that published the name.",
		ShortDescription: "Outputs the verification result.",
	},

	Arguments: []cmds.Argument{
		cmds.StringArg("name", true, false, "The name to query."),
	},
	Options: []cmds.Option{
		cmds.BoolOption("verbose", "v", "Print extra information."),
	},
	Run: func(req *cmds.Request, res cmds.ResponseEmitter, env cmds.Environment) error {
		var start time.Time
		verbose, _ := req.Options["verbose"].(bool)
		if verbose {
			start = time.Now()
		}
		nd, err := cmdenv.GetNode(env)
		if err != nil {
			return err
		}

		if nd.BSDHT == nil {
			return errors.New("routing service is not a RQDHT")
		}

		name := req.Arguments[0]

		ctx, cancel := context.WithCancel(req.Context)
		defer cancel()

		d := nd.BSDHT
		errCh := make(chan error, 1)

		go func() {
			defer close(errCh)
			defer cancel()
			peers, err := d.LookupName(ctx, name)
			if err != nil {
				errCh <- err
				return
			}
			if t, _ := d.LookupLocal(ctx, name); t {
				ret := BsnsEntry{
					Name:  "(self)",
					Value: d.Node.Id().String(),
				}

				if err := res.Emit(ret); err != nil {
					errCh <- err
					return
				}
			}
			for _, e := range peers {
				ret := BsnsEntry{
					Name:  e.Key().String(),
					Value: e.Id().String(),
				}
				if err := res.Emit(ret); err != nil {
					errCh <- err
					return
				}
			}
			if verbose {
				fmt.Printf("time= %d\n", (time.Now().UnixMilli() - start.UnixMilli()))
			}
		}()

		return <-errCh
	},
	Encoders: cmds.EncoderMap{
		cmds.Text: cmds.MakeTypedEncoder(func(req *cmds.Request, w io.Writer, out *BsnsEntry) error {
			_, err := fmt.Fprintf(w, "%s\t%s\n", cmdenv.EscNonPrint(out.Value), cmdenv.EscNonPrint(out.Name))
			return err
		}),
	},
	Type: BsnsEntry{},
}
