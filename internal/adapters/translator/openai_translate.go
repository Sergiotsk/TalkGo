package translator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	// gpt-4o-mini is ~3x faster than gpt-4o for conversational translation
	// with negligible quality difference for short phrases.
	defaultTranslateModel   = "gpt-4o-mini"
	defaultTranslateBaseURL = "https://api.openai.com/v1/chat/completions"
)

// TextTranslatorConfig holds configuration for the GPT-4o text translator.
type TextTranslatorConfig struct {
	APIKey  string
	Model   string // default: gpt-4o
	BaseURL string // default: https://api.openai.com/v1/chat/completions
}

func (c *TextTranslatorConfig) applyDefaults() {
	if c.Model == "" {
		c.Model = defaultTranslateModel
	}
	if c.BaseURL == "" {
		c.BaseURL = defaultTranslateBaseURL
	}
}

// TextTranslator translates text using the OpenAI Chat Completions API.
type TextTranslator struct {
	cfg    TextTranslatorConfig
	client *http.Client
}

// NewTextTranslator creates a TextTranslator ready to use.
func NewTextTranslator(cfg TextTranslatorConfig) *TextTranslator {
	cfg.applyDefaults()
	return &TextTranslator{cfg: cfg, client: &http.Client{}}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Translate sends text to GPT-4o and returns the translation in targetLang.
func (t *TextTranslator) Translate(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: t.cfg.Model,
		Messages: []chatMessage{
			{
				Role: "system",
				Content: fmt.Sprintf(
					"You are a professional interpreter. "+
						"Translate the following text from %s to %s. "+
						"Output ONLY the translation. No explanations, no alternatives, no notes.",
					sourceLang, targetLang,
				),
			},
			{Role: "user", Content: text},
		},
	})
	if err != nil {
		return "", fmt.Errorf("translate: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("translate: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("translate: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("translate: api error %d: %s", resp.StatusCode, string(b))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("translate: decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("translate: empty response")
	}
	return result.Choices[0].Message.Content, nil
}
