// Package demo builds a small project with a believable amount of drift, so a
// public instance has something to show. It uses the real scanner and ledger:
// the seals are genuine, the edits afterwards are genuine, nothing is faked
// in the status computation.
package demo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/111Reihan/kiro/internal/engine"
	"github.com/111Reihan/kiro/internal/ledger"
	"github.com/111Reihan/kiro/internal/spec"
)

type file struct{ path, body string }

func put(root string, fs []file) error {
	for _, f := range fs {
		p := filepath.Join(root, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(f.body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func goJSON(rs [][3]string) string {
	var b strings.Builder
	for _, r := range rs {
		fmt.Fprintf(&b, "{\"Action\":%q,\"Package\":%q,\"Test\":%q}\n", r[0], r[1], r[2])
	}
	return b.String()
}

// Build writes the demo project under root/project and its ledger under
// root/data, and returns the two paths.
func Build(root string) (project, data string, err error) {
	project, data = filepath.Join(root, "project"), filepath.Join(root, "data")
	e, err := engine.New(data)
	if err != nil {
		return "", "", err
	}
	l := ledger.New(e)

	// day one: requirements are written, tests prove them, a human seals them
	if err := put(project, day1); err != nil {
		return "", "", err
	}
	if err := step(l, project, goJSON([][3]string{
		{"pass", "shop/checkout", "TestConfirmLatency"}, {"pass", "shop/checkout", "TestDeclineCreatesNoOrder"},
		{"pass", "shop/checkout", "TestRetryLimit"}, {"pass", "shop/checkout", "TestQueueWhileDown"},
		{"pass", "shop/checkout", "TestChargeLog"}, {"pass", "shop/checkout", "TestShipEmail"},
		{"pass", "shop/auth", "TestLockout"}, {"pass", "shop/auth", "TestPasswordHash"},
	}), true); err != nil {
		return "", "", err
	}

	// weeks later: the spec moved on, the code and tests did not keep up
	if err := put(project, day2); err != nil {
		return "", "", err
	}
	if err := os.Remove(filepath.Join(project, "specs/checkout/tests/chargelog_test.go")); err != nil {
		return "", "", err
	}
	if err := step(l, project, goJSON([][3]string{
		{"pass", "shop/checkout", "TestConfirmLatency"}, {"pass", "shop/checkout", "TestDeclineCreatesNoOrder"},
		{"pass", "shop/checkout", "TestRetryLimit"}, {"pass", "shop/checkout", "TestQueueWhileDown"},
		{"pass", "shop/notify", "TestShipEmail"},
		{"fail", "shop/auth", "TestLockout"}, {"pass", "shop/auth", "TestPasswordHash"},
	}), false); err != nil {
		return "", "", err
	}
	return project, data, nil
}

func step(l *ledger.Ledger, project, results string, seal bool) error {
	p, err := spec.Scan(project)
	if err != nil {
		return err
	}
	if _, err := l.Record(p); err != nil {
		return err
	}
	rs, err := spec.ParseResults([]byte(results))
	if err != nil {
		return err
	}
	if _, err := l.AddRun(rs, "go test -json ./..."); err != nil {
		return err
	}
	if seal {
		_, err = l.Seal(p, nil)
	}
	return err
}

var day1 = []file{
	{"specs/checkout/requirements.md", `# Checkout

### Requirement 1: Payment

1. WHEN a customer submits an order THEN the system SHALL confirm payment within 2 seconds.
2. IF the card is declined THEN the system SHALL NOT create an order.

### Requirement 2: Resilience

1. The system SHALL retry a failed charge at most 3 times.
2. WHILE the payment provider is unavailable the system SHALL queue orders for up to 15 minutes.
3. The system SHALL log every charge attempt.
4. WHEN an order ships THEN the system SHALL email the customer.
`},
	{"specs/auth/requirements.md", `# Authentication

### Requirement 1: Accounts

1. The system SHALL lock an account after 5 failed sign-in attempts.
2. The system SHALL store passwords using a salted hash.
3. The system SHALL support sign-in with a passkey.
`},
	{"specs/checkout/tests/checkout_test.go", `package checkout

import "testing"

// covers: checkout/1.1
func TestConfirmLatency(t *testing.T) {
	limit := 2
	if limit != 2 {
		t.Fatal("slow")
	}
}

// covers: checkout/1.2
func TestDeclineCreatesNoOrder(t *testing.T) {
	orders := 0
	if orders != 0 {
		t.Fatal("order created")
	}
}

// covers: checkout/2.1
func TestRetryLimit(t *testing.T) {
	const max = 3
	_ = max
}

// covers: checkout/2.2
func TestQueueWhileDown(t *testing.T) {
	const minutes = 15
	_ = minutes
}
`},
	{"specs/checkout/tests/chargelog_test.go", `package checkout

import "testing"

// covers: checkout/2.3
func TestChargeLog(t *testing.T) {
	_ = "every attempt is logged"
}

// covers: checkout/2.4
func TestShipEmail(t *testing.T) {
	_ = "email sent on ship"
}
`},
	{"specs/auth/tests/auth_test.go", `package auth

import "testing"

// covers: auth/1.1
func TestLockout(t *testing.T) {
	const attempts = 5
	_ = attempts
}

// covers: auth/1.2
func TestPasswordHash(t *testing.T) {
	_ = "argon2id with a per-user salt"
}
`},
}

var day2 = []file{
	{"specs/checkout/requirements.md", `# Checkout

### Requirement 1: Payment

1. WHEN a customer submits an order THEN the system SHALL confirm payment within 5 seconds.
2. IF the card is declined THEN the system SHALL create a pending order.

### Requirement 2: Resilience

1. The system SHALL retry a failed charge at most 3 times.
2. WHILE the payment provider is unavailable the system SHALL queue orders for up to 15 minutes.
`},
	{"specs/notify/requirements.md", `# Notifications

### Requirement 1: Email

1. WHEN an order ships THEN the system SHALL email the customer.
2. The system SHALL be user-friendly.
`},
	{"specs/notify/tests/notify_test.go", `package notify

import "testing"

// covers: notify/1.1
func TestShipEmail(t *testing.T) {
	_ = "email sent on ship"
}
`},
	{"specs/auth/tests/auth_test.go", `package auth

import "testing"

// covers: auth/1.1
func TestLockout(t *testing.T) {
	const attempts = 5
	_ = attempts
}

// covers: auth/1.2
func TestPasswordHash(t *testing.T) {
	_ = "bcrypt cost 4"
}
`},
}
