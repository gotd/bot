package main

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/gotd/bot/net"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

func (b *Bot) answerNet(
	ctx context.Context,
	send *message.Builder,
	peer tg.InputPeerClass,
	m *tg.Message,
	gpt net.Net,
) error {
	return b.getReply(ctx, send, peer, m, func(msg *tg.Message) error {
		prompt := msg.GetMessage()
		result, err := gpt.Query(ctx, prompt)
		if err != nil {
			if _, err := send.Text(ctx, "GPT server request failed"); err != nil {
				return xerrors.Errorf("send: %w", err)
			}
			return xerrors.Errorf("send GPT request: %w", err)
		}

		_, err = b.sender.To(peer).ReplyMsg(msg).StyledText(ctx,
			styling.Bold(prompt),
			styling.Plain(result),
		)
		return err
	})
}
