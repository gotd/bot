package metrics

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-faster/errors"
	"github.com/gotd/td/fileid"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"

	"github.com/gotd/bot/internal/dispatch"
)

type Middleware struct {
	next       dispatch.MessageHandler
	downloader *downloader.Downloader
	metrics    Metrics

	token      string
	httpClient *http.Client

	logger *zap.Logger
}

// NewMiddleware creates new metrics middleware
func NewMiddleware(
	next dispatch.MessageHandler,
	d *downloader.Downloader,
	metrics Metrics,
	opts MiddlewareOptions,
) Middleware {
	opts.setDefaults()
	return Middleware{
		next:       next,
		downloader: d,
		metrics:    metrics,
		token:      opts.Token,
		httpClient: opts.HTTPClient,
		logger:     opts.Logger,
	}
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
		return errors.Wrap(err, "stream")
	}

	m.logger.Info("Downloaded media",
		zap.Int64("bytes", w.Bytes),
		zap.String("sha256", fmt.Sprintf("%x", h.Sum(nil))),
	)

	return nil
}

func (m Middleware) getFile(ctx context.Context, id string) (rErr error) {
	u := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", m.token, url.QueryEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return errors.Wrap(err, "create request")
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "send request")
	}
	defer multierr.AppendInvoke(&rErr, multierr.Close(resp.Body))

	var result struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return errors.Wrap(err, "decode json")
	}
	if !result.OK {
		return errors.Errorf("API error %d: %s", result.ErrorCode, result.Description)
	}

	return nil
}

func (m Middleware) tryFileID(ctx context.Context, id fileid.FileID) error {
	if m.token == "" {
		return nil
	}

	encoded, err := fileid.EncodeFileID(id)
	if err != nil {
		return errors.Wrap(err, "encode")
	}

	return m.getFile(ctx, encoded)
}

func (m Middleware) handleMedia(ctx context.Context, rpc *tg.Client, msg *tg.Message) error {
	log := m.logger.With(zap.Int("msg_id", msg.ID), zap.Stringer("peer_id", msg.PeerID))
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
			return errors.Wrap(err, "download")
		}

		if err := m.tryFileID(ctx, fileid.FromDocument(doc)); err != nil {
			log.Warn("Test document FileID", zap.Error(err))
		}

	case *tg.MessageMediaPhoto:
		p, ok := media.Photo.AsNotEmpty()
		if !ok {
			return nil
		}
		size := maxSize(p.Sizes)
		if err := m.downloadMedia(ctx, rpc, &tg.InputPhotoFileLocation{
			ID:            p.ID,
			AccessHash:    p.AccessHash,
			FileReference: p.FileReference,
			ThumbSize:     size,
		}); err != nil {
			return errors.Wrap(err, "download")
		}

		thumbType := 'x'
		if len(size) >= 1 {
			thumbType = rune(size[0])
		}

		if err := m.tryFileID(ctx, fileid.FromPhoto(p, thumbType)); err != nil {
			log.Warn("Test photo FileID", zap.Error(err))
		}
	}

	return nil
}

// OnMessage implements dispatch.MessageHandler.
func (m Middleware) OnMessage(ctx context.Context, e dispatch.MessageEvent) error {
	m.metrics.Messages.Inc()

	if err := m.next.OnMessage(ctx, e); err != nil {
		return err
	}

	if err := m.handleMedia(ctx, e.RPC(), e.Message); err != nil {
		return errors.Wrap(err, "handle media")
	}

	m.metrics.Responses.Inc()
	return nil
}
