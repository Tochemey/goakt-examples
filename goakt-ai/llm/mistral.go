// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type mistralClient struct {
	apiKey    string
	model     string
	maxTokens int
	client    *http.Client
}

// NewMistralClient creates a Mistral client
func NewMistralClient(apiKey, model string, maxTokens int) Client {
	if model == "" {
		model = "mistral-small-latest"
	}
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	return &mistralClient{
		apiKey:    apiKey,
		model:     model,
		maxTokens: maxTokens,
		client:    &http.Client{Timeout: 90 * time.Second},
	}
}

type mistralRequest struct {
	Model     string       `json:"model"`
	Messages  []mistralMsg `json:"messages"`
	MaxTokens int          `json:"max_tokens,omitempty"`
}

type mistralMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mistralResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *string `json:"error,omitempty"`
}

func (c *mistralClient) Complete(ctx context.Context, prompt, systemPrompt string) (string, error) {
	messages := []mistralMsg{}
	if systemPrompt != "" {
		messages = append(messages, mistralMsg{Role: "system", Content: systemPrompt})
	}
	messages = append(messages, mistralMsg{Role: "user", Content: prompt})

	body, _ := json.Marshal(mistralRequest{
		Model:     c.model,
		Messages:  messages,
		MaxTokens: c.maxTokens,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.mistral.ai/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result mistralResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Error != nil {
		return "", fmt.Errorf("mistral api error: %s", *result.Error)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no completion returned")
	}
	return result.Choices[0].Message.Content, nil
}
