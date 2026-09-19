package message

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var imageMimes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

var documentMimes = map[string]bool{
	"application/pdf":    true,
	"application/zip":    true,
	"application/json":   true,
	"application/xml":    true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
	"application/vnd.ms-excel": true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.ms-powerpoint":                                             true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"text/plain": true,
	"text/csv":   true,
	"text/xml":   true,
}

var blockedMimeParts = []string{
	"executable",
	"x-msdownload",
	"x-sh",
	"x-shellscript",
	"x-bat",
	"x-msdos-program",
	"java-archive",
	"x-httpd",
}

var ErrMediaTooLarge = errors.New("media file is too large")
var ErrUnsupportedMediaType = errors.New("unsupported media type")

func ValidateImageData(data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("image file is empty")
	}
	detected := normalizeMime(http.DetectContentType(data))
	if !imageMimes[detected] {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedMediaType, detected)
	}
	return detected, nil
}

func NormalizeDocumentMime(claimed string) (string, error) {
	mime := normalizeMime(claimed)
	if mime == "" || mime == "application/octet-stream" {
		mime = "application/octet-stream"
	}
	for _, part := range blockedMimeParts {
		if strings.Contains(mime, part) {
			return "", fmt.Errorf("%w: %s", ErrUnsupportedMediaType, mime)
		}
	}
	if !documentMimes[mime] && mime != "application/octet-stream" {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedMediaType, mime)
	}
	return mime, nil
}

func normalizeMime(raw string) string {
	mime := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = strings.TrimSpace(mime[:i])
	}
	return mime
}
