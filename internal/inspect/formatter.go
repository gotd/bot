package inspect

import (
	"encoding/json"
	"io"

	"github.com/gotd/td/tdp"
	"github.com/gotd/td/tg"
)

// Formatter is a message formatter.
type Formatter func(io.Writer, *tg.Message) error

// JSON returns JSON inspect handler.
func JSON() Handler {
	return New(func(w io.Writer, m *tg.Message) error {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(m)
	})
}

// Pretty returns tdp-based inspect handler.
func Pretty() Handler {
	return New(func(w io.Writer, m *tg.Message) error {
		if _, err := io.WriteString(w, tdp.Format(m, tdp.WithTypeID)); err != nil {
			return err
		}

		return nil
	})
}
