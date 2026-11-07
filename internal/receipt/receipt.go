// Package receipt makes a quote check portable.
//
// A receipt says: "this key saw this quote in this version of this source and
// graded it so". It does NOT say the source is authentic or that the time is
// true; the signer's clock and the source text are taken as given. Anyone can
// re-check the signature and the grading offline, and, if they hold the source,
// that the source is the one the receipt names.
package receipt

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/111Reihan/kiro/internal/align"
	"github.com/111Reihan/kiro/internal/canon"
	"github.com/111Reihan/kiro/internal/verify"
)

const domain = "revenant-receipt-v1\n" // keeps these signatures from being valid anywhere else

type Body struct {
	V        int             `json:"v"`
	Kind     string          `json:"kind"`
	Issued   string          `json:"issued"` // signer's clock, self-asserted
	Quote    string          `json:"quote"`
	Verdict  verify.Verdict  `json:"verdict"`
	Severity verify.Severity `json:"severity,omitempty"`
	Sim      int             `json:"sim"`
	Elided   bool            `json:"elided,omitempty"`
	Doc      string          `json:"doc"`
	Ver      int             `json:"ver"`
	Blob     string          `json:"blob"` // SHA-256 of the source version's bytes
	LogSeq   int             `json:"log_seq"`
	LogHash  string          `json:"log_hash"` // hash of the log event that recorded that version
	Spans    []verify.Span   `json:"spans"`
	Ops      []align.Op      `json:"ops,omitempty"`
	Notes    []string        `json:"notes,omitempty"`
}

type Envelope struct {
	Body Body   `json:"body"`
	Alg  string `json:"alg"`
	Key  string `json:"key"` // first 16 hex chars of SHA-256(pub)
	Pub  string `json:"pub"` // base64
	Sig  string `json:"sig"` // base64 over domain + canonical body
}

func KeyID(pub ed25519.PublicKey) string {
	s := sha256.Sum256(pub)
	return hex.EncodeToString(s[:8])
}

func Sign(b Body, priv ed25519.PrivateKey) (Envelope, error) {
	msg, err := canon.Marshal(b)
	if err != nil {
		return Envelope{}, err
	}
	pub := priv.Public().(ed25519.PublicKey)
	return Envelope{
		Body: b, Alg: "ed25519", Key: KeyID(pub),
		Pub: base64.StdEncoding.EncodeToString(pub),
		Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, append([]byte(domain), msg...))),
	}, nil
}

// ID is the receipt's content address.
func ID(e Envelope) (string, error) {
	b, err := canon.Marshal(e)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

// Report lists what a check proved and what it could not.
type Report struct {
	Key        string   `json:"key"`
	Checked    []string `json:"checked"`
	NotChecked []string `json:"not_checked"`
}

// Verify checks the signature and that the grading can be reproduced from the
// excerpts the receipt carries.
func Verify(e Envelope) (Report, error) {
	r := Report{Key: e.Key, NotChecked: []string{
		"that the named source is authentic (only that it matches the hash)",
		"that the issue time is true (signer's own clock)",
		"who holds the key (the key id is not an identity)",
	}}
	if e.Alg != "ed25519" || e.Body.V != 1 || e.Body.Kind != "revenant.receipt" {
		return r, errors.New("not a revenant v1 receipt")
	}
	pub, err := base64.StdEncoding.DecodeString(e.Pub)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return r, errors.New("bad public key")
	}
	if KeyID(pub) != e.Key {
		return r, errors.New("key id does not match public key")
	}
	sig, err := base64.StdEncoding.DecodeString(e.Sig)
	if err != nil {
		return r, errors.New("bad signature encoding")
	}
	msg, err := canon.Marshal(e.Body)
	if err != nil {
		return r, err
	}
	if !ed25519.Verify(pub, append([]byte(domain), msg...), sig) {
		return r, errors.New("signature does not verify")
	}
	r.Checked = append(r.Checked, "signature is valid for the body")

	// Re-grade the quote against the excerpts alone.
	var ex []string
	for _, s := range e.Body.Spans {
		ex = append(ex, s.Excerpt)
	}
	got := verify.Check(e.Body.Quote, verify.Prepare(e.Body.Doc, e.Body.Ver, strings.Join(ex, " … ")))
	if got.Verdict != e.Body.Verdict || got.Severity != e.Body.Severity {
		return r, fmt.Errorf("grading does not reproduce: receipt says %s/%s, excerpts give %s/%s",
			e.Body.Verdict, e.Body.Severity, got.Verdict, got.Severity)
	}
	r.Checked = append(r.Checked, "grading reproduces from the embedded excerpts")
	return r, nil
}

// CheckSource confirms a source file is the one the receipt names.
func CheckSource(e Envelope, src []byte) error {
	s := sha256.Sum256(src)
	if hex.EncodeToString(s[:]) != e.Body.Blob {
		return errors.New("source bytes do not match the receipt's source hash")
	}
	rs := []rune(string(src))
	for _, sp := range e.Body.Spans {
		if sp.Start < 0 || sp.End > len(rs) || sp.Start > sp.End || string(rs[sp.Start:sp.End]) != sp.Excerpt {
			return fmt.Errorf("excerpt at %d-%d is not in the source", sp.Start, sp.End)
		}
	}
	return nil
}
