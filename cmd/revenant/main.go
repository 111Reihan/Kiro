// Command revenant checks quotes against versioned sources. One binary: web
// app, command line, and MCP server for coding agents.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/111Reihan/kiro/examples"
	"github.com/111Reihan/kiro/internal/api"
	"github.com/111Reihan/kiro/internal/engine"
	"github.com/111Reihan/kiro/internal/mcp"
	"github.com/111Reihan/kiro/internal/receipt"
	"github.com/111Reihan/kiro/internal/store"
	"github.com/111Reihan/kiro/internal/web"
)

const version = "0.1.0"

const usage = `revenant - check quotes against versioned sources

usage: revenant <command> [flags] [arguments]      (flags go before arguments)

  serve                         web app and JSON API         [-addr a] [-demo] [-readonly]
  add <id> <file|->             add a source, or a new version of one   [-title t]
  check <file|->                grade every quoted passage   [-json] [-doc id]...
  receipt <quote|->             sign a receipt for a quote   -doc id [-ver n]
  verify-receipt <file|->       check a receipt offline      [-source file] [-pin key]
  audit                         verify the hash chain and every stored blob
  key                           print this install's signing key id and public key
  mcp                           MCP server on stdin/stdout for coding agents
  version

Every command that touches data takes -data <dir> (default $REVENANT_DATA or ./data).
check exits 0 when every quote is verbatim, 1 otherwise, 2 on bad usage.
`

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

type cli struct {
	in       io.Reader
	out, err io.Writer
}

func run(args []string, in io.Reader, out, errw io.Writer) int {
	c := cli{in, out, errw}
	if len(args) == 0 {
		fmt.Fprint(errw, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "serve":
		return c.serve(rest)
	case "add":
		return c.add(rest)
	case "check":
		return c.check(rest)
	case "receipt":
		return c.receipt(rest)
	case "verify-receipt":
		return c.verifyReceipt(rest)
	case "audit":
		return c.audit(rest)
	case "key":
		return c.key(rest)
	case "mcp":
		return c.mcp(rest)
	case "version":
		fmt.Fprintln(out, "revenant", version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return 0
	}
	fmt.Fprintf(errw, "revenant: unknown command %q\n\n%s", cmd, usage)
	return 2
}

// ---- helpers ----

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (c cli) flags(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.err)
	return fs, fs.String("data", envOr("REVENANT_DATA", "data"), "data directory")
}

// parse returns -1 to carry on, or the exit code to stop with.
func (c cli) parse(fs *flag.FlagSet, args []string, min, max int) int {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if n := fs.NArg(); n < min || n > max {
		fmt.Fprintf(c.err, "revenant %s: expected %d to %d arguments, got %d\n", fs.Name(), min, max, n)
		fs.Usage()
		return 2
	}
	return -1
}

func (c cli) fail(err error) int {
	fmt.Fprintf(c.err, "revenant: %v\n", err)
	return 2
}

// read returns the file's text, or stdin's when name is "-".
func (c cli) read(name string) (string, error) {
	var r io.Reader = c.in
	if name != "-" {
		f, err := os.Open(name)
		if err != nil {
			return "", err
		}
		defer f.Close()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, engine.MaxDocBytes+1)) // the engine rejects anything over the cap
	return string(b), err
}

type repeat []string

func (r *repeat) String() string     { return strings.Join(*r, ",") }
func (r *repeat) Set(v string) error { *r = append(*r, v); return nil }

// ---- commands ----

func (c cli) serve(args []string) int {
	fs, data := c.flags("serve")
	def := "127.0.0.1:8080"
	if port := os.Getenv("PORT"); port != "" { // hosting platforms hand the port over this way
		def = "0.0.0.0:" + port
	}
	addr := fs.String("addr", def, "listen address")
	demo := fs.Bool("demo", false, "load the bundled public-domain sources at start")
	ro := fs.Bool("readonly", false, "public demo: only checking text and verifying receipts may write")
	if code := c.parse(fs, args, 0, 0); code >= 0 {
		return code
	}
	e, err := engine.New(*data)
	if err != nil {
		return c.fail(err)
	}
	if *demo {
		ids, err := examples.Load(e)
		if err != nil {
			return c.fail(err)
		}
		fmt.Fprintf(c.out, "demo sources loaded: %s\n", strings.Join(ids, ", "))
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return c.fail(fmt.Errorf("bad -addr: %w", err))
	}
	loop := host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return c.fail(err)
	}
	srv := &http.Server{
		Handler:           api.Handler(e, api.Options{UI: web.FS(), Loopback: loop, ReadOnly: *ro}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	fmt.Fprintf(c.out, "revenant %s on http://%s  data=%s  key=%s\n", version, ln.Addr(), *data, e.KeyID())
	if !loop && !*ro {
		fmt.Fprintln(c.err, "warning: listening on a non-loopback address. There is no login; anyone who can reach it can read and add sources.")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return c.fail(err)
	}
	return 0
}

func (c cli) add(args []string) int {
	fs, data := c.flags("add")
	title := fs.String("title", "", "document title (default: the id)")
	if code := c.parse(fs, args, 2, 2); code >= 0 {
		return code
	}
	txt, err := c.read(fs.Arg(1))
	if err != nil {
		return c.fail(err)
	}
	e, err := engine.New(*data)
	if err != nil {
		return c.fail(err)
	}
	v, err := e.PutDoc(fs.Arg(0), *title, txt)
	if errors.Is(err, engine.ErrNoChange) {
		fmt.Fprintf(c.out, "%s is unchanged\n", fs.Arg(0))
		return 0
	}
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintf(c.out, "%s is at version %d\n", v.ID, v.Ver)
	return 0
}

func (c cli) check(args []string) int {
	fs, data := c.flags("check")
	asJSON := fs.Bool("json", false, "print JSON")
	var docs repeat
	fs.Var(&docs, "doc", "only check against this source (repeatable)")
	if code := c.parse(fs, args, 1, 1); code >= 0 {
		return code
	}
	txt, err := c.read(fs.Arg(0))
	if err != nil {
		return c.fail(err)
	}
	e, err := engine.New(*data)
	if err != nil {
		return c.fail(err)
	}
	found, err := e.CheckText(txt, engine.Scope{Docs: docs})
	if err != nil {
		return c.fail(err)
	}
	bad := 0
	for _, f := range found {
		if f.Result.Verdict != "verbatim" {
			bad++
		}
	}
	code := 0
	if bad > 0 {
		code = 1
	}
	if *asJSON {
		enc := json.NewEncoder(c.out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"found": found, "not_verbatim": bad}); err != nil {
			return c.fail(err)
		}
		return code
	}
	if len(found) == 0 {
		fmt.Fprintln(c.out, "no quoted passages of 4 or more words found")
		return 0
	}
	for _, f := range found {
		r := f.Result
		label := string(r.Verdict)
		if r.Severity != "" {
			label += "/" + string(r.Severity)
		}
		where := ""
		if r.Doc != "" {
			where = fmt.Sprintf("%s v%d", r.Doc, r.Ver)
		}
		q := f.Text
		if utf8.RuneCountInString(q) > 70 {
			q = string([]rune(q)[:70]) + "..."
		}
		fmt.Fprintf(c.out, "%-20s %3d%%  %-16s %q\n", label, r.Sim/10, where, q)
		for _, o := range r.Ops {
			if o.Kind != "eq" {
				fmt.Fprintf(c.out, "    %-4s quote=%q source=%q\n", o.Kind, o.Q, o.S)
			}
		}
		for _, n := range r.Notes {
			fmt.Fprintf(c.out, "    note: %s\n", n)
		}
	}
	fmt.Fprintf(c.out, "%d of %d quotes are not verbatim\n", bad, len(found))
	return code
}

func (c cli) receipt(args []string) int {
	fs, data := c.flags("receipt")
	doc := fs.String("doc", "", "source document id (required)")
	ver := fs.Int("ver", 0, "source version (default: latest)")
	if code := c.parse(fs, args, 1, 1); code >= 0 {
		return code
	}
	if *doc == "" {
		return c.fail(errors.New("-doc is required"))
	}
	quote := fs.Arg(0)
	if quote == "-" {
		q, err := c.read("-")
		if err != nil {
			return c.fail(err)
		}
		quote = strings.TrimSpace(q)
	}
	e, err := engine.New(*data)
	if err != nil {
		return c.fail(err)
	}
	env, id, err := e.Issue(quote, *doc, *ver)
	if errors.Is(err, engine.ErrNoMatch) {
		fmt.Fprintln(c.err, "revenant: no receipt: that quote is not found in the source")
		return 1
	}
	if err != nil {
		return c.fail(err)
	}
	enc := json.NewEncoder(c.out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(env); err != nil {
		return c.fail(err)
	}
	fmt.Fprintf(c.err, "receipt id %s\n", id)
	return 0
}

func (c cli) verifyReceipt(args []string) int {
	fs := flag.NewFlagSet("verify-receipt", flag.ContinueOnError)
	fs.SetOutput(c.err)
	source := fs.String("source", "", "the source file the receipt names; checks its hash and excerpts")
	pin := fs.String("pin", "", "expected public key (base64); fail if the receipt is signed by another")
	if code := c.parse(fs, args, 1, 1); code >= 0 {
		return code
	}
	raw, err := c.read(fs.Arg(0))
	if err != nil {
		return c.fail(err)
	}
	var env receipt.Envelope
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&env); err != nil {
		return c.fail(fmt.Errorf("not a receipt file: %w", err))
	}
	rep, err := receipt.Verify(env)
	if err == nil && *pin != "" {
		if *pin != env.Pub {
			err = errors.New("signed by a different key than the pinned one")
		} else {
			rep.Checked = append(rep.Checked, "signed by the pinned key")
		}
	}
	if err == nil && *source != "" {
		src, rerr := os.ReadFile(*source)
		if rerr != nil {
			return c.fail(rerr)
		}
		if err = receipt.CheckSource(env, src); err == nil {
			rep.Checked = append(rep.Checked, "source file matches the receipt's hash and excerpts")
		}
	}
	if err != nil {
		fmt.Fprintf(c.out, "FAIL: %v\n", err)
		return 1
	}
	fmt.Fprintf(c.out, "OK  %s  %s v%d  key %s\n", env.Body.Verdict, env.Body.Doc, env.Body.Ver, rep.Key)
	for _, s := range rep.Checked {
		fmt.Fprintf(c.out, "  checked:     %s\n", s)
	}
	for _, s := range rep.NotChecked {
		fmt.Fprintf(c.out, "  NOT checked: %s\n", s)
	}
	return 0
}

func (c cli) audit(args []string) int {
	fs, data := c.flags("audit")
	if code := c.parse(fs, args, 0, 0); code >= 0 {
		return code
	}
	st, err := store.Open(*data)
	if err == nil {
		err = st.Verify()
	}
	if err != nil {
		fmt.Fprintf(c.out, "FAIL: %v\n", err)
		return 1
	}
	n, head := st.Head()
	fmt.Fprintf(c.out, "OK  %d events, head %s\n", n, head)
	return 0
}

func (c cli) key(args []string) int {
	fs, data := c.flags("key")
	if code := c.parse(fs, args, 0, 0); code >= 0 {
		return code
	}
	e, err := engine.New(*data)
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintf(c.out, "key id  %s\npublic  %s\n", e.KeyID(), e.PubKey())
	return 0
}

func (c cli) mcp(args []string) int {
	fs, data := c.flags("mcp")
	if code := c.parse(fs, args, 0, 0); code >= 0 {
		return code
	}
	e, err := engine.New(*data)
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintf(c.err, "revenant mcp %s ready, data=%s\n", version, *data)
	if err := (&mcp.Server{E: e, Version: version}).Run(c.in, c.out); err != nil && !errors.Is(err, io.EOF) {
		return c.fail(err)
	}
	return 0
}
