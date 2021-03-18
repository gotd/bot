package main

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"net/http"

	"golang.org/x/xerrors"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

type result struct {
	Replies []string `json:"replies"`
}

type query struct {
	Prompt string `json:"prompt"`
	Length int    `json:"length"`
}

func (b *Bot) requestGPT2(ctx context.Context, q query) ([]string, error) {
	// TODO(tdakkota): rate limiting.
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(q); err != nil {
		return nil, xerrors.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx,
		http.MethodPost, "https://pelevin.gpt.dobro.ai/generate/", &buf,
	)
	if err != nil {
		return nil, xerrors.Errorf("create request: %w", err)
	}
	defer req.Body.Close()

	req.Header.Set("User-Agent",
		`Mozilla/5.0 (Windows NT 1337.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4000.1`,
	)
	req.Header.Set("Origin", "https://porfirevich.ru")

	resp, err := b.http.Do(req)
	if err != nil {
		return nil, xerrors.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, xerrors.Errorf("bad code %d", resp.StatusCode)
	}

	var r result
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, xerrors.Errorf("decode response: %w", err)
	}
	if len(r.Replies) < 1 {
		return nil, xerrors.Errorf("got empty result %v", r)
	}

	return r.Replies, nil
}

func (b *Bot) answerGPT2(
	ctx tg.UpdateContext,
	send *message.Builder,
	peer tg.InputPeerClass,
	m *tg.Message,
) error {
	return b.getReply(ctx, send, peer, m, func(msg *tg.Message) error {
		prompt := msg.GetMessage()
		result, err := b.requestGPT2(ctx, query{
			Prompt: prompt,
			Length: rand.Intn(b.gpt.MaxLength-b.gpt.MinLength) + b.gpt.MinLength,
		})
		if err != nil {
			if _, err := send.Text(ctx, "GPT2 server request failed"); err != nil {
				return xerrors.Errorf("send: %w", err)
			}
			return xerrors.Errorf("send GPT2 request: %w", err)
		}

		_, err = b.sender.Peer(peer).ReplyMsg(msg).StyledText(ctx,
			message.Bold(prompt),
			message.Plain(result[0]),
		)
		return err
	})
}
