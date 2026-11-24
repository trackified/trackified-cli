// Command trackified is the command-line client for the Trackified parcel-tracking API.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/trackified/trackified-go"
	"golang.org/x/term"
)

// version is set by GoReleaser.
var version = "dev"

const usage = `trackified - track parcels from the command line

Usage:
  trackified login                         store your API key
  trackified track <number> [--carrier c]  start tracking a parcel
  trackified get <id>                      show a tracking and its events
  trackified list [--status s] [--all]     list trackings
  trackified watch <id> [--interval 60s]   print new events until delivered
  trackified retrack <id>                  check the carrier again now
  trackified stop <id>                     stop tracking a parcel
  trackified carriers [search]             list or search carriers
  trackified detect <number>               guess the carrier of a number
  trackified webhooks list|add|rm|test     manage webhook endpoints
  trackified usage                         this month's usage and limits
  trackified verify-webhook --secret s --header h --body-file f
  trackified version

Every command takes --json for machine-readable output.
The API key comes from TRACKIFIED_API_KEY, else the file written by "trackified login".
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// cli carries what every command needs.
type cli struct {
	ctx    context.Context
	stdin  io.Reader
	out    io.Writer
	json   bool
	client func() (*trackified.Client, error)
}

func run(ctx context.Context, args []string, stdin io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(out, usage)
		return 0
	}
	c := &cli{ctx: ctx, stdin: stdin, out: out, client: newClient}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(errOut, "trackified: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err := cmd(c, args[1:]); err != nil {
		var ue usageError
		if errors.As(err, &ue) {
			fmt.Fprintf(errOut, "trackified %s: %s\n", args[0], ue)
			return 2
		}
		if errors.Is(err, errSilent) {
			return 1
		}
		fmt.Fprintln(errOut, "trackified:", strings.TrimPrefix(err.Error(), "trackified: "))
		return 1
	}
	return 0
}

type usageError string

func (e usageError) Error() string { return string(e) }

// errSilent exits 1 after the command already printed its result.
var errSilent = errors.New("silent")

// flags creates a FlagSet with the shared --json flag.
func (c *cli) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&c.json, "json", false, "print JSON")
	return fs
}

// parse parses flags placed anywhere among the arguments and returns the positional ones.
func parse(fs *flag.FlagSet, args []string, wantPos int) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, usageError(err.Error())
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		pos, args = append(pos, args[0]), args[1:]
	}
	if wantPos >= 0 && len(pos) != wantPos {
		return nil, usageError(fmt.Sprintf("expected %d argument(s), got %d", wantPos, len(pos)))
	}
	return pos, nil
}

// emit prints v as JSON with --json, otherwise calls human.
func (c *cli) emit(v any, human func()) error {
	if c.json {
		enc := json.NewEncoder(c.out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	human()
	return nil
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "trackified", "config.json"), nil
}

type config struct {
	APIKey string `json:"api_key"`
}

func loadAPIKey() string {
	if k := os.Getenv("TRACKIFIED_API_KEY"); k != "" {
		return k
	}
	p, err := configPath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var cfg config
	_ = json.Unmarshal(data, &cfg)
	return cfg.APIKey
}

func saveAPIKey(key string) (string, error) {
	p, err := configPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	data, _ := json.Marshal(config{APIKey: key})
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return "", err
	}
	return p, os.Chmod(p, 0o600) // WriteFile keeps the mode of an existing file
}

func newClient() (*trackified.Client, error) {
	key := loadAPIKey()
	if key == "" {
		return nil, errors.New(`no API key: run "trackified login" or set TRACKIFIED_API_KEY`)
	}
	return trackified.New(trackified.WithAPIKey(key), trackified.WithUserAgent("trackified-cli/"+version))
}

// readSecret reads one line, without echo when stdin is a terminal.
func readSecret(in io.Reader, prompt io.Writer) (string, error) {
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fmt.Fprint(prompt, "API key: ")
		b, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(prompt)
		return strings.TrimSpace(string(b)), err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
