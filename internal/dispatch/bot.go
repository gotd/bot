package dispatch

import (
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

// Bot represents generic Telegram bot state and event dispatcher.
type Bot struct {
	handler MessageHandler

	rpc        *tg.Client
	sender     *message.Sender
	downloader *downloader.Downloader

	logger *zap.Logger
}

// NewBot creates new bot.
func NewBot(raw *tg.Client, handler MessageHandler) *Bot {
	return &Bot{
		rpc:        raw,
		sender:     message.NewSender(raw),
		handler:    handler,
		downloader: downloader.NewDownloader(),
		logger:     zap.NewNop(),
	}
}

// WithSender sets message sender to use.
func (b *Bot) WithSender(sender *message.Sender) *Bot {
	b.sender = sender
	return b
}

// WithLogger sets logger.
func (b *Bot) WithLogger(logger *zap.Logger) *Bot {
	b.logger = logger
	return b
}

// Register sets handlers using given dispatcher.
func (b *Bot) Register(dispatcher tg.UpdateDispatcher) *Bot {
	dispatcher.OnNewMessage(b.OnNewMessage)
	dispatcher.OnNewChannelMessage(b.OnNewChannelMessage)
	return b
}
