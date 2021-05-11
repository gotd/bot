package main

import (
	"context"

	"go.uber.org/zap"

	"github.com/gotd/contrib/updates"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

var _ updates.Handler = (*GapAdapter)(nil)

type GapAdapter struct {
	next telegram.UpdateHandler
	ctx  context.Context
	log  *zap.Logger
}

func NewGapAdapter(
	ctx context.Context,
	log *zap.Logger,
	next telegram.UpdateHandler,
) *GapAdapter {
	return &GapAdapter{next, ctx, log}
}

func (a *GapAdapter) HandleDiff(diff updates.DiffUpdate) error {
	return a.next.Handle(a.ctx, &tg.Updates{
		Updates: append(
			append(
				msgsToUpdates(diff.NewMessages),
				encryptedMsgsToUpdates(diff.NewEncryptedMessages)...,
			),
			diff.OtherUpdates...,
		),
		Users: diff.Users,
		Chats: diff.Chats,
	})
}

func (a *GapAdapter) HandleUpdates(u updates.Updates) error {
	return a.next.Handle(a.ctx, &tg.Updates{
		Updates: u.Updates,
		Users:   u.Ents.AsUsers(),
		Chats:   u.Ents.AsChats(),
	})
}

func (a *GapAdapter) ChannelTooLong(channelID int) {
	a.log.Warn("ChannelTooLong", zap.Int("channel_id", channelID))
}

func msgsToUpdates(msgs []tg.MessageClass) []tg.UpdateClass {
	var updates []tg.UpdateClass
	for _, msg := range msgs {
		updates = append(updates, &tg.UpdateNewMessage{
			Message:  msg,
			Pts:      -1,
			PtsCount: -1,
		})
	}
	return updates
}

func encryptedMsgsToUpdates(msgs []tg.EncryptedMessageClass) []tg.UpdateClass {
	var updates []tg.UpdateClass
	for _, msg := range msgs {
		updates = append(updates, &tg.UpdateNewEncryptedMessage{
			Message: msg,
			Qts:     -1,
		})
	}
	return updates
}
