// Package translate provides Gemini AI translation for CMON.
//
// Translates complaint fields from English-script Gujarati (transliteration)
// to proper Gujarati script using the Gemini API. For example:
//   - "GAM" → "ગામ" (village)
//   - "LITE NATHI" → "લાઇટ નથી" (no electricity)
//
// Graceful degradation: if API key is not set, translation is disabled.
// On 429 rate limit errors, returns empty string so only English is sent.
package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const systemPrompt = `You are a translator for an Indian electricity complaint system.
The input contains complaint fields (Name, Details, Address) written in Gujarati using English/Latin script (transliteration).
Convert each field to proper Gujarati script (ગુજરાતી).
Rules:
- Transliterate Gujarati words from English script to Gujarati script
- Keep numbers, dates, and complaint IDs as-is
- If a field is already in English (like a proper name), transliterate it phonetically
- Output ONLY the translated fields in the exact same format, nothing else`

// Translator wraps the Gemini API client for transliteration.
type Translator struct {
	apiKey string
	model  string
	client *http.Client
}

// NewTranslator creates a new Gemini-based Translator.
//
// Returns nil if apiKey is empty (graceful degradation).
func NewTranslator(_ context.Context, apiKey string) (*Translator, error) {
	if apiKey == "" {
		log.Println("⚠️  GEMINI_API_KEY not set. Gujarati translation disabled.")
		return nil, nil
	}

	log.Println("✓ Gemini translation configured successfully")

	return &Translator{
		apiKey: apiKey,
		model:  "gemini-2.5-flash",
		client: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// geminiRequest / geminiResponse for the REST API
type geminiRequest struct {
	SystemInstruction *content  `json:"system_instruction,omitempty"`
	Contents          []content `json:"contents"`
}

type content struct {
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// BatchTranslateToGujarati translates multiple fields in a single Gemini API call.
//
// Sends all fields as a structured prompt and parses the response.
// Returns empty strings on 429 rate limit (caller sends English-only).
func (t *Translator) BatchTranslateToGujarati(ctx context.Context, texts []string) ([]string, error) {
	if t == nil || len(texts) == 0 {
		return texts, nil
	}

	// Build prompt with labeled fields for structured output
	prompt := fmt.Sprintf("Name: %s\nDetails: %s\nLocation: %s\nArea: %s",
		texts[0], texts[1], texts[2], texts[3])

	reqBody := geminiRequest{
		SystemInstruction: &content{
			Parts: []part{{Text: systemPrompt}},
		},
		Contents: []content{
			{Parts: []part{{Text: prompt}}},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		t.model, t.apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle 429 rate limit — return empty so caller sends English-only
	if resp.StatusCode == 429 {
		log.Println("  ⚠️  Gemini 429 rate limit — skipping translation")
		return nil, fmt.Errorf("rate limited")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if geminiResp.Error != nil {
		if geminiResp.Error.Code == 429 {
			log.Println("  ⚠️  Gemini 429 rate limit — skipping translation")
			return nil, fmt.Errorf("rate limited")
		}
		return nil, fmt.Errorf("API error: %s", geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from Gemini")
	}

	// Parse the structured response
	responseText := geminiResp.Candidates[0].Content.Parts[0].Text
	return parseTranslationResponse(responseText, texts), nil
}

// parseTranslationResponse extracts translated fields from Gemini's response.
// Falls back to original text if parsing fails for any field.
func parseTranslationResponse(response string, originals []string) []string {
	result := make([]string, len(originals))
	copy(result, originals) // Default to originals

	labels := []string{"Name:", "Details:", "Location:", "Area:"}

	for i, label := range labels {
		idx := strings.Index(response, label)
		if idx == -1 {
			continue
		}

		// Extract value after label
		valueStart := idx + len(label)
		remaining := response[valueStart:]

		// Find end of this field (next label or end of string)
		endIdx := len(remaining)
		for _, nextLabel := range labels {
			if nextIdx := strings.Index(remaining, nextLabel); nextIdx > 0 && nextIdx < endIdx {
				endIdx = nextIdx
			}
		}

		value := strings.TrimSpace(remaining[:endIdx])
		if value != "" {
			result[i] = value
		}
	}

	return result
}

// Close is a no-op for the HTTP-based client (satisfies the interface pattern).
func (t *Translator) Close() error {
	return nil
}
