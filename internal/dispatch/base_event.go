package dispatch

import (
	"context"
	"io"

	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

func (b *Bot) baseEvent() baseEvent {
	return baseEvent{
		sender: b.sender,
		logger: b.logger,
		rpc:    b.rpc,
		rand:   b.rand,
	}
}

type baseEvent struct {
	sender *message.Sender
	logger *zap.Logger
	rpc    *tg.Client
	rand   io.Reader
}

// Logger returns associated logger.
func (e baseEvent) Logger() *zap.Logger {
	return e.logger
}

// RPC returns Telegram RPC client.
func (e baseEvent) RPC() *tg.Client {
	return e.rpc
}

// Sender returns *message.Sender
func (e baseEvent) Sender() *message.Sender {
	return e.sender
}

func findMessage(r tg.MessagesMessagesClass, msgID int) (*tg.Message, error) {
	slice, ok := r.(interface{ GetMessages() []tg.MessageClass })
	if !ok {
		return nil, xerrors.Errorf("unexpected type %T", r)
	}

	msgs := slice.GetMessages()
	for _, m := range msgs {
		msg, ok := m.(*tg.Message)
		if !ok || msg.ID != msgID {
			continue
		}

		return msg, nil
	}

	return nil, xerrors.Errorf("message %d not found in response %+v", msgID, msgs)
}

func (e baseEvent) getMessage(ctx context.Context, msgID int) (*tg.Message, error) {
	r, err := e.rpc.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}})
	if err != nil {
		return nil, xerrors.Errorf("get message: %w", err)
	}

	return findMessage(r, msgID)
}

func (e baseEvent) getChannelMessage(ctx context.Context, channel *tg.InputChannel, msgID int) (*tg.Message, error) {
	r, err := e.rpc.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: channel,
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}},
	})
	if err != nil {
		return nil, xerrors.Errorf("get message: %w", err)
	}

	return findMessage(r, msgID)
}
