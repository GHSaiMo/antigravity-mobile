package proxy

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

func isRemoteURI(uri string) bool {
	return strings.HasPrefix(uri, "vscode-remote://") || strings.HasPrefix(uri, "ssh://")
}

func normalizeURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}

	if isRemoteURI(uri) {
		return strings.TrimSuffix(uri, "/")
	}

	// Fix 2-slash file://d:/... -> file:///d:/...
	if strings.HasPrefix(uri, "file://") && !strings.HasPrefix(uri, "file:///") {
		rest := strings.TrimPrefix(uri, "file://")
		if len(rest) >= 2 && isWindowsDriveLetter(rest[0]) && rest[1] == ':' {
			uri = "file:///" + rest
		}
	}

	// Unescape %3A or any percent-encoded characters (like %20 or %E9...)
	if strings.Contains(uri, "%") {
		if unescaped, err := url.PathUnescape(uri); err == nil {
			uri = unescaped
		}
	}

	if strings.HasPrefix(uri, "file://") {
		clean := strings.TrimPrefix(uri, "file://")
		clean = filepath.ToSlash(clean)
		if runtime.GOOS == "windows" {
			if len(clean) > 2 && clean[0] == '/' && isWindowsDriveLetter(clean[1]) && clean[2] == ':' {
				clean = clean[1:]
			}
			if len(clean) >= 2 && isWindowsDriveLetter(clean[0]) && clean[1] == ':' {
				clean = strings.ToLower(string(clean[0])) + clean[1:]
				return "file:///" + strings.TrimSuffix(clean, "/")
			}
		}
		return strings.TrimSuffix("file://"+clean, "/")
	}

	// Direct Windows drive path like D:\Projects...
	if runtime.GOOS == "windows" && len(uri) >= 2 && isWindowsDriveLetter(uri[0]) && uri[1] == ':' {
		uri = strings.ToLower(string(uri[0])) + uri[1:]
		return "file:///" + filepath.ToSlash(strings.TrimSuffix(uri, "\\/"))
	}

	if strings.HasPrefix(uri, "/") {
		uri = "file://" + uri
	}
	return strings.TrimSuffix(uri, "/")
}

func uriToPath(rawURI string) string {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return ""
	}

	if isRemoteURI(rawURI) {
		u, err := url.Parse(rawURI)
		if err == nil && u.Path != "" {
			if unescaped, uErr := url.PathUnescape(u.Path); uErr == nil {
				return unescaped
			}
			return u.Path
		}
		for _, prefix := range []string{"vscode-remote://", "ssh://"} {
			if strings.HasPrefix(rawURI, prefix) {
				rest := strings.TrimPrefix(rawURI, prefix)
				if idx := strings.Index(rest, "/"); idx != -1 {
					pathPart := rest[idx:]
					if unescaped, uErr := url.PathUnescape(pathPart); uErr == nil {
						return unescaped
					}
					return pathPart
				}
			}
		}
		return rawURI
	}

	// Fix 2-slash file://d:/... -> file:///d:/...
	if strings.HasPrefix(rawURI, "file://") && !strings.HasPrefix(rawURI, "file:///") {
		rest := strings.TrimPrefix(rawURI, "file://")
		if len(rest) >= 2 && isWindowsDriveLetter(rest[0]) && rest[1] == ':' {
			rawURI = "file:///" + rest
		}
	}

	if !strings.HasPrefix(rawURI, "file://") {
		if strings.Contains(rawURI, "%") {
			if unescaped, err := url.PathUnescape(rawURI); err == nil {
				rawURI = unescaped
			}
		}
		return filepath.Clean(rawURI)
	}

	u, err := url.Parse(rawURI)
	var path string
	if err != nil {
		path = strings.TrimPrefix(rawURI, "file://")
	} else {
		p, uErr := url.PathUnescape(u.Path)
		if uErr == nil {
			path = p
		} else {
			path = u.Path
		}
	}

	if strings.Contains(path, "%") {
		if unescaped, err := url.PathUnescape(path); err == nil {
			path = unescaped
		}
	}

	if runtime.GOOS == "windows" {
		if len(path) > 2 && (path[0] == '/' || path[0] == '\\') && isWindowsDriveLetter(path[1]) && path[2] == ':' {
			path = path[1:]
		}
		if len(path) >= 2 && isWindowsDriveLetter(path[0]) && path[1] == ':' {
			path = strings.ToLower(string(path[0])) + path[1:]
		}
		return filepath.FromSlash(path)
	}
	return filepath.Clean(path)
}
