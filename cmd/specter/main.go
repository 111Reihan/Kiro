// Command specter links requirements to the tests that prove them and tells
// you when either side drifts.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/111Reihan/kiro/internal/align"
	"github.com/111Reihan/kiro/internal/api"
	"github.com/111Reihan/kiro/internal/demo"
	"github.com/111Reihan/kiro/internal/engine"
	"github.com/111Reihan/kiro/internal/ledger"
	"github.com/111Reihan/kiro/internal/spec"
	"github.com/111Reihan/kiro/internal/store"
	"github.com/111Reihan/kiro/internal/web"
)

const version = "0.1.0"

const usage = `specter: keep requirements and the tests that prove them honest

  specter status [-json]            where every requirement stands
  specter check [-strict]           exit 1 when something drifted (for CI)
  specter run [-seal] -- CMD...     run tests (go test -json ./...), store the run
  specter ingest FILE               store a go test -json or JUnit XML file
  specter seal [ID...]              sign requirements whose tests passed
  specter explain ID                show the ghost diff for one requirement
  specter export ID                 print a requirement's signed attestation
  specter verify FILE [-key K] [-log PATH]
                                    check an attestation without trusting this machine
  specter audit                     check the hash chain, blobs and every seal
  specter serve [-addr A] [-readonly] [-demo]
  specter key                       print the public signing key
  specter version

flags for most commands: -dir PROJECT (default .)  -data DIR (default PROJECT/.specter)
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

type env struct {
	out, err io.Writer
}

func run(args []string, out, errw io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(errw, usage)
		return 2
	}
	v := &env{out, errw}
	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "status":
		err = v.status(rest)
	case "check":
		err = v.check(rest)
	case "run":
		err = v.runTests(rest)
	case "ingest":
		err = v.ingest(rest)
	case "seal":
		err = v.seal(rest)
	case "explain":
		err = v.explain(rest)
	case "export":
		err = v.export(rest)
	case "verify":
		err = v.verify(rest)
	case "audit":
		err = v.audit(rest)
	case "serve":
		err = v.serve(rest)
	case "key":
		err = v.key(rest)
	case "version", "-v", "--version":
		fmt.Fprintln(out, "specter", version)
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
	default:
		fmt.Fprintf(errw, "specter: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	var ue usageErr
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errDrift):
		return 1
	case errors.As(err, &ue):
		fmt.Fprintln(errw, "specter:", err)
		return 2
	case errors.Is(err, flag.ErrHelp):
		return 0
	}
	fmt.Fprintln(errw, "specter:", err)
	return 1
}

type usageErr struct{ msg string }

func (u usageErr) Error() string { return u.msg }

var errDrift = errors.New("drift found")

type common struct {
	dir, data string
}

func (v *env) flags(name string, c *common) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(v.err)
	fs.StringVar(&c.dir, "dir", ".", "project directory")
	fs.StringVar(&c.data, "data", "", "ledger directory (default DIR/.specter)")
	return fs
}

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usageErr{err.Error()}
	}
	return nil
}

// open scans the project and opens its ledger.
func (c *common) open() (*ledger.Ledger, *spec.Project, error) {
	p, err := spec.Scan(c.dir)
	if err != nil {
		return nil, nil, err
	}
	if c.data == "" {
		c.data = filepath.Join(p.Root, ".specter")
		if err := os.MkdirAll(c.data, 0o755); err != nil {
			return nil, nil, err
		}
		// the log is evidence worth committing; the key is not
		ig := filepath.Join(c.data, ".gitignore")
		if _, err := os.Stat(ig); err != nil {
			_ = os.WriteFile(ig, []byte("key.ed25519\n"), 0o644)
		}
	}
	e, err := engine.New(c.data)
	if err != nil {
		return nil, nil, err
	}
	return ledger.New(e), p, nil
}

func (v *env) report(args []string, name string) (*ledger.Report, *ledger.Ledger, *spec.Project, *flag.FlagSet, error) {
	var c common
	fs := v.flags(name, &c)
	return v.reportWith(fs, &c, args)
}

func (v *env) reportWith(fs *flag.FlagSet, c *common, args []string) (*ledger.Report, *ledger.Ledger, *spec.Project, *flag.FlagSet, error) {
	if err := parse(fs, args); err != nil {
		return nil, nil, nil, nil, err
	}
	l, p, err := c.open()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	rep, err := l.Status(p)
	return rep, l, p, fs, err
}

func (v *env) status(args []string) error {
	var c common
	fs := v.flags("status", &c)
	asJSON := fs.Bool("json", false, "print JSON")
	rep, _, _, _, err := v.reportWith(fs, &c, args)
	if err != nil {
		return err
	}
	if *asJSON {
		e := json.NewEncoder(v.out)
		e.SetIndent("", "  ")
		return e.Encode(rep)
	}
	tw := tabwriter.NewWriter(v.out, 0, 2, 2, ' ', 0)
	for _, r := range rep.Reqs {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Status, r.ID, clip(r.Text, 70))
	}
	for _, o := range rep.Orphans {
		moved := ""
		if o.MovedTo != "" {
			moved = fmt.Sprintf(" (words now in %s, %d%% alike)", o.MovedTo, o.Match)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s%s\n", ledger.Orphaned, o.Req, clip(o.Text, 50), moved)
	}
	tw.Flush()
	fmt.Fprintf(v.out, "\n%s\n", summary(rep))
	for _, w := range rep.Warnings {
		fmt.Fprintln(v.err, "warning:", w)
	}
	return nil
}

func summary(rep *ledger.Report) string {
	var parts []string
	for _, s := range ledger.Order {
		if n := rep.Counts[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	if len(parts) == 0 {
		return "no requirements found (looked for **R1**, REQ-1: and Kiro requirements.md layouts)"
	}
	return strings.Join(parts, ", ")
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func (v *env) check(args []string) error {
	var c common
	fs := v.flags("check", &c)
	strict := fs.Bool("strict", false, "also fail on uncovered and unsealed requirements")
	rep, _, _, _, err := v.reportWith(fs, &c, args)
	if err != nil {
		return err
	}
	bad := map[string]bool{ledger.Failing: true, ledger.Stale: true, ledger.Orphaned: true}
	if *strict {
		bad[ledger.Uncovered], bad[ledger.Unsealed] = true, true
	}
	n := 0
	for _, r := range rep.Reqs {
		if !bad[r.Status] {
			continue
		}
		n++
		fmt.Fprintf(v.out, "%s  %s  %s\n", r.Status, r.ID, clip(r.Text, 80))
		for _, why := range r.Reasons {
			fmt.Fprintf(v.out, "    - %s\n", why)
		}
		if r.Change != nil {
			fmt.Fprintf(v.out, "    %s\n", inline(r.Change.Ops))
		}
	}
	if bad[ledger.Orphaned] {
		for _, o := range rep.Orphans {
			n++
			fmt.Fprintf(v.out, "orphaned  %s  sealed but gone\n", o.Req)
		}
	}
	fmt.Fprintln(v.out, summary(rep))
	if n > 0 {
		return errDrift
	}
	return nil
}

// inline writes a word diff as plain text: [-old-]{+new+}.
func inline(ops []align.Op) string {
	var b strings.Builder
	for i, o := range ops {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch o.Kind {
		case "eq":
			b.WriteString(o.S)
		case "sub":
			fmt.Fprintf(&b, "[-%s-]{+%s+}", o.Q, o.S)
		case "del":
			fmt.Fprintf(&b, "[-%s-]", o.Q)
		case "ins":
			fmt.Fprintf(&b, "{+%s+}", o.S)
		}
	}
	return b.String()
}

func (v *env) runTests(args []string) error {
	var c common
	fs := v.flags("run", &c)
	doSeal := fs.Bool("seal", false, "seal passing requirements after the run")
	if err := parse(fs, args); err != nil {
		return err
	}
	cmdline := fs.Args()
	if len(cmdline) == 0 {
		return usageErr{"run needs a command after --, for example: specter run -seal -- go test -json ./..."}
	}
	l, p, err := c.open()
	if err != nil {
		return err
	}
	cmd := exec.Command(cmdline[0], cmdline[1:]...)
	cmd.Dir = p.Root
	cmd.Stderr = v.err
	var buf bytes.Buffer
	cmd.Stdout = &limitWriter{w: &buf, left: 64 << 20}
	runErr := cmd.Run() // failing tests make the command exit non-zero; that is data, not an error
	rs, perr := spec.ParseResults(buf.Bytes())
	if perr != nil {
		if runErr != nil {
			return fmt.Errorf("%v; and %v", runErr, perr)
		}
		return fmt.Errorf("%v (use go test -json, or write JUnit XML and run specter ingest)", perr)
	}
	return v.store(l, p, rs, strings.Join(cmdline, " "), *doSeal)
}

func (v *env) ingest(args []string) error {
	var c common
	fs := v.flags("ingest", &c)
	doSeal := fs.Bool("seal", false, "seal passing requirements after storing")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr{"ingest needs one file"}
	}
	b, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	rs, err := spec.ParseResults(b)
	if err != nil {
		return err
	}
	l, p, err := c.open()
	if err != nil {
		return err
	}
	return v.store(l, p, rs, "ingest "+filepath.Base(fs.Arg(0)), *doSeal)
}

func (v *env) store(l *ledger.Ledger, p *spec.Project, rs []spec.Result, cmd string, doSeal bool) error {
	if _, err := l.Record(p); err != nil {
		return err
	}
	ri, err := l.AddRun(rs, cmd)
	if err != nil {
		return err
	}
	fmt.Fprintf(v.out, "stored run: %d passed, %d failed, %d skipped\n", ri.Pass, ri.Fail, ri.Skip)
	if doSeal {
		return v.doSeal(l, p, nil)
	}
	return nil
}

func (v *env) doSeal(l *ledger.Ledger, p *spec.Project, ids []string) error {
	res, err := l.Seal(p, ids)
	if err != nil {
		return err
	}
	for _, s := range res.Sealed {
		fmt.Fprintf(v.out, "sealed   %s  (%d tests)\n", s.Req, len(s.Tests))
	}
	for _, s := range res.Skipped {
		if s.Why != "already verified" {
			fmt.Fprintf(v.out, "skipped  %s  %s\n", s.Req, s.Why)
		}
	}
	return nil
}

func (v *env) seal(args []string) error {
	var c common
	fs := v.flags("seal", &c)
	if err := parse(fs, args); err != nil {
		return err
	}
	l, p, err := c.open()
	if err != nil {
		return err
	}
	return v.doSeal(l, p, fs.Args())
}

func (v *env) explain(args []string) error {
	var c common
	fs := v.flags("explain", &c)
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr{"explain needs one requirement id"}
	}
	l, p, err := c.open()
	if err != nil {
		return err
	}
	rep, err := l.Status(p)
	if err != nil {
		return err
	}
	for _, r := range rep.Reqs {
		if r.ID != fs.Arg(0) {
			continue
		}
		fmt.Fprintf(v.out, "%s  %s\n%s:%d\n\n%s\n", r.ID, r.Status, r.File, r.Line, r.Text)
		if r.Sealed != nil {
			fmt.Fprintf(v.out, "\nsealed in event %d at %s\n", r.Sealed.Seq, r.Sealed.TS)
		}
		if r.Change != nil {
			fmt.Fprintf(v.out, "\nghost diff (%s change):\n  %s\n", r.Change.Severity, inline(r.Change.Ops))
		}
		for _, why := range r.Reasons {
			fmt.Fprintf(v.out, "- %s\n", why)
		}
		for _, t := range r.Tests {
			fmt.Fprintf(v.out, "test %s  %s:%d  %s\n", t.Result, t.File, t.Line, t.Name)
		}
		return nil
	}
	for _, o := range rep.Orphans {
		if o.Req == fs.Arg(0) {
			fmt.Fprintf(v.out, "%s  orphaned\nsealed text: %s\n", o.Req, o.Text)
			if o.MovedTo != "" {
				fmt.Fprintf(v.out, "these words now live in %s (%d%% alike)\n", o.MovedTo, o.Match)
			}
			return nil
		}
	}
	return fmt.Errorf("no requirement %q", fs.Arg(0))
}

func (v *env) export(args []string) error {
	var c common
	fs := v.flags("export", &c)
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return usageErr{"export needs one requirement id"}
	}
	l, _, err := c.open()
	if err != nil {
		return err
	}
	s, ok, err := l.SealOf(fs.Arg(0))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s has no seal", fs.Arg(0))
	}
	e := json.NewEncoder(v.out)
	e.SetIndent("", "  ")
	e.SetEscapeHTML(false)
	return e.Encode(s)
}

func (v *env) verify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(v.err)
	key := fs.String("key", "", "public key (base64) the attestation must be signed with")
	logp := fs.String("log", "", "a copy of specter.log to check the anchor against")
	// allow the file before or after the flags
	var files []string
	for len(args) > 0 {
		if err := parse(fs, args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		files = append(files, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(files) != 1 {
		return usageErr{"verify needs one attestation file"}
	}
	b, err := os.ReadFile(files[0])
	if err != nil {
		return err
	}
	var s ledger.Signed
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return fmt.Errorf("not an attestation: %w", err)
	}
	if err := ledger.VerifySigned(s, *key); err != nil {
		return err
	}
	fmt.Fprintf(v.out, "signature ok   %s signed by key %s\n", s.Req, keyID(s.Key))
	if *key == "" {
		fmt.Fprintln(v.out, "note           no -key given: this proves the file is intact, not who signed it")
	}
	if *logp != "" {
		evs, err := store.ReadLog(*logp)
		if err != nil {
			return err
		}
		if err := ledger.CheckAnchor(s.Attestation, evs); err != nil {
			return err
		}
		fmt.Fprintf(v.out, "anchor ok      event %d of the log has hash %.12s…\n", s.Anchor.Seq, s.Anchor.Hash)
	}
	return nil
}

func keyID(b64 string) string {
	var pub []byte
	if err := json.Unmarshal([]byte(`"`+b64+`"`), &pub); err != nil || len(pub) != 32 {
		return "?"
	}
	return engine.KeyID(pub)
}

func (v *env) audit(args []string) error {
	var c common
	fs := v.flags("audit", &c)
	if err := parse(fs, args); err != nil {
		return err
	}
	l, _, err := c.open()
	if err != nil {
		return err
	}
	a, err := l.Audit()
	if err != nil {
		return err
	}
	fmt.Fprintf(v.out, "events %d  head %.16s…  key %s\n", a.Events, a.Head, a.KeyID)
	bad := 0
	if a.ChainOK {
		fmt.Fprintln(v.out, "chain ok")
	} else {
		bad++
		fmt.Fprintln(v.out, "CHAIN BROKEN:", a.ChainErr)
	}
	for _, s := range a.Seals {
		if !s.OK {
			bad++
			fmt.Fprintf(v.out, "BAD SEAL  event %d  %s  %s\n", s.Seq, s.Req, s.Err)
		}
	}
	fmt.Fprintf(v.out, "%d seals checked\n", len(a.Seals))
	if bad > 0 {
		return errDrift
	}
	return nil
}

func (v *env) key(args []string) error {
	var c common
	fs := v.flags("key", &c)
	if err := parse(fs, args); err != nil {
		return err
	}
	l, _, err := c.open()
	if err != nil {
		return err
	}
	fmt.Fprintf(v.out, "%s\nkey id %s\n", l.E.PubKey(), l.E.KeyID())
	return nil
}

func (v *env) serve(args []string) error {
	var c common
	fs := v.flags("serve", &c)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	ro := fs.Bool("readonly", false, "refuse rescan and seal")
	dm := fs.Bool("demo", false, "serve a built-in project with drift (implies -readonly)")
	if err := parse(fs, args); err != nil {
		return err
	}
	var l *ledger.Ledger
	var root string
	if *dm {
		tmp, err := os.MkdirTemp("", "specter-demo-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		proj, data, err := demo.Build(tmp)
		if err != nil {
			return err
		}
		e, err := engine.New(data)
		if err != nil {
			return err
		}
		l, root, *ro = ledger.New(e), proj, true
	} else {
		var p *spec.Project
		var err error
		if l, p, err = c.open(); err != nil {
			return err
		}
		root = p.Root
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return usageErr{"bad -addr: " + err.Error()}
	}
	ip := net.ParseIP(host)
	loop := host == "localhost" || (ip != nil && ip.IsLoopback())
	srv := &http.Server{Addr: *addr, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
		Handler: api.Handler(l, api.Options{Root: root, ReadOnly: *ro, Loopback: loop, Static: web.FS()})}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	mode := "read-write"
	if *ro {
		mode = "read-only"
	}
	fmt.Fprintf(v.out, "specter %s on http://%s  (%s, key %s)\n", version, ln.Addr(), mode, l.E.KeyID())
	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sc)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type limitWriter struct {
	w    io.Writer
	left int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if len(p) > l.left {
		return 0, errors.New("test output is over 64 MiB")
	}
	l.left -= len(p)
	return l.w.Write(p)
}
