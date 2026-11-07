// Package mcp serves the engine to coding agents (Kiro, Claude Code, ...) over
// the Model Context Protocol: JSON-RPC 2.0, one message per line on stdio.
//
// stdout carries protocol messages only. Anything else goes to the log writer.
package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/111Reihan/kiro/internal/engine"
)

const maxLine = 8 << 20

var protocols = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

type Server struct {
	E       *engine.Engine
	Version string
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
}

type errReply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   rpcError        `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Run reads requests until in closes. Each reply is written as one line.
func (s *Server) Run(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), maxLine)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if resp := s.handle([]byte(line)); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// handle returns nil for notifications, which get no reply.
func (s *Server) handle(line []byte) any {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return errReply{"2.0", json.RawMessage("null"), rpcError{-32700, "parse error"}}
	}
	if len(req.ID) == 0 {
		return nil
	}
	fail := func(code int, msg string) any { return errReply{"2.0", req.ID, rpcError{code, msg}} }
	ok := func(v any) any { return reply{"2.0", req.ID, v} }
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		ver := protocols[0]
		for _, v := range protocols {
			if v == p.ProtocolVersion {
				ver = v
			}
		}
		return ok(map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "revenant", "version": s.Version},
			"instructions": "Revenant checks quotes against versioned sources. Add sources first (add_source), " +
				"then check_text on a draft. unsupported means not found in the loaded sources, not proven false.",
		})
	case "ping":
		return ok(map[string]any{})
	case "tools/list":
		return ok(map[string]any{"tools": tools})
	case "tools/call":
		var p struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return fail(-32602, "bad params")
		}
		text, err := s.call(p.Name, p.Args)
		if errors.Is(err, errUnknownTool) {
			return fail(-32602, "unknown tool "+p.Name)
		}
		if err != nil { // tool failures are results the model can read and act on
			return ok(map[string]any{"isError": true, "content": []any{map[string]any{"type": "text", "text": err.Error()}}})
		}
		return ok(map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}})
	}
	return fail(-32601, "method not found")
}

var errUnknownTool = errors.New("unknown tool")

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var (
	str  = map[string]any{"type": "string"}
	num  = map[string]any{"type": "integer"}
	strs = map[string]any{"type": "array", "items": str}
)

var tools = []map[string]any{
	{
		"name":        "check_text",
		"description": "Find every double-quoted passage in a draft and grade each against the loaded sources: verbatim, drifted (with severity), unsupported, or no-source. Use on a markdown file after editing it.",
		"inputSchema": schema(map[string]any{"text": str, "docs": strs}, "text"),
	},
	{
		"name":        "check_quote",
		"description": "Grade one quote against the loaded sources and return the matched span and a word diff.",
		"inputSchema": schema(map[string]any{"quote": str, "docs": strs}, "quote"),
	},
	{
		"name":        "who_haunts",
		"description": "List the citations that land on a character range [start,end) (rune offsets) of a source, and where each one stands now.",
		"inputSchema": schema(map[string]any{"doc": str, "start": num, "end": num, "ver": num}, "doc", "start", "end"),
	},
	{
		"name":        "issue_receipt",
		"description": "Sign a receipt for a quote found in a source version. Returns the receipt id and the receipt JSON, which can be verified offline.",
		"inputSchema": schema(map[string]any{"quote": str, "doc": str, "ver": num}, "quote", "doc"),
	},
	{
		"name":        "list_sources",
		"description": "List the loaded source documents with their version counts.",
		"inputSchema": schema(map[string]any{}),
	},
	{
		"name":        "add_source",
		"description": "Add a source document, or a new version of one. Old versions are kept.",
		"inputSchema": schema(map[string]any{"id": str, "text": str, "title": str}, "id", "text"),
	},
}

func (s *Server) call(name string, args json.RawMessage) (string, error) {
	if len(args) == 0 {
		args = json.RawMessage("{}")
	}
	var a struct {
		Text  string   `json:"text"`
		Quote string   `json:"quote"`
		Docs  []string `json:"docs"`
		Doc   string   `json:"doc"`
		ID    string   `json:"id"`
		Title string   `json:"title"`
		Start int      `json:"start"`
		End   int      `json:"end"`
		Ver   int      `json:"ver"`
	}
	known := false
	for _, t := range tools {
		known = known || t["name"] == name
	}
	if !known {
		return "", errUnknownTool
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("bad arguments: %w", err)
	}
	sc := engine.Scope{Docs: a.Docs}
	switch name {
	case "check_text":
		found, err := s.E.CheckText(a.Text, sc)
		if err != nil {
			return "", err
		}
		return summarize(found), nil
	case "check_quote":
		r, err := s.E.Check(a.Quote, sc)
		if err != nil {
			return "", err
		}
		return pretty(r)
	case "who_haunts":
		c, err := s.E.Haunts(a.Doc, a.Ver, a.Start, a.End)
		if err != nil {
			return "", err
		}
		return pretty(c)
	case "issue_receipt":
		env, id, err := s.E.Issue(a.Quote, a.Doc, a.Ver)
		if err != nil {
			return "", err
		}
		return pretty(map[string]any{"id": id, "receipt": env})
	case "list_sources":
		d, err := s.E.Docs()
		if err != nil {
			return "", err
		}
		return pretty(d)
	default: // add_source
		v, err := s.E.PutDoc(a.ID, a.Title, a.Text)
		if errors.Is(err, engine.ErrNoChange) {
			return fmt.Sprintf("%s is unchanged; no new version", a.ID), nil
		}
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s is now at version %d", v.ID, v.Ver), nil
	}
}

func pretty(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}

// summarize keeps the answer short: one line per quote, then a count line.
func summarize(found []engine.Found) string {
	if len(found) == 0 {
		return "no quoted passages of 4 or more words found"
	}
	var b strings.Builder
	counts := map[string]int{}
	for _, f := range found {
		r := f.Result
		counts[string(r.Verdict)]++
		q := f.Text
		if utf8.RuneCountInString(q) > 80 {
			q = string([]rune(q)[:80]) + "..."
		}
		fmt.Fprintf(&b, "[%s", r.Verdict)
		if r.Severity != "" {
			fmt.Fprintf(&b, "/%s", r.Severity)
		}
		fmt.Fprintf(&b, "] %d%%", r.Sim/10)
		if r.Doc != "" {
			fmt.Fprintf(&b, " %s v%d", r.Doc, r.Ver)
		}
		fmt.Fprintf(&b, "  %q\n", q)
		for _, o := range r.Ops {
			if o.Kind != "eq" {
				fmt.Fprintf(&b, "    %s: quote %q, source %q\n", o.Kind, o.Q, o.S)
			}
		}
	}
	fmt.Fprintf(&b, "%d verbatim, %d drifted, %d unsupported, %d no-source", counts["verbatim"], counts["drifted"], counts["unsupported"], counts["no-source"])
	return b.String()
}
