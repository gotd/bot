package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/go-github/v33/github"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"

	"github.com/gotd/bot/net"
)

type Bot struct {
	state  *State
	client *telegram.Client

	rpc        *tg.Client
	sender     *message.Sender
	downloader *downloader.Downloader
	http       *http.Client

	gpt2 net.Net
	gpt3 net.Net

	github *github.Client

	logger       *zap.Logger
	m            Metrics
	githubSecret string
}

func NewBot(state *State, client *telegram.Client, metrics Metrics) *Bot {
	raw := tg.NewClient(client)
	httpClient := http.DefaultClient
	return &Bot{
		state:      state,
		client:     client,
		rpc:        raw,
		sender:     message.NewSender(raw),
		downloader: downloader.NewDownloader(),
		http:       httpClient,
		logger:     zap.NewNop(),
		m:          metrics,
		gpt2:       net.NewGPT2().WithClient(httpClient),
		gpt3:       net.NewGPT3().WithClient(httpClient),
	}
}

// WithLogger sets logger.
func (b *Bot) WithLogger(logger *zap.Logger) *Bot {
	b.logger = logger
	return b
}

func (b *Bot) WithStart(t time.Time) *Bot {
	b.m.Start = t
	return b
}

func (b *Bot) WithGPT2(g net.Net) *Bot {
	b.gpt2 = g
	return b
}

func (b *Bot) WithGPT3(g net.Net) *Bot {
	b.gpt3 = g
	return b
}

func (b *Bot) WithGithub(gh *github.Client) *Bot {
	b.github = gh
	return b
}

func (b *Bot) WithGithubSecret(secret string) *Bot {
	b.githubSecret = secret
	return b
}

func (b *Bot) handleUser(ctx context.Context, user *tg.User, m *tg.Message) error {
	b.logger.Info("Got message",
		zap.String("text", m.Message),
		zap.Int("user_id", user.ID),
		zap.String("user_first_name", user.FirstName),
		zap.String("username", user.Username),
	)

	if _, err := b.sender.To(user.AsInputPeer()).
		Textf(ctx, "No u %s, @%s", m.Message, user.Username); err != nil {
		return xerrors.Errorf("send: %w", err)
	}

	return nil
}

func maxSize(sizes []tg.PhotoSizeClass) string {
	var (
		maxSize string
		maxH    int
	)

	for _, size := range sizes {
		if s, ok := size.(interface {
			GetH() int
			GetType() string
		}); ok && s.GetH() > maxH {
			maxH = s.GetH()
			maxSize = s.GetType()
		}
	}

	return maxSize
}

func (b *Bot) downloadMedia(ctx context.Context, loc tg.InputFileLocationClass) error {
	h := sha256.New()
	metric := &metricWriter{}

	if _, err := b.downloader.Download(b.rpc, loc).
		Stream(ctx, io.MultiWriter(h, metric)); err != nil {
		return xerrors.Errorf("stream: %w", err)
	}

	b.logger.Info("Downloaded media",
		zap.Int64("bytes", metric.Bytes),
		zap.String("sha256", fmt.Sprintf("%x", h.Sum(nil))),
	)
	b.m.MediaBytes.Add(metric.Bytes)

	return nil
}

func (b *Bot) handleMedia(ctx context.Context, msg *tg.Message) error {
	switch m := msg.Media.(type) {
	case *tg.MessageMediaDocument:
		doc, ok := m.Document.AsNotEmpty()
		if !ok {
			return nil
		}
		if err := b.downloadMedia(ctx, &tg.InputDocumentFileLocation{
			ID:            doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
		}); err != nil {
			return xerrors.Errorf("download: %w", err)
		}
	case *tg.MessageMediaPhoto:
		p, ok := m.Photo.AsNotEmpty()
		if !ok {
			return nil
		}
		if err := b.downloadMedia(ctx, &tg.InputPhotoFileLocation{
			ID:            p.ID,
			AccessHash:    p.AccessHash,
			FileReference: p.FileReference,
			ThumbSize:     maxSize(p.Sizes),
		}); err != nil {
			return xerrors.Errorf("download: %w", err)
		}
	}

	return nil
}

func (b *Bot) handleChat(ctx context.Context, peer *tg.Chat, m *tg.Message) error {
	b.logger.Info("Got message from chat",
		zap.String("text", m.Message),
		zap.Int("chat_id", peer.ID),
	)

	return b.answer(ctx, m, &tg.InputPeerChat{
		ChatID: peer.ID,
	})
}

func (b *Bot) handleChannel(ctx context.Context, peer *tg.Channel, m *tg.Message) error {
	b.logger.Info("Got message from channel",
		zap.String("text", m.Message),
		zap.Int("channel_id", peer.ID),
	)

	return b.answer(ctx, m, &tg.InputPeerChannel{
		ChannelID:  peer.ID,
		AccessHash: peer.AccessHash,
	})
}

func (b *Bot) Handle(ctx context.Context, e tg.Entities, msg tg.MessageClass) error {
	switch m := msg.(type) {
	case *tg.Message:
		if m.Out {
			return nil
		}

		b.m.Messages.Inc()
		if err := b.handleMedia(ctx, m); err != nil {
			return xerrors.Errorf("handle media: %w", err)
		}

		switch peer := m.PeerID.(type) {
		case *tg.PeerUser:
			user, ok := e.Users[peer.UserID]
			if !ok {
				return xerrors.Errorf("unknown user ID %d", peer.UserID)
			}
			return b.handleUser(ctx, user, m)
		case *tg.PeerChat:
			chat, ok := e.Chats[peer.ChatID]
			if !ok {
				return xerrors.Errorf("unknown chat ID %d", peer.ChatID)
			}
			return b.handleChat(ctx, chat, m)
		case *tg.PeerChannel:
			channel, ok := e.Channels[peer.ChannelID]
			if !ok {
				return xerrors.Errorf("unknown channel ID %d", peer.ChannelID)
			}
			return b.handleChannel(ctx, channel, m)
		}
	}

	return nil
}

func (b *Bot) OnNewMessage(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
	if err := b.Handle(ctx, e, u.Message); err != nil {
		if !tg.IsUserBlocked(err) {
			return xerrors.Errorf("handle message %d: %w", u.Message.GetID(), err)
		}

		b.logger.Debug("Bot is blocked by user")
	}

	if err := b.state.Commit(u.Pts); err != nil {
		return xerrors.Errorf("commit: %w", err)
	}
	return nil
}

func (b *Bot) OnNewChannelMessage(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
	if err := b.Handle(ctx, e, u.Message); err != nil {
		return xerrors.Errorf("handle: %w", err)
	}

	if err := b.state.Commit(u.Pts); err != nil {
		return xerrors.Errorf("commit: %w", err)
	}
	return nil
}
