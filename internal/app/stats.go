package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/gotd/td/tg"

	"github.com/gotd/bot/internal/dispatch"
)

// Handler implements stats request handler.
type Handler struct {
}

// NewHandler creates new Handler.
func NewHandler() Handler {
	return Handler{}
}

func (h Handler) stats() string {
	var w strings.Builder
	fmt.Fprintf(&w, "Statistics:\n\n")
	fmt.Fprintln(&w, "TL Layer version:", tg.Layer)
	if v := GetVersion(); v != "" {
		fmt.Fprintln(&w, "Version:", v)
	}

	return w.String()
}

// OnMessage implements dispatch.MessageHandler.
func (h Handler) OnMessage(ctx context.Context, e dispatch.MessageEvent) error {
	_, err := e.Reply().Text(ctx, h.stats())
	return err
}
