package tts

import (
	"context"
	"io"
	"net/http"
	"strings"

	"go.uber.org/multierr"
	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"

	"github.com/gotd/bot/internal/dispatch"
)

// Handler implements GPT request handler.
type Handler struct {
	client *http.Client
}

// New creates new Handler.
func New(client *http.Client) Handler {
	return Handler{client: client}
}

func (h Handler) requestTTS(ctx context.Context, msg, lang string) (io.ReadCloser, error) {
	// TODO(tdakkota): rate limiting.
	req, err := http.NewRequestWithContext(ctx,
		http.MethodGet, "https://translate.google.com.vn/translate_tts", nil,
	)
	if err != nil {
		return nil, xerrors.Errorf("create request: %w", err)
	}

	q := req.URL.Query()
	q.Add("ie", "UTF-8")
	q.Add("q", msg)
	q.Add("tl", lang)
	q.Add("client", "tw-ob")
	req.URL.RawQuery = q.Encode()

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("send request: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Close body to prevent resource leaking.
		_ = resp.Body.Close()
		return nil, xerrors.Errorf("bad code %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// OnMessage implements dispatch.MessageHandler.
func (h Handler) OnMessage(ctx context.Context, e dispatch.MessageEvent) error {
	lang := "en"
	cmd := strings.ToLower(e.Message.Message)
	cmd = strings.TrimSuffix(strings.TrimSpace(cmd), "@gotd_echo_bot")
	if strings.HasPrefix(cmd, "/tts_") {
		lang = strings.TrimSpace(strings.TrimPrefix(cmd, "/tts_"))
	}

	return e.WithReply(ctx, func(reply *tg.Message) (rerr error) {
		body, err := h.requestTTS(ctx, reply.GetMessage(), lang)
		if err != nil {
			if _, err := e.Reply().Text(ctx, "TTS server request failed"); err != nil {
				return xerrors.Errorf("send: %w", err)
			}
			return xerrors.Errorf("send TTS request: %w", err)
		}
		defer func() {
			multierr.AppendInto(&rerr, body.Close())
		}()

		_, err = e.Reply().Upload(message.FromReader("tts.mp3", body)).Voice(ctx)
		return err
	})
}
