package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/tdp"
	"github.com/gotd/td/tg"
)

func (b *Bot) answer(ctx tg.UpdateContext, m *tg.Message, peer tg.InputPeerClass, replyMsgID int) error {
	switch {
	case strings.HasPrefix(m.Message, "/bot"):
		return b.answerWhat(ctx, peer, replyMsgID)
	case strings.HasPrefix(m.Message, "/stat"):
		return b.answerStat(ctx, peer, replyMsgID)
	case strings.HasPrefix(m.Message, "/dice"):
		return b.answerDice(ctx, peer, replyMsgID)
	case strings.HasPrefix(m.Message, "/json"):
		return b.answerInspect(ctx, peer, m, func(w io.Writer, m *tg.Message) error {
			encoder := json.NewEncoder(w)
			encoder.SetIndent("", "\t")
			return encoder.Encode(m)
		})
	case strings.HasPrefix(m.Message, "/pprint"), strings.HasPrefix(m.Message, "/pp"):
		return b.answerInspect(ctx, peer, m, func(w io.Writer, m *tg.Message) error {
			if _, err := io.WriteString(w, tdp.Format(m, tdp.WithTypeID)); err != nil {
				return err
			}

			return nil
		})
	default:
		// Ignoring.
		return nil
	}
}

func (b *Bot) answerWhat(ctx tg.UpdateContext, peer tg.InputPeerClass, replyMsgID int) error {
	if err := b.sendMessage(ctx, &tg.MessagesSendMessageRequest{
		Message:      "What?",
		Peer:         peer,
		ReplyToMsgID: replyMsgID,
	}); err != nil {
		return xerrors.Errorf("send: %w", err)
	}
	return nil
}

func (b *Bot) answerDice(ctx tg.UpdateContext, peer tg.InputPeerClass, replyMsgID int) error {
	if err := b.sendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer:         peer,
		ReplyToMsgID: replyMsgID,
		Media:        &tg.InputMediaDice{Emoticon: "🎲"},
	}); err != nil {
		return xerrors.Errorf("send media: %w", err)
	}

	return nil
}

func (b *Bot) getChannelMessage(ctx context.Context, channel *tg.InputChannel, msgID int) (*tg.Message, error) {
	r, err := b.rpc.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
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

type formatter func(io.Writer, *tg.Message) error

func (b *Bot) answerInspect(ctx tg.UpdateContext, peer tg.InputPeerClass, m *tg.Message, f formatter) error {
	h, ok := m.GetReplyTo()
	if !ok {
		if err := b.sendMessage(ctx, &tg.MessagesSendMessageRequest{
			Message:      "Message must be a reply",
			Peer:         peer,
			ReplyToMsgID: m.ID,
		}); err != nil {
			return xerrors.Errorf("send: %w", err)
		}

		return nil
	}

	channel, ok := peer.(*tg.InputPeerChannel)
	if !ok {
		// Skip non-channel messages.
		return nil
	}

	b.logger.Info("Fetching message",
		zap.Int("msg_id", m.ID),
		zap.Int("reply_to_msg_id", h.ReplyToMsgID),
		zap.Int("channel_id", channel.ChannelID),
	)

	msg, err := b.getChannelMessage(ctx, &tg.InputChannel{
		ChannelID:  channel.ChannelID,
		AccessHash: channel.AccessHash,
	}, h.ReplyToMsgID)
	if err != nil {
		if err := b.sendMessage(ctx, &tg.MessagesSendMessageRequest{
			Message:      fmt.Sprintf("Message %d not found", h.ReplyToMsgID),
			Peer:         peer,
			ReplyToMsgID: m.ID,
		}); err != nil {
			return xerrors.Errorf("send: %w", err)
		}

		return nil
	}

	var w strings.Builder
	if err := f(&w, msg); err != nil {
		return xerrors.Errorf("encode message %d: %w", msg.ID, err)
	}

	s := w.String()
	req := &tg.MessagesSendMessageRequest{
		Message:      s,
		Peer:         peer,
		ReplyToMsgID: msg.ID,
	}

	req.SetEntities(formatMessage(s, func(offset, limit int) tg.MessageEntityClass {
		return &tg.MessageEntityPre{Offset: offset, Length: limit}
	}))

	if err := b.sendMessage(ctx, req); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}

func (b *Bot) answerStat(ctx tg.UpdateContext, peer tg.InputPeerClass, replyMsgID int) error {
	var w strings.Builder
	fmt.Fprintf(&w, "Statistics:\n\n")
	fmt.Fprintln(&w, "Messages:", b.m.Messages.Load())
	fmt.Fprintln(&w, "Responses:", b.m.Responses.Load())
	fmt.Fprintln(&w, "Media:", humanize.IBytes(uint64(b.m.MediaBytes.Load())))
	fmt.Fprintln(&w, "Uptime:", time.Since(b.m.Start).Round(time.Second))
	if v := getVersion(); v != "" {
		fmt.Fprintln(&w, "Version:", v)
	}

	if err := b.sendMessage(ctx, &tg.MessagesSendMessageRequest{
		Message:      w.String(),
		Peer:         peer,
		ReplyToMsgID: replyMsgID,
	}); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}
