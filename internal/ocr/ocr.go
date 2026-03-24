// Package ocr defines the OCR provider interface and supporting types.
package ocr

import "context"

// Provider is the abstraction for any OCR backend.
// Implementations: MockProvider (development), YandexVisionProvider, GoogleVisionProvider, etc.
type Provider interface {
	// RecognizeText sends image bytes (PNG or JPEG) to the OCR backend
	// and returns the recognized text.
	RecognizeText(ctx context.Context, imageData []byte) (string, error)
}
