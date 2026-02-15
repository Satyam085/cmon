// Package translate provides Google Cloud Translation integration for CMON.
//
// This package wraps the cloud.google.com/go/translate v3 client to translate
// complaint text from English to Gujarati. It supports graceful degradation:
// if credentials or project ID are not configured, translation is silently
// disabled and the caller is expected to handle nil Translator.
package translate

import (
	"context"
	"fmt"
	"log"

	translate "cloud.google.com/go/translate/apiv3"
	translatepb "cloud.google.com/go/translate/apiv3/translatepb"
)

// Translator wraps the Google Cloud Translation v3 client.
//
// If nil, translation is disabled. Callers should nil-check before calling methods.
type Translator struct {
	client    *translate.TranslationClient
	projectID string
}

// NewTranslator creates a new Translator using Application Default Credentials.
//
// Returns nil (not an error) if projectID is empty, enabling graceful degradation.
// The caller should check for nil before using the returned Translator.
//
// Parameters:
//   - ctx: Context for client initialization
//   - projectID: Google Cloud project ID (from GOOGLE_PROJECT_ID env var)
//
// Returns:
//   - *Translator: Ready-to-use translator, or nil if not configured
//   - error: Client creation error (only if projectID is set but client fails)
func NewTranslator(ctx context.Context, projectID string) (*Translator, error) {
	if projectID == "" {
		log.Println("⚠️  GOOGLE_PROJECT_ID not set. Gujarati translation disabled.")
		return nil, nil
	}

	client, err := translate.NewTranslationClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create translation client: %w", err)
	}

	log.Println("✓ Google Cloud Translation configured successfully")

	return &Translator{
		client:    client,
		projectID: projectID,
	}, nil
}

// TranslateToGujarati translates the given English text to Gujarati.
//
// Parameters:
//   - ctx: Context for the API call
//   - text: English text to translate
//
// Returns:
//   - string: Gujarati translation
//   - error: Translation API error
func (t *Translator) TranslateToGujarati(ctx context.Context, text string) (string, error) {
	if t == nil || text == "" {
		return text, nil
	}

	req := &translatepb.TranslateTextRequest{
		Parent:             fmt.Sprintf("projects/%s/locations/global", t.projectID),
		SourceLanguageCode: "en",
		TargetLanguageCode: "gu",
		Contents:           []string{text},
		MimeType:           "text/plain",
	}

	resp, err := t.client.TranslateText(ctx, req)
	if err != nil {
		return "", fmt.Errorf("translation API error: %w", err)
	}

	if len(resp.GetTranslations()) == 0 {
		return "", fmt.Errorf("no translations returned")
	}

	return resp.GetTranslations()[0].GetTranslatedText(), nil
}

// BatchTranslateToGujarati translates multiple texts to Gujarati in a single API call.
//
// Parameters:
//   - ctx: Context for the API call
//   - texts: Slice of English texts to translate
//
// Returns:
//   - []string: Translated texts in same order as input
//   - error: Translation API error
func (t *Translator) BatchTranslateToGujarati(ctx context.Context, texts []string) ([]string, error) {
	if t == nil || len(texts) == 0 {
		return texts, nil
	}

	req := &translatepb.TranslateTextRequest{
		Parent:             fmt.Sprintf("projects/%s/locations/global", t.projectID),
		SourceLanguageCode: "en",
		TargetLanguageCode: "gu",
		Contents:           texts,
		MimeType:           "text/plain",
	}

	resp, err := t.client.TranslateText(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("translation API error: %w", err)
	}

	translations := resp.GetTranslations()
	if len(translations) != len(texts) {
		return nil, fmt.Errorf("expected %d translations, got %d", len(texts), len(translations))
	}

	result := make([]string, len(translations))
	for i, t := range translations {
		result[i] = t.GetTranslatedText()
	}
	return result, nil
}

// Close cleans up the translation client resources.
func (t *Translator) Close() error {
	if t == nil || t.client == nil {
		return nil
	}
	return t.client.Close()
}
