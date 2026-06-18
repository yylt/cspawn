package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func GenerateID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func GetAbsolutePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}
	return os.WriteFile(dst, data, 0644)
}

func NormalizeImage(image string) (string, error) {
	if !strings.Contains(image, ":") && !strings.Contains(image, "@") {
		return "", fmt.Errorf("image tag or digest required: %s (format: name:tag or name@sha256:digest)", image)
	}

	namePart := image
	if idx := strings.Index(image, "@"); idx != -1 {
		namePart = image[:idx]
	} else if idx := strings.Index(image, ":"); idx != -1 {
		namePart = image[:idx]
	}

	if !strings.Contains(namePart, "/") {
		if strings.Contains(image, "@") {
			return "docker.io/library/" + image, nil
		}
		return "docker.io/library/" + image, nil
	}

	firstPart := strings.Split(namePart, "/")[0]
	if !strings.Contains(firstPart, ".") {
		return "docker.io/" + image, nil
	}

	return image, nil
}

func ImageToRootfsName(image string) (string, error) {
	normalized, err := NormalizeImage(image)
	if err != nil {
		return "", err
	}

	normalized = strings.TrimPrefix(normalized, "docker.io/library/")
	normalized = strings.TrimPrefix(normalized, "docker.io/")

	var namePart, identifier string

	if strings.Contains(normalized, "@") {
		parts := strings.SplitN(normalized, "@", 2)
		namePart = parts[0]
		digest := parts[1]
		if strings.HasPrefix(digest, "sha256:") {
			hash := strings.TrimPrefix(digest, "sha256:")
			if len(hash) > 12 {
				hash = hash[:12]
			}
			identifier = "sha256-" + hash
		} else {
			identifier = digest
		}
	} else {
		parts := strings.SplitN(normalized, ":", 2)
		namePart = parts[0]
		identifier = parts[1]
	}

	nameSegments := strings.Split(namePart, "/")
	namePart = nameSegments[len(nameSegments)-1]

	return namePart + "_" + identifier, nil
}
