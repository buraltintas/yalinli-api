package validation

import (
	"bytes"
	"errors"
	"net/http"
)

var allowedImageMIME = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

func ValidateImageContent(data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("empty image")
	}
	ct := http.DetectContentType(data)
	if _, ok := allowedImageMIME[ct]; !ok {
		return "", errors.New("unsupported image format")
	}
	if bytes.Contains(bytes.ToLower(data[:min(len(data), 512)]), []byte("<svg")) {
		return "", errors.New("svg is not allowed")
	}
	return ct, nil
}

func ExtensionFromMIME(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
