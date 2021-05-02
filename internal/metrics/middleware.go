package metrics

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"golang.org/x/xerrors"

	"github.com/gotd/bot/internal/dispatch"
)

type Middleware struct {
	next       dispatch.MessageHandler
	downloader *downloader.Downloader
	metrics    Metrics

	logger *zap.Logger
}

// NewMiddleware creates new metrics middleware
func NewMiddleware(next dispatch.MessageHandler, d *downloader.Downloader, metrics Metrics) Middleware {
	return Middleware{
		next:       next,
		downloader: d,
		metrics:    metrics,
		logger:     zap.NewNop(),
	}
}

// WithLogger sets logger.
func (m *Middleware) WithLogger(logger *zap.Logger) *Middleware {
	m.logger = logger
	return m
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

func (m Middleware) downloadMedia(ctx context.Context, rpc *tg.Client, loc tg.InputFileLocationClass) error {
	h := sha256.New()
	w := &metricWriter{
		Increase: m.metrics.MediaBytes.Add,
	}

	if _, err := m.downloader.Download(rpc, loc).
		Stream(ctx, io.MultiWriter(h, w)); err != nil {
		return xerrors.Errorf("stream: %w", err)
	}

	m.logger.Info("Downloaded media",
		zap.Int64("bytes", w.Bytes),
		zap.String("sha256", fmt.Sprintf("%x", h.Sum(nil))),
	)

	return nil
}

func (m Middleware) handleMedia(ctx context.Context, rpc *tg.Client, msg *tg.Message) error {
	switch media := msg.Media.(type) {
	case *tg.MessageMediaDocument:
		doc, ok := media.Document.AsNotEmpty()
		if !ok {
			return nil
		}
		if err := m.downloadMedia(ctx, rpc, &tg.InputDocumentFileLocation{
			ID:            doc.ID,
			AccessHash:    doc.AccessHash,
			FileReference: doc.FileReference,
		}); err != nil {
			return xerrors.Errorf("download: %w", err)
		}
	case *tg.MessageMediaPhoto:
		p, ok := media.Photo.AsNotEmpty()
		if !ok {
			return nil
		}
		if err := m.downloadMedia(ctx, rpc, &tg.InputPhotoFileLocation{
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

// OnMessage implements bot.MessageHandler.
func (m Middleware) OnMessage(ctx context.Context, e dispatch.MessageEvent) error {
	m.metrics.Messages.Inc()

	if err := m.next.OnMessage(ctx, e); err != nil {
		return err
	}

	if err := m.handleMedia(ctx, e.RPC(), e.Message); err != nil {
		return xerrors.Errorf("handle media: %w", err)
	}

	m.metrics.Responses.Inc()
	return nil
}
