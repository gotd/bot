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

	baseEvent
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

// MessageHandler is a simple message event handler.
type MessageHandler interface {
	OnMessage(ctx context.Context, e MessageEvent) error
}

// MessageHandlerFunc is a functional adapter for Handler.
type MessageHandlerFunc func(ctx context.Context, e MessageEvent) error

// OnMessage implements MessageHandler.
func (h MessageHandlerFunc) OnMessage(ctx context.Context, e MessageEvent) error {
	return h(ctx, e)
}
