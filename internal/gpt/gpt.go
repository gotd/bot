package gpt

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"

	"github.com/gotd/bot/internal/dispatch"
	"github.com/gotd/bot/internal/gentext"
)

// Handler implements GPT request handler.
type Handler struct {
	net gentext.Net
}

// New creates new Handler.
func New(net gentext.Net) Handler {
	return Handler{net: net}
}

// OnMessage implements dispatch.MessageHandler.
func (h Handler) OnMessage(ctx context.Context, e dispatch.MessageEvent) error {
	return e.WithReply(ctx, func(reply *tg.Message) error {
		prompt := reply.GetMessage()
		result, err := h.net.Query(ctx, prompt)
		if err != nil {
			if _, err := e.Reply().Text(ctx, "GPT server request failed"); err != nil {
				return xerrors.Errorf("send: %w", err)
			}
			return xerrors.Errorf("send GPT request: %w", err)
		}

		_, err = e.Reply().StyledText(ctx,
			styling.Bold(prompt),
			styling.Plain(result),
		)
		return err
	})
}
