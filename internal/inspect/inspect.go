package inspect

import (
	"context"
	"strings"

	"github.com/go-faster/errors"

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
			return errors.Wrapf(err, "encode message %d", reply.ID)
		}

		if _, err := e.Reply().StyledText(ctx, styling.Code(w.String())); err != nil {
			return errors.Wrap(err, "send")
		}

		return nil
	})
}
