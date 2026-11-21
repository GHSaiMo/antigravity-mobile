package notifier

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

var (
	urlUserInfoRE   = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^/\s:@]+:)[^@\s/]+(@)`)
	authSchemeRE    = regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[A-Za-z0-9._~+/=-]{6,}`)
	ya29TokenRE     = regexp.MustCompile(`\bya29\.[A-Za-z0-9._-]+`)
	googleRefreshRE = regexp.MustCompile(`1//[A-Za-z0-9_-]{6,}`)
	jwtTokenRE      = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]+`)
	skTokenRE       = regexp.MustCompile(`\b(?:sk|rk|pk)-[A-Za-z0-9_-]{8,}`)
	googleAPIKeyRE  = regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{16,}`)
	ghTokenRE       = regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{12,}`)
	kvSecretAssignRE = regexp.MustCompile(`(?i)(\b(?:[a-z0-9_.-]*?(?:token|secret|password|passwd|api[-_]?key|private[-_]?key|access[-_]?key)|key)\s*=\s*)(['"]?)[^\s'"&]+(['"]?)`)
	cliSecretFlagRE  = regexp.MustCompile(`(?i)((?:^|\s)--?(?:[a-z0-9_-]*?(?:token|secret|password|passwd|api[-_]?key)|pass|key)\b(?:\s*=\s*|\s+))(['"]?)[^\s'"]+(['"]?)`)
	emailInTextRE    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
)

// MaskEmailForPush masks the local-part of an email address while keeping a short prefix and domain.
func MaskEmailForPush(email string) string {
	email = strings.TrimSpace(email)
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return email
	}
	local := email[:at]
	domain := email[at+1:]
	if strings.HasSuffix(local, "***") {
		return email
	}
	runes := []rune(local)
	switch {
	case len(runes) > 2:
		return string(runes[:2]) + "***@" + domain
	default:
		return string(runes[:1]) + "***@" + domain
	}
}

// SanitizePushText masks credentials, bearer/API/OAuth tokens, URL userinfo, and email prefixes
// before text leaves the gateway to external push relays or appears on device Lock Screens.
func SanitizePushText(s string) string {
	if s == "" {
		return ""
	}
	s = urlUserInfoRE.ReplaceAllString(s, "${1}***${2}")
	s = authSchemeRE.ReplaceAllString(s, "$1 ***")
	s = ya29TokenRE.ReplaceAllString(s, "ya29.***")
	s = googleRefreshRE.ReplaceAllString(s, "1//***")
	s = jwtTokenRE.ReplaceAllString(s, "eyJ***")
	s = skTokenRE.ReplaceAllString(s, "sk-***")
	s = googleAPIKeyRE.ReplaceAllString(s, "AIza***")
	s = ghTokenRE.ReplaceAllString(s, "gh_***")
	s = kvSecretAssignRE.ReplaceAllString(s, "${1}***")
	s = cliSecretFlagRE.ReplaceAllString(s, "${1}***")
	if strings.IndexByte(s, '@') >= 0 {
		s = emailInTextRE.ReplaceAllStringFunc(s, MaskEmailForPush)
	}
	return s
}

func isLoopbackHost(host string) bool {
	h := strings.TrimSpace(strings.ToLower(host))
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// validatePushEndpointURL enforces HTTPS for all non-loopback push endpoints,
// permitting plain HTTP strictly for loopback unit-test servers (127.0.0.1 / ::1 / localhost).
func validatePushEndpointURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid push endpoint URL")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("refusing insecure non-loopback http push endpoint %q (https required)", u.Host)
	default:
		return fmt.Errorf("unsupported push endpoint scheme %q", u.Scheme)
	}
}
