package dispatch

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

// MessageEvent represents message event.
type MessageEvent struct {
	Peer    tg.InputPeerClass
	Message *tg.Message

	user    *tg.User
	chat    *tg.Chat
	channel *tg.Channel

	sender *message.Sender
	logger *zap.Logger
	rpc    *tg.Client
}

// User returns User object and true if message got from user.
// False and nil otherwise.
func (e MessageEvent) User() (*tg.User, bool) {
	return e.user, e.user != nil
}

// Chat returns Chat object and true if message got from chat.
// False and nil otherwise.
func (e MessageEvent) Chat() (*tg.Chat, bool) {
	return e.chat, e.chat != nil
}

// Channel returns Channel object and true if message got from channel.
// False and nil otherwise.
func (e MessageEvent) Channel() (*tg.Channel, bool) {
	return e.channel, e.channel != nil
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

func (e MessageEvent) getMessage(ctx context.Context, msgID int) (*tg.Message, error) {
	r, err := e.rpc.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}})
	if err != nil {
		return nil, xerrors.Errorf("get message: %w", err)
	}

	return findMessage(r, msgID)
}

func (e MessageEvent) getChannelMessage(ctx context.Context, channel *tg.InputChannel, msgID int) (*tg.Message, error) {
	r, err := e.rpc.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: channel,
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}},
	})
	if err != nil {
		return nil, xerrors.Errorf("get message: %w", err)
	}

	return findMessage(r, msgID)
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

	var (
		msg *tg.Message
		err error
		log = e.logger.With(
			zap.Int("msg_id", e.Message.ID),
			zap.Int("reply_to_msg_id", h.ReplyToMsgID),
		)
	)
	switch p := e.Peer.(type) {
	case *tg.InputPeerChannel:
		log.Info("Fetching message", zap.Int("channel_id", p.ChannelID))

		msg, err = e.getChannelMessage(ctx, &tg.InputChannel{
			ChannelID:  p.ChannelID,
			AccessHash: p.AccessHash,
		}, h.ReplyToMsgID)
	case *tg.InputPeerChat:
		log.Info("Fetching message", zap.Int("chat_id", p.ChatID))

		msg, err = e.getMessage(ctx, h.ReplyToMsgID)
	case *tg.InputPeerUser:
		log.Info("Fetching message", zap.Int("chat_id", p.UserID))

		msg, err = e.getMessage(ctx, h.ReplyToMsgID)
	}
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
