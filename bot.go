package main

import (
	"fmt"

	"github.com/gotd/td/telegram"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/tg"
)

type Bot struct {
	state  *State
	client *telegram.Client

	logger *zap.Logger
}

func NewBot(state *State, client *telegram.Client) *Bot {
	return &Bot{state: state, client: client, logger: zap.NewNop()}
}

// WithLogger sets logger.
func (b *Bot) WithLogger(logger *zap.Logger) *Bot {
	b.logger = logger
	return b
}

func (b *Bot) handleUser(ctx tg.UpdateContext, user *tg.User, m *tg.Message) error {
	b.logger.With(
		zap.String("text", m.Message),
		zap.Int("user_id", user.ID),
		zap.String("user_first_name", user.FirstName),
		zap.String("username", user.Username),
	).Info("Got message")

	reply := &tg.MessagesSendMessageRequest{
		Message: fmt.Sprintf("No u %s, @%s", m.Message, user.Username),
		Peer: &tg.InputPeerUser{
			UserID:     user.ID,
			AccessHash: user.AccessHash,
		},
	}
	reply.SetReplyToMsgID(m.ID)

	if err := b.client.SendMessage(ctx, reply); err != nil {
		if tg.IsUserBlocked(err) {
			b.logger.Debug("Bot is blocked by user")
			return nil
		}

		return xerrors.Errorf("send message: %w", err)
	}

	return nil
}

func (b *Bot) answerWhat(ctx tg.UpdateContext, peer tg.InputPeerClass, to int) error {
	reply := &tg.MessagesSendMessageRequest{
		Message: "What?",
		Peer:    peer,
	}
	reply.SetReplyToMsgID(to)

	if err := b.client.SendMessage(ctx, reply); err != nil {
		if tg.IsUserBlocked(err) {
			b.logger.Debug("Bot is blocked by user")
			return nil
		}

		return xerrors.Errorf("send message: %w", err)
	}

	return nil
}

func (b *Bot) handleChat(ctx tg.UpdateContext, peer *tg.Chat, m *tg.Message) error {
	b.logger.With(
		zap.String("text", m.Message),
		zap.Int("channel_id", peer.ID),
	).Info("Got message from chat")

	return b.answerWhat(ctx, &tg.InputPeerChat{
		ChatID: peer.ID,
	}, m.ID)
}

func (b *Bot) handleChannel(ctx tg.UpdateContext, peer *tg.Channel, m *tg.Message) error {
	b.logger.With(
		zap.String("text", m.Message),
		zap.Int("channel_id", peer.ID),
	).Info("Got message from channel")

	return b.answerWhat(ctx, &tg.InputPeerChannel{
		ChannelID:  peer.ID,
		AccessHash: peer.AccessHash,
	}, m.ID)
}

func (b *Bot) Handle(ctx tg.UpdateContext, msg tg.MessageClass) error {
	switch m := msg.(type) {
	case *tg.Message:
		if m.Out {
			return nil
		}

		switch peer := m.PeerID.(type) {
		case *tg.PeerUser:
			user, ok := ctx.Users[peer.UserID]
			if !ok {
				return xerrors.Errorf("unknown user ID %d", peer.UserID)
			}
			return b.handleUser(ctx, user, m)
		case *tg.PeerChat:
			chat, ok := ctx.Chats[peer.ChatID]
			if !ok {
				return xerrors.Errorf("unknown chat ID %d", peer.ChatID)
			}
			return b.handleChat(ctx, chat, m)
		case *tg.PeerChannel:
			channel, ok := ctx.Channels[peer.ChannelID]
			if !ok {
				return xerrors.Errorf("unknown channel ID %d", peer.ChannelID)
			}
			return b.handleChannel(ctx, channel, m)
		}
	}

	return nil
}

func (b *Bot) OnNewMessage(ctx tg.UpdateContext, u *tg.UpdateNewMessage) error {
	if err := b.Handle(ctx, u.Message); err != nil {
		return xerrors.Errorf("handle: %w", err)
	}

	if err := b.state.Commit(u.Pts); err != nil {
		return xerrors.Errorf("commit: %w", err)
	}
	return nil
}

func (b *Bot) OnNewChannelMessage(ctx tg.UpdateContext, u *tg.UpdateNewChannelMessage) error {
	if err := b.Handle(ctx, u.Message); err != nil {
		return xerrors.Errorf("handle: %w", err)
	}

	if err := b.state.Commit(u.Pts); err != nil {
		return xerrors.Errorf("commit: %w", err)
	}
	return nil
}
