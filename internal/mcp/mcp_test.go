package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/111Reihan/kiro/internal/engine"
)

// session feeds lines to a server and returns the decoded replies.
func session(t *testing.T, e *engine.Engine, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	s := &Server{E: e, Version: "test"}
	if err := s.Run(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var rs []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("stdout line is not JSON: %q", l)
		}
		rs = append(rs, m)
	}
	return rs
}

func toolText(t *testing.T, r map[string]any) (string, bool) {
	t.Helper()
	res, _ := r["result"].(map[string]any)
	c, _ := res["content"].([]any)
	if len(c) != 1 {
		t.Fatalf("bad tool result: %v", r)
	}
	text, _ := c[0].(map[string]any)["text"].(string)
	isErr, _ := res["isError"].(bool)
	return text, isErr
}

func call(id int, name, args string) string {
	return `{"jsonrpc":"2.0","id":` + string(rune('0'+id)) + `,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
}

func TestHandshakeAndTools(t *testing.T) {
	e, _ := engine.New(t.TempDir())
	rs := session(t, e,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"nope"}`,
	)
	if len(rs) != 4 { // the notification gets no reply
		t.Fatalf("got %d replies, want 4: %v", len(rs), rs)
	}
	if rs[0]["result"].(map[string]any)["protocolVersion"] != "2024-11-05" {
		t.Errorf("version not negotiated: %v", rs[0])
	}
	if n := len(rs[2]["result"].(map[string]any)["tools"].([]any)); n != len(tools) {
		t.Errorf("listed %d tools, want %d", n, len(tools))
	}
	if rs[3]["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Errorf("unknown method: %v", rs[3])
	}
}

func TestCheckFlow(t *testing.T) {
	e, _ := engine.New(t.TempDir())
	src := "Four score and seven years ago our fathers brought forth on this continent, a new nation, conceived in Liberty."
	draft := `He said "Four score and seven years ago our fathers brought forth on this continent" and later "that government shall never perish from the earth entirely".`
	b, _ := json.Marshal(map[string]any{"id": "g", "text": src})
	d, _ := json.Marshal(map[string]any{"text": draft})
	rs := session(t, e,
		call(1, "add_source", string(b)),
		call(2, "add_source", string(b)),
		call(3, "check_text", string(d)),
		call(4, "who_haunts", `{"doc":"g","start":0,"end":10}`),
		call(5, "issue_receipt", `{"quote":"Four score and seven years ago","doc":"g"}`),
		call(6, "issue_receipt", `{"quote":"this is not in the source at all","doc":"g"}`),
		call(7, "list_sources", `{}`),
		call(8, "nope", `{}`),
	)
	if tx, isErr := toolText(t, rs[0]); isErr || !strings.Contains(tx, "version 1") {
		t.Errorf("add_source: %q", tx)
	}
	if tx, _ := toolText(t, rs[1]); !strings.Contains(tx, "unchanged") {
		t.Errorf("repeat add_source: %q", tx)
	}
	tx, _ := toolText(t, rs[2])
	for _, want := range []string{"[verbatim]", "[unsupported]", "1 verbatim, 0 drifted, 1 unsupported"} {
		if !strings.Contains(tx, want) {
			t.Errorf("check_text output lacks %q:\n%s", want, tx)
		}
	}
	if tx, _ := toolText(t, rs[3]); strings.TrimSpace(tx) != "[]" {
		t.Errorf("who_haunts with no cites: %q", tx)
	}
	if tx, isErr := toolText(t, rs[4]); isErr || !strings.Contains(tx, `"id"`) {
		t.Errorf("issue_receipt: %q", tx)
	}
	if tx, isErr := toolText(t, rs[5]); !isErr || !strings.Contains(tx, "not found") {
		t.Errorf("receipt for unsupported quote should be a tool error: %q", tx)
	}
	if tx, _ := toolText(t, rs[6]); !strings.Contains(tx, `"versions": 1`) {
		t.Errorf("list_sources: %q", tx)
	}
	if rs[7]["error"] == nil {
		t.Errorf("unknown tool must be a protocol error: %v", rs[7])
	}
}

func TestBadInputNeverBreaksTheStream(t *testing.T) {
	e, _ := engine.New(t.TempDir())
	rs := session(t, e, `not json`, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"oops"}`, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	if len(rs) != 3 || rs[0]["error"] == nil || rs[1]["error"] == nil || rs[2]["result"] == nil {
		t.Fatalf("replies: %v", rs)
	}
}
