package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DefaultAntigravityIcon is the public URL for the Antigravity icon in this repository.
const DefaultAntigravityIcon = "https://raw.githubusercontent.com/GHSaiMo/antigravity-mobile/main/web/icons/icon-192.png"

// NotificationConfig holds settings for push notifications (Bark / Webhook).
type NotificationConfig struct {
	Enabled       bool
	BarkEndpoint  string // Normalized POST/GET endpoint, e.g. "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9"
	BarkRawURL    string
	IconURL       string
	Group         string
	SoundAction   string
	SoundComplete string
}

// LoadDotEnv searches for a .env file in standard locations (CWD, parent directory, binary directory)
// and populates environment variables that are not already set.
func LoadDotEnv(paths ...string) {
	searchPaths := append(paths, ".env", "../.env")
	if execPath, err := os.Executable(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(filepath.Dir(execPath), ".env"))
		searchPaths = append(searchPaths, filepath.Join(filepath.Dir(execPath), "..", ".env"))
	}

	for _, p := range searchPaths {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			parseEnvFile(p)
			return // Load the first found .env
		}
	}
}

func parseEnvFile(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		// Strip surrounding quotes
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
		}

		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

// NormalizeBarkEndpoint extracts the clean Bark API base device URL from various input formats.
// Supported inputs:
//   - "8CTWiePgCJqiZNsJBaaDX9" -> "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9"
//   - "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9/" -> "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9"
//   - "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9/xxx?icon=yyy" -> "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9"
//   - "https://my-bark.com/mykey" -> "https://my-bark.com/mykey"
func NormalizeBarkEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Case 1: Just the device key without URL scheme
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		// Clean any trailing slashes or path
		key := strings.Trim(raw, "/")
		if idx := strings.Index(key, "/"); idx != -1 {
			key = key[:idx]
		}
		if key != "" {
			return "https://api.day.app/" + key
		}
		return ""
	}

	// Case 2: Full URL
	u, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(raw, "/")
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) > 0 && segments[0] != "" {
		// Scheme + Host + first path segment (device key)
		return fmt.Sprintf("%s://%s/%s", u.Scheme, u.Host, segments[0])
	}

	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

// GetNotificationConfig parses environment variables and returns a NotificationConfig.
func GetNotificationConfig() NotificationConfig {
	barkRaw := os.Getenv("BARK_URL")
	if barkRaw == "" {
		barkRaw = os.Getenv("BARK_SERVER_URL")
	}
	if barkRaw == "" {
		barkRaw = os.Getenv("BARK_KEY")
	}

	endpoint := NormalizeBarkEndpoint(barkRaw)

	// Enabled if explicitly set or if a valid Bark endpoint is present
	enabled := endpoint != ""
	if v := os.Getenv("BARK_ENABLE"); v != "" {
		vLower := strings.ToLower(v)
		enabled = (vLower == "1" || vLower == "true" || vLower == "yes")
	}

	icon := os.Getenv("BARK_ICON")
	if icon == "" {
		icon = os.Getenv("BARK_ICON_URL")
	}
	if icon == "" {
		icon = DefaultAntigravityIcon
	}

	group := os.Getenv("BARK_GROUP")
	if group == "" {
		group = "Antigravity"
	}

	soundAction := os.Getenv("BARK_SOUND_ACTION")
	if soundAction == "" {
		soundAction = "alarm"
	}

	soundComplete := os.Getenv("BARK_SOUND_COMPLETE")
	if soundComplete == "" {
		soundComplete = "glass"
	}

	return NotificationConfig{
		Enabled:       enabled,
		BarkEndpoint:  endpoint,
		BarkRawURL:    barkRaw,
		IconURL:       icon,
		Group:         group,
		SoundAction:   soundAction,
		SoundComplete: soundComplete,
	}
}
