package main

import (
	"path/filepath"
	"strings"
)

func photoContentType(extension string) (string, bool) {
	switch strings.ToLower(filepath.Ext(extension)) {
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".png":
		return "image/png", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	case ".bmp":
		return "image/bmp", true
	case ".avif":
		return "image/avif", true
	default:
		return "", false
	}
}
