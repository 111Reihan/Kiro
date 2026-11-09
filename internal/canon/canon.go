// Package canon writes JSON the same way every time: object keys sorted,
// no HTML escaping, no trailing newline. Hashes and signatures are taken over
// these bytes ("specter-canon-v1"). It is deliberately not RFC 8785; numbers
// are only ever integers here, so the difference never shows.
package canon

import (
	"bytes"
	"encoding/json"
)

func enc(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// Marshal round-trips v through a generic value so struct field order stops
// mattering and map keys come out sorted.
func Marshal(v any) ([]byte, error) {
	raw, err := enc(v)
	if err != nil {
		return nil, err
	}
	var x any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&x); err != nil {
		return nil, err
	}
	return enc(x)
}
