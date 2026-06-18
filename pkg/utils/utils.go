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
	if !strings.Contains(image, ":") {
		return "", fmt.Errorf("image tag required: %s (format: name:tag)", image)
	}

	namePart := strings.Split(image, ":")[0]

	if !strings.Contains(namePart, "/") {
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

	parts := strings.SplitN(normalized, ":", 2)
	namePart := parts[0]
	tag := parts[1]

	nameSegments := strings.Split(namePart, "/")
	namePart = nameSegments[len(nameSegments)-1]

	return namePart + "_" + tag, nil
}
