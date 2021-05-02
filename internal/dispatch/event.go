package dispatch

import (
	"context"
	"fmt"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"golang.org/x/xerrors"
)

// MessageEvent represents message event.
type MessageEvent struct {
	Peer    tg.InputPeerClass
	Message *tg.Message
	sender  *message.Sender
	logger  *zap.Logger
	rpc     *tg.Client
}

// Logger returns associated logger.
func (e MessageEvent) Logger() *zap.Logger {
	return e.logger
}

// RPC returns Telegram RPC client.
func (e MessageEvent) RPC() *tg.Client {
	return e.rpc
}

// Sender returns *message.Sender
func (e MessageEvent) Sender() *message.Sender {
	return e.sender
}

func (e MessageEvent) getChannelMessage(ctx context.Context, channel *tg.InputChannel, msgID int) (*tg.Message, error) {
	r, err := e.rpc.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: channel,
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}},
	})
	if err != nil {
		return nil, xerrors.Errorf("get message: %w", err)
	}

	slice, ok := r.(interface{ GetMessages() []tg.MessageClass })
	if !ok {
		return nil, xerrors.Errorf("unexpected type %T", r)
	}

	msgs := slice.GetMessages()
	if len(msgs) < 1 {
		return nil, xerrors.Errorf("unexpected empty response %+v", msgs)
	}

	msg, ok := msgs[0].(*tg.Message)
	if !ok {
		return nil, xerrors.Errorf("unexpected type %T", msg)
	}

	return msg, nil
}

// WithReply calls given callback if current message event is a reply message.
func (e MessageEvent) WithReply(ctx context.Context, cb func(reply *tg.Message) error) error {
	h, ok := e.Message.GetReplyTo()
	if !ok {
		if _, err := e.Reply().Text(ctx, "Message must be a reply"); err != nil {
			return xerrors.Errorf("send: %w", err)
		}
		return nil
	}

	channel, ok := e.Peer.(*tg.InputPeerChannel)
	if !ok {
		// Skip non-channel messages.
		return nil
	}

	e.logger.Info("Fetching message",
		zap.Int("msg_id", e.Message.ID),
		zap.Int("reply_to_msg_id", h.ReplyToMsgID),
		zap.Int("channel_id", channel.ChannelID),
	)

	msg, err := e.getChannelMessage(ctx, &tg.InputChannel{
		ChannelID:  channel.ChannelID,
		AccessHash: channel.AccessHash,
	}, h.ReplyToMsgID)
	if err != nil {
		if _, err := e.Reply().Text(ctx, fmt.Sprintf("Message %d not found", h.ReplyToMsgID)); err != nil {
			return xerrors.Errorf("send: %w", err)
		}
		return nil
	}

	return cb(msg)
}

// Reply creates new message builder to reply.
func (e MessageEvent) Reply() *message.Builder {
	return e.sender.To(e.Peer).ReplyMsg(e.Message)
}
