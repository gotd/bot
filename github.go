package main

import (
	"context"
	"net/http/httputil"

	"github.com/google/go-github/v33/github"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

func (b *Bot) answerGH(
	ctx context.Context,
	send *message.Builder,
	peer tg.InputPeerClass,
	m *tg.Message,
) error {
	resp, err := b.github.Actions.CreateWorkflowDispatchEventByFileName(
		ctx, "gotd", "td", "bot.yml",
		github.CreateWorkflowDispatchEventRequest{
			Ref: "main",
			Inputs: map[string]interface{}{
				"telegram": true,
				"message":  m,
			},
		},
	)
	if err != nil {
		return xerrors.Errorf("dispatch workflow: %w", err)
	}

	data, err := httputil.DumpResponse(resp.Response, true)
	if err != nil {
		return xerrors.Errorf("dump response: %w", err)
	}

	if _, err := send.StyledText(ctx, styling.Pre(string(data), "")); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}
