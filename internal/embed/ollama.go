// Package embed talks to a local Ollama embedding endpoint.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Provider embeds text via Ollama /api/embed.
type Provider struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewProvider constructs an Ollama embedding client. baseURL is like http://127.0.0.1:11434.
func NewProvider(baseURL, model string) *Provider {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Provider{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// Model returns the configured model name.
func (p *Provider) Model() string { return p.model }

// Ping checks that the embed endpoint responds for a tiny input.
func (p *Provider) Ping(ctx context.Context) error {
	_, err := p.Embed(ctx, []string{"ping"})
	return err
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed returns one L2-oriented vector per input string.
func (p *Provider) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	body, err := json.Marshal(embedRequest{Model: p.model, Input: inputs})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request failed: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out embedResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("embed decode: %w", err)
	}
	if len(out.Embeddings) != len(inputs) {
		return nil, fmt.Errorf("embed: got %d vectors for %d inputs", len(out.Embeddings), len(inputs))
	}
	for i := range out.Embeddings {
		out.Embeddings[i] = l2normalize(out.Embeddings[i])
	}
	return out.Embeddings, nil
}

func l2normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	inv := float32(1 / sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
	return v
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 16; i++ {
		z = 0.5 * (z + x/z)
	}
	return z
}
