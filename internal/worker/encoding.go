package worker

import (
	"encoding/base64"

	"github.com/fastschema/qjs"
)

// setupEncoding registers global atob() and btoa() functions matching the
// Web API specification. These are Go-backed for correctness and performance.
func setupEncoding(rt *qjs.Runtime) error {
	ctx := rt.Context()

	// atob(data) — decodes a base64-encoded string to a binary (Latin-1) string.
	// Each decoded byte becomes a Unicode code point 0-255 in the result.
	ctx.SetFunc("atob", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 1 {
			return nil, errMissingArg("atob", 1)
		}

		encoded := args[0].String()
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			// Try with padding tolerance (matches browser behavior).
			decoded, err = base64.RawStdEncoding.DecodeString(encoded)
			if err != nil {
				return nil, errInvalidArg("atob", "invalid base64 string")
			}
		}

		// Convert raw bytes to a string of Latin-1 code points.
		// Each byte 0-255 becomes a single Unicode code point, NOT a UTF-8
		// multi-byte sequence. This matches browser atob() behavior.
		runes := make([]rune, len(decoded))
		for i, b := range decoded {
			runes[i] = rune(b)
		}
		return c.NewString(string(runes)), nil
	})

	// btoa(data) — encodes a binary (Latin-1) string to base64.
	// Each character's Unicode code point (must be 0-255) becomes one byte.
	ctx.SetFunc("btoa", func(this *qjs.This) (*qjs.Value, error) {
		c := this.Context()
		args := this.Args()
		if len(args) < 1 {
			return nil, errMissingArg("btoa", 1)
		}

		input := args[0].String()
		// Extract Latin-1 byte values from the rune sequence.
		// Go's String() from QJS returns UTF-8, but the JS string contains
		// code points 0-255 (Latin-1). We must iterate runes, not bytes.
		bytes := make([]byte, 0, len(input))
		for _, r := range input {
			if r > 255 {
				return nil, errInvalidArg("btoa", "string contains characters outside of the Latin1 range")
			}
			bytes = append(bytes, byte(r))
		}

		encoded := base64.StdEncoding.EncodeToString(bytes)
		return c.NewString(encoded), nil
	})

	return nil
}
