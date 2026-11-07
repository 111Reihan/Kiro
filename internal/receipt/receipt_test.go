package receipt

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	mrand "math/rand"
	"testing"

	"github.com/111Reihan/kiro/internal/prop"
	"github.com/111Reihan/kiro/internal/verify"
)

const src = "Four score and seven years ago our fathers brought forth on this continent, a new nation, conceived in Liberty."

func issue(t *testing.T, quote string) (Envelope, ed25519.PrivateKey) {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	r := verify.Check(quote, verify.Prepare("gb", 1, src))
	env, err := Sign(Body{V: 1, Kind: "revenant.receipt", Issued: "2026-11-08T00:00:00Z", Quote: quote,
		Verdict: r.Verdict, Severity: r.Severity, Sim: r.Sim, Doc: "gb", Ver: 1, Blob: "ab", LogSeq: 1, LogHash: "cd",
		Spans: r.Spans, Ops: r.Ops}, priv)
	if err != nil {
		t.Fatal(err)
	}
	return env, priv
}

func TestRoundTrip(t *testing.T) {
	for _, q := range []string{
		"our fathers brought forth on this continent",
		"our fathers brought forth upon this continent",
		"Four score and seven years ago ... conceived in Liberty",
	} {
		env, _ := issue(t, q)
		if _, err := Verify(env); err != nil {
			t.Errorf("%q: %v", q, err)
		}
	}
}

func TestReportSaysWhatIsNotProved(t *testing.T) {
	env, _ := issue(t, "our fathers brought forth on this continent")
	r, err := Verify(env)
	if err != nil || len(r.Checked) != 2 || len(r.NotChecked) == 0 {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestSourceCheck(t *testing.T) {
	env, _ := issue(t, "our fathers brought forth on this continent")
	if err := CheckSource(env, []byte(src)); err == nil {
		t.Fatal("blob hash was fake, CheckSource should have failed")
	}
}

// Property: changing any signed field makes verification fail.
func TestPropTamperFails(t *testing.T) {
	prop.Check(t, 300, func(r *mrand.Rand) error {
		env, _ := issue(t, "our fathers brought forth on this continent")
		muts := []func(*Envelope){
			func(e *Envelope) { e.Body.Quote += "x" },
			func(e *Envelope) { e.Body.Verdict = verify.Drifted },
			func(e *Envelope) { e.Body.Sim-- },
			func(e *Envelope) { e.Body.Doc = "other" },
			func(e *Envelope) { e.Body.Ver++ },
			func(e *Envelope) { e.Body.Blob = "ff" },
			func(e *Envelope) { e.Body.LogHash = "ee" },
			func(e *Envelope) { e.Body.Issued = "2020-01-01T00:00:00Z" },
			func(e *Envelope) { e.Body.Spans[0].Start++ },
			func(e *Envelope) { e.Body.Spans[0].Excerpt += "!" },
			func(e *Envelope) { e.Body.Notes = []string{"added"} },
			func(e *Envelope) {
				b, _ := base64.StdEncoding.DecodeString(e.Sig)
				b[r.Intn(len(b))] ^= 1 << uint(r.Intn(8))
				e.Sig = base64.StdEncoding.EncodeToString(b)
			},
			func(e *Envelope) { // a different key signing the same body
				_, p2, _ := ed25519.GenerateKey(rand.Reader)
				e2, _ := Sign(e.Body, p2)
				e.Pub, e.Key = e2.Pub, e2.Key
			},
		}
		i := r.Intn(len(muts))
		muts[i](&env)
		if _, err := Verify(env); err == nil {
			return fmt.Errorf("mutation %d passed verification", i)
		}
		return nil
	})
}

func TestGradingMustReproduce(t *testing.T) {
	// A signer lying about the verdict is caught even with a valid signature.
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	r := verify.Check("our fathers brought forth upon this continent", verify.Prepare("gb", 1, src))
	env, _ := Sign(Body{V: 1, Kind: "revenant.receipt", Quote: "our fathers brought forth upon this continent",
		Verdict: verify.Verbatim, Doc: "gb", Ver: 1, Spans: r.Spans}, priv)
	if _, err := Verify(env); err == nil {
		t.Fatal("a drifted quote signed as verbatim must not verify")
	}
}
