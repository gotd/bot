package gentext

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-faster/errors"
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
		endpoint: "https://api.aicloud.sbercloud.ru/public/v1/public_inference/gpt3/predict",
	}
}

type gpt3Result struct {
	Predictions string `json:"predictions"`
}

type gpt3Query struct {
	Text string `json:"text"`
}

func (g *GPT3) Query(ctx context.Context, query string) (string, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(gpt3Query{
		Text: query,
	}); err != nil {
		return "", errors.Wrap(err, "encode request")
	}

	req, err := http.NewRequestWithContext(ctx,
		http.MethodPost, g.endpoint, &buf,
	)
	if err != nil {
		return "", errors.Wrap(err, "create request")
	}
	defer req.Body.Close()

	req.Header.Set("User-Agent",
		`Mozilla/5.0 (Windows NT 1337.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4000.1`,
	)
	req.Header.Set("Origin", "https://russiannlp.github.io")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
	req.Header.Set("Content-Type", "application/json;charset=utf-8")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "send request")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", errors.Errorf("bad code %d", resp.StatusCode)
	}

	var r gpt3Result
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", errors.Wrap(err, "decode response")
	}

	return r.Predictions, nil
}
