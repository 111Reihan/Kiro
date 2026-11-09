package demo

import (
	"testing"

	"github.com/111Reihan/kiro/internal/engine"
	"github.com/111Reihan/kiro/internal/ledger"
	"github.com/111Reihan/kiro/internal/spec"
)

func TestDemoShowsEveryStatus(t *testing.T) {
	proj, data, err := Build(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(data)
	if err != nil {
		t.Fatal(err)
	}
	p, err := spec.Scan(proj)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := ledger.New(e).Status(p)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"checkout/1.1": ledger.Stale, "checkout/1.2": ledger.Stale, "checkout/2.1": ledger.Verified,
		"checkout/2.2": ledger.Verified, "auth/1.1": ledger.Failing, "auth/1.2": ledger.Stale,
		"auth/1.3": ledger.Uncovered, "notify/1.1": ledger.Unsealed, "notify/1.2": ledger.Uncovered,
	}
	got := map[string]string{}
	for _, r := range rep.Reqs {
		got[r.ID] = r.Status
	}
	for id, s := range want {
		if got[id] != s {
			t.Errorf("%s: want %s got %s", id, s, got[id])
		}
	}
	orph := map[string]ledger.Orphan{}
	for _, o := range rep.Orphans {
		orph[o.Req] = o
	}
	if orph["checkout/2.4"].MovedTo != "notify/1.1" {
		t.Errorf("moved requirement: %+v", orph)
	}
	if o, ok := orph["checkout/2.3"]; !ok || o.MovedTo != "" {
		t.Errorf("deleted requirement: %+v", orph)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("demo has warnings: %v", rep.Warnings)
	}
	if a, err := ledger.New(e).Audit(); err != nil || !a.ChainOK {
		t.Errorf("audit: %+v %v", a, err)
	}
}
