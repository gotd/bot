package gentext

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"net/http"

	"github.com/go-faster/errors"
)

type GPT2 struct {
	client    *http.Client
	endpoint  string
	minLength int
	maxLength int
}

func (g *GPT2) WithClient(c *http.Client) *GPT2 {
	g.client = c
	return g
}

func (g *GPT2) WithEndpoint(endpoint string) *GPT2 {
	g.endpoint = endpoint
	return g
}

func (g *GPT2) WithMinLength(minLength int) *GPT2 {
	g.minLength = minLength
	return g
}
func (g *GPT2) WithMaxLength(maxLength int) *GPT2 {
	g.maxLength = maxLength
	return g
}

func NewGPT2() *GPT2 {
	return &GPT2{
		client:    http.DefaultClient,
		endpoint:  "https://pelevin.gpt.dobro.ai/generate/",
		minLength: 5,
		maxLength: 95,
	}
}

type gpt2Result struct {
	Replies []string `json:"replies"`
}

type gpt2Query struct {
	Prompt string `json:"prompt"`
	Length int    `json:"length"`
}

func (g *GPT2) Query(ctx context.Context, query string) (string, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(gpt2Query{
		Prompt: query,
		Length: rand.Intn(g.maxLength-g.minLength) + g.minLength,
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
	req.Header.Set("Origin", "https://porfirevich.ru")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "send request")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", errors.Errorf("bad code %d", resp.StatusCode)
	}

	var r gpt2Result
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", errors.Wrap(err, "decode response")
	}
	if len(r.Replies) < 1 {
		return "", errors.Errorf("got empty result %v", r)
	}

	return r.Replies[0], nil
}
