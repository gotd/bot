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
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

func (b *Bot) answer(ctx tg.UpdateContext, m *tg.Message, peer tg.InputPeerClass) error {
	send := b.sender.Peer(peer).ReplyMsg(m)
	switch {
	case strings.HasPrefix(m.Message, "/bot"):
		if _, err := send.Text(ctx, "What?"); err != nil {
			return xerrors.Errorf("answer text: %w", err)
		}
	case strings.HasPrefix(m.Message, "/stat"):
		if _, err := send.Text(ctx, b.stats()); err != nil {
			return xerrors.Errorf("answer stats: %w", err)
		}
	case strings.HasPrefix(m.Message, "/dice"):
		if _, err := send.Dice(ctx); err != nil {
			return xerrors.Errorf("answer dice: %w", err)
		}
	case strings.HasPrefix(m.Message, "/basketball"):
		if _, err := send.Basketball(ctx); err != nil {
			return xerrors.Errorf("answer basketball: %w", err)
		}
	case strings.HasPrefix(m.Message, "/darts"):
		if _, err := send.Darts(ctx); err != nil {
			return xerrors.Errorf("answer darts: %w", err)
		}
	case strings.HasPrefix(m.Message, "/tts"):
		lang := "en"
		cmd := strings.ToLower(m.Message)
		cmd = strings.TrimSuffix(strings.TrimSpace(cmd), "@gotd_echo_bot")
		if strings.HasPrefix(cmd, "/tts_") {
			lang = strings.TrimSpace(strings.TrimPrefix(cmd, "/tts_"))
		}

		if err := b.answerTTS(ctx, send, peer, m, lang); err != nil {
			return xerrors.Errorf("answer tts: %w", err)
		}
	case strings.HasPrefix(m.Message, "/gpt2"):
		if err := b.answerGPT2(ctx, send, peer, m); err != nil {
			return xerrors.Errorf("answer gpt2: %w", err)
		}
	case strings.HasPrefix(m.Message, "/json"):
		if err := b.answerInspect(ctx, send, peer, m, func(w io.Writer, m *tg.Message) error {
			encoder := json.NewEncoder(w)
			encoder.SetIndent("", "\t")
			return encoder.Encode(m)
		}); err != nil {
			return xerrors.Errorf("answer inspect: %w", err)
		}
	case strings.HasPrefix(m.Message, "/pprint"), strings.HasPrefix(m.Message, "/pp"):
		if err := b.answerInspect(ctx, send, peer, m, func(w io.Writer, m *tg.Message) error {
			if _, err := io.WriteString(w, tdp.Format(m, tdp.WithTypeID)); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return xerrors.Errorf("answer inspect: %w", err)
		}
	default:
		// Ignoring.
		return nil
	}

	// Increasing total response count metric.
	b.m.Responses.Inc()
	return nil
}

func (b *Bot) stats() string {
	var w strings.Builder
	fmt.Fprintf(&w, "Statistics:\n\n")
	fmt.Fprintln(&w, "Messages:", b.m.Messages.Load())
	fmt.Fprintln(&w, "Responses:", b.m.Responses.Load())
	fmt.Fprintln(&w, "Media:", humanize.IBytes(uint64(b.m.MediaBytes.Load())))
	fmt.Fprintln(&w, "Uptime:", time.Since(b.m.Start).Round(time.Second))
	if v := getVersion(); v != "" {
		fmt.Fprintln(&w, "Version:", v)
	}

	return w.String()
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

func (b *Bot) getReply(
	ctx tg.UpdateContext,
	send *message.Builder,
	peer tg.InputPeerClass,
	m *tg.Message,
	cb func(msg *tg.Message) error,
) error {
	h, ok := m.GetReplyTo()
	if !ok {
		if _, err := send.Text(ctx, "Message must be a reply"); err != nil {
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
		if _, err := send.Text(ctx, fmt.Sprintf("Message %d not found", h.ReplyToMsgID)); err != nil {
			return xerrors.Errorf("send: %w", err)
		}
		return nil
	}

	return cb(msg)
}

type formatter func(io.Writer, *tg.Message) error

func (b *Bot) answerInspect(
	ctx tg.UpdateContext,
	send *message.Builder,
	peer tg.InputPeerClass,
	m *tg.Message, f formatter,
) error {
	return b.getReply(ctx, send, peer, m, func(msg *tg.Message) error {
		var w strings.Builder
		if err := f(&w, msg); err != nil {
			return xerrors.Errorf("encode message %d: %w", msg.ID, err)
		}

		if _, err := send.StyledText(ctx, message.Pre(w.String(), "")); err != nil {
			return xerrors.Errorf("send: %w", err)
		}

		return nil
	})
}
