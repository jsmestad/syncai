package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jsmestad/syncai/internal/load"
)

const sourceEnvironment = "SYNCAI_SOURCE"

type file struct {
	Source string `json:"source"`
}

func Root() (string, error) {
	if root := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); root != "" {
		return filepath.Abs(root)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config"), nil
}

func Path() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "syncai", "config.json"), nil
}

func DefaultSource() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "syncai", "ai-source"), nil
}

func ResolveSource(explicit string) (string, error) {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return absoluteSource(explicit)
	}
	if environment := strings.TrimSpace(os.Getenv(sourceEnvironment)); environment != "" {
		return absoluteSource(environment)
	}
	saved, err := LoadSource()
	if err != nil {
		return "", err
	}
	if saved != "" {
		return saved, nil
	}
	return absoluteSource("ai-source")
}

func LoadSource() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	var saved file
	if err := json.Unmarshal(raw, &saved); err != nil {
		return "", fmt.Errorf("parsing %s: %w", path, err)
	}
	saved.Source = strings.TrimSpace(saved.Source)
	if saved.Source == "" {
		return "", fmt.Errorf("%s: source is empty", path)
	}
	if !filepath.IsAbs(saved.Source) {
		return "", fmt.Errorf("%s: source must be an absolute path", path)
	}
	return filepath.Clean(saved.Source), nil
}

func SaveSource(source string) (string, error) {
	absolute, err := absoluteSource(source)
	if err != nil {
		return "", err
	}
	path, err := Path()
	if err != nil {
		return "", err
	}
	root, err := Root()
	if err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(file{Source: absolute}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := load.WriteFileReplacing(root, path, append(raw, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("saving %s: %w", path, err)
	}
	return path, nil
}

func absoluteSource(source string) (string, error) {
	absolute, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("resolving source path %s: %w", source, err)
	}
	return filepath.Clean(absolute), nil
}
