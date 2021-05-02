package inspect

import (
	"context"
	"strings"

	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"

	"github.com/gotd/bot/internal/dispatch"
)

// Handler implements inspect request handler.
type Handler struct {
	fmt Formatter
}

// New creates new Handler.
func New(fmt Formatter) Handler {
	return Handler{fmt: fmt}
}

// OnMessage implements dispatch.MessageHandler.
func (h Handler) OnMessage(ctx context.Context, e dispatch.MessageEvent) error {
	return e.WithReply(ctx, func(reply *tg.Message) error {
		var w strings.Builder
		if err := h.fmt(&w, reply); err != nil {
			return xerrors.Errorf("encode message %d: %w", reply.ID, err)
		}

		if _, err := e.Reply().StyledText(ctx, styling.Pre(w.String())); err != nil {
			return xerrors.Errorf("send: %w", err)
		}

		return nil
	})
}
