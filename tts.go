package main

import (
	"context"
	"io"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

func (b *Bot) requestTTS(ctx context.Context, msg, lang string) (io.ReadCloser, error) {
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

	resp, err := b.http.Do(req)
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

func (b *Bot) answerTTS(
	ctx context.Context,
	send *message.Builder,
	peer tg.InputPeerClass,
	m *tg.Message,
	lang string,
) error {
	return b.getReply(ctx, send, peer, m, func(msg *tg.Message) error {
		body, err := b.requestTTS(ctx, msg.GetMessage(), lang)
		if err != nil {
			if _, err := send.Text(ctx, "TTS server request failed"); err != nil {
				return xerrors.Errorf("send: %w", err)
			}
			return xerrors.Errorf("send TTS request: %w", err)
		}
		defer ignoreClose(body)

		_, err = b.sender.To(peer).ReplyMsg(msg).
			Upload(message.FromReader("tts.mp3", body)).
			Voice(ctx)
		return err
	})
}
