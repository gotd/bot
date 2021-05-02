package gentext

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"golang.org/x/xerrors"
)

type GPT3 struct {
	client   *http.Client
	endpoint string
}

func (g *GPT3) WithClient(c *http.Client) *GPT3 {
	g.client = c
	return g
}

func (g *GPT3) WithEndpoint(endpoint string) *GPT3 {
	g.endpoint = endpoint
	return g
}

func NewGPT3() *GPT3 {
	return &GPT3{
		client:   http.DefaultClient,
		endpoint: "https://api.sbercloud.ru/v2/aicloud/gpt3",
	}
}

type gpt3Result struct {
	Status string `json:"status"`
	Data   string `json:"data"`
}

type gpt3Query struct {
	Question string `json:"question"`
}

func (g *GPT3) Query(ctx context.Context, query string) (string, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(gpt3Query{
		Question: query,
	}); err != nil {
		return "", xerrors.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx,
		http.MethodPost, g.endpoint, &buf,
	)
	if err != nil {
		return "", xerrors.Errorf("create request: %w", err)
	}
	defer req.Body.Close()

	req.Header.Set("User-Agent",
		`Mozilla/5.0 (Windows NT 1337.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4000.1`,
	)
	req.Header.Set("Origin", "https://sbercloud.ru")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Content-Type", "application/json;charset=utf-8")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", xerrors.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", xerrors.Errorf("bad code %d", resp.StatusCode)
	}

	var r gpt3Result
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", xerrors.Errorf("decode response: %w", err)
	}
	if r.Status != "success" {
		return "", xerrors.Errorf("got bad status: %q", r.Status)
	}

	return r.Data, nil
}
