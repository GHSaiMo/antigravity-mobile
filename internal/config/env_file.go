package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GetActiveEnvPath finds the most appropriate .env file to update.
// Priority:
// 1. Current working directory .env (if exists)
// 2. Global user ~/.multigravity/.env (default and created if needed)
func GetActiveEnvPath() string {
	// Check current directory first
	if fi, err := os.Stat(".env"); err == nil && !fi.IsDir() {
		if abs, err := filepath.Abs(".env"); err == nil {
			return abs
		}
		return ".env"
	}

	// Global data dir
	dataDir := GetDataDir()
	return filepath.Join(dataDir, ".env")
}

// UpdateEnvVariables updates or adds key-value pairs in the active .env file.
// It preserves existing comments and formatting, uncomments matching keys,
// appends new keys, and updates os.Setenv in the active process.
func UpdateEnvVariables(updates map[string]string) (string, error) {
	if len(updates) == 0 {
		return "", nil
	}

	targetPath := GetActiveEnvPath()
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return targetPath, fmt.Errorf("failed to create config dir: %w", err)
	}

	var lines []string
	var existingContent []byte
	if b, err := os.ReadFile(targetPath); err == nil {
		existingContent = b
	}

	scanner := bufio.NewScanner(strings.NewReader(string(existingContent)))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	handled := make(map[string]bool)
	var newLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check active definition: KEY=VALUE
		if !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, "=") {
			parts := strings.SplitN(trimmed, "=", 2)
			k := strings.TrimSpace(parts[0])
			if newVal, ok := updates[k]; ok {
				newLines = append(newLines, fmt.Sprintf("%s=%s", k, newVal))
				handled[k] = true
				continue
			}
		}

		// Check commented definition: # KEY=VALUE or # KEY=
		if strings.HasPrefix(trimmed, "#") {
			commentContent := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			if strings.Contains(commentContent, "=") {
				parts := strings.SplitN(commentContent, "=", 2)
				k := strings.TrimSpace(parts[0])
				if newVal, ok := updates[k]; ok && newVal != "" {
					newLines = append(newLines, fmt.Sprintf("%s=%s", k, newVal))
					handled[k] = true
					continue
				}
			}
		}

		newLines = append(newLines, line)
	}

	// Append any keys not yet handled
	for k, v := range updates {
		if !handled[k] && v != "" {
			newLines = append(newLines, fmt.Sprintf("%s=%s", k, v))
		}
	}

	output := strings.Join(newLines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	tmpFile := targetPath + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(output), 0600); err != nil {
		return targetPath, fmt.Errorf("failed to write tmp env file: %w", err)
	}
	if err := os.Rename(tmpFile, targetPath); err != nil {
		return targetPath, fmt.Errorf("failed to rename env file: %w", err)
	}

	// Update current runtime environment
	for k, v := range updates {
		_ = os.Setenv(k, v)
	}

	return targetPath, nil
}
