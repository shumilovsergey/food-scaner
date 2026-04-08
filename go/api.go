package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const claudeAPI = "https://api.anthropic.com/v1/messages"
const model = "claude-opus-4-6"
const systemPrompt = `You are a nutrition expert. Analyze the food in this image and estimate the nutritional content for the visible portion.
Respond with ONLY a valid JSON object in this exact format, no other text:
{"name": "food name", "grams": estimated_grams, "calories": estimated_calories}`

type ScanRequest struct {
	Image     string `json:"image"`
	MediaType string `json:"media_type"`
}

type ScanResult struct {
	Name     string  `json:"name"`
	Grams    float64 `json:"grams"`
	Calories float64 `json:"calories"`
}

func handleScan(w http.ResponseWriter, r *http.Request) {
	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Image == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.MediaType == "" {
		req.MediaType = "image/jpeg"
	}

	result, err := scanWithClaude(req.Image, req.MediaType)
	if err != nil {
		http.Error(w, fmt.Sprintf("scan failed: %v", err), http.StatusInternalServerError)
		return
	}

	userID := userIDFromCtx(r)
	insertScan(userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func scanWithClaude(imageBase64, mediaType string) (*ScanResult, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	payload := map[string]any{
		"model":      model,
		"max_tokens": 256,
		"system":     systemPrompt,
		"messages": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": mediaType,
							"data":       imageBase64,
						},
					},
					{
						"type": "text",
						"text": "What food is this? Give me the name, estimated grams, and calories.",
					},
				},
			},
		},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", claudeAPI, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("claude api error %d: %s", resp.StatusCode, string(b))
	}

	var claudeResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return nil, err
	}
	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from claude")
	}

	text := strings.TrimSpace(claudeResp.Content[0].Text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON in response: %s", text)
	}

	var result ScanResult
	if err := json.Unmarshal([]byte(text[start:end+1]), &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %v", err)
	}
	return &result, nil
}
