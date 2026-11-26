package cockpit

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseJetskiOAuthFromEmbeddedJWT(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"new@example.com","exp":2000000000}`))
	jwt := "eyJhbGciOiJub25lIn0." + payload + ".sig"
	blob := "noise ya29.a0TEST_ACCESS_TOKEN more 1//0eREFRESHTOKEN " + jwt
	tok, err := parseJetskiOAuth(blob)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "ya29.a0TEST_ACCESS_TOKEN" {
		t.Fatalf("access = %s", tok.AccessToken)
	}
	if tok.RefreshToken != "1//0eREFRESHTOKEN" {
		t.Fatalf("refresh = %s", tok.RefreshToken)
	}
	if tok.Email != "new@example.com" {
		t.Fatalf("email = %s", tok.Email)
	}
	if tok.Expiry.Unix() != 2000000000 {
		t.Fatalf("exp = %s", tok.Expiry)
	}
}

func TestParseJetskiOAuthStripsSQLiteWrap(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"wrap@example.com","exp":2000000000}`))
	jwt := "eyJhbGciOiJub25lIn0." + payload + ".sig"
	inner := "ya29.a0WRAPTOKEN and 1//0eWRAPREFRESH " + jwt
	outer := base64.StdEncoding.EncodeToString([]byte(inner))
	var wrapped strings.Builder
	for i := 0; i < len(outer); i += 80 {
		end := i + 80
		if end > len(outer) {
			end = len(outer)
		}
		wrapped.WriteString(outer[i:end])
		wrapped.WriteByte('\n')
	}
	tok, err := parseJetskiOAuth(wrapped.String())
	if err != nil {
		t.Fatal(err)
	}
	if tok.Email != "wrap@example.com" {
		t.Fatalf("email = %s", tok.Email)
	}
	if tok.AccessToken != "ya29.a0WRAPTOKEN" {
		t.Fatalf("access = %s", tok.AccessToken)
	}
}

func TestGeminiKeychainEnvelope(t *testing.T) {
	tok := &parsedOAuth{
		AccessToken:  "ya29.a0abc",
		RefreshToken: "1//0eref",
		Email:        "new@example.com",
		Expiry:       time.Unix(2000000000, 0).UTC(),
	}
	env := map[string]any{
		"token": map[string]any{
			"access_token":  tok.AccessToken,
			"token_type":    "Bearer",
			"refresh_token": tok.RefreshToken,
			"expiry":        tok.Expiry.UTC().Format("2006-01-02T15:04:05.000000Z"),
		},
		"auth_method": "consumer",
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	secret := "go-keyring-base64:" + base64.StdEncoding.EncodeToString(raw)
	if !strings.HasPrefix(secret, "go-keyring-base64:") {
		t.Fatal(secret)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "go-keyring-base64:"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(decoded, &got); err != nil {
		t.Fatal(err)
	}
	if got["auth_method"] != "consumer" {
		t.Fatalf("%v", got)
	}
}

func TestWriteJetskiOAuthFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("MGY_ANTIGRAVITY_DATA_DIR", "")
	t.Setenv("MULTIGRAVITY_ANTIGRAVITY_DATA_DIR", "")
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", "")

	tok := &parsedOAuth{
		AccessToken:  "ya29.a0abc",
		RefreshToken: "1//0eref",
		IDToken:      "eyJ.e30.sig",
		Email:        "New@Example.com",
		Expiry:       time.Unix(2000000000, 0),
	}
	if err := writeJetskiOAuthFile(tok); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(tmp, ".gemini", "jetski-standalone-oauth-token"))
	if err != nil {
		t.Fatal(err)
	}
	var doc jetskiOAuthFile
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Token.AccessToken != "ya29.a0abc" || doc.IDToken != "eyJ.e30.sig" || doc.AuthMethod != "consumer" {
		t.Fatalf("unexpected file: %+v", doc)
	}
	if doc.IDTokenEmail != "new@example.com" {
		t.Fatalf("id_token_email = %q, want new@example.com", doc.IDTokenEmail)
	}
	if doc.Token.ExpiresIn <= 0 {
		t.Fatalf("token.expires_in = %d, want > 0", doc.Token.ExpiresIn)
	}
	if !strings.Contains(doc.Token.Expiry, "2033") && !strings.Contains(doc.Token.Expiry, "2000000000") {
		// RFC3339 of unix 2000000000 is 2033-05-18
		if !strings.HasPrefix(doc.Token.Expiry, "2033-") {
			t.Fatalf("expiry = %s", doc.Token.Expiry)
		}
	}
	// Non-existent antigravity / antigravity-cli dirs must not be created implicitly.
	if _, err := os.Stat(filepath.Join(tmp, ".gemini", "antigravity-cli")); !os.IsNotExist(err) {
		t.Fatalf("expected ~/.gemini/antigravity-cli not to be created, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".gemini", "antigravity")); !os.IsNotExist(err) {
		t.Fatalf("expected ~/.gemini/antigravity not to be created, err=%v", err)
	}
}

func TestWriteJetskiOAuthFileSyncsAntigravityDirsAndDedupesSymlinks(t *testing.T) {
	t.Run("cli_only_dir_synced_with_jwt_email_fallback", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("USERPROFILE", tmp)
		t.Setenv("MGY_ANTIGRAVITY_DATA_DIR", "")
		t.Setenv("MULTIGRAVITY_ANTIGRAVITY_DATA_DIR", "")
		t.Setenv("ANTIGRAVITY_APP_DATA_DIR", "")

		cliDir := filepath.Join(tmp, ".gemini", "antigravity-cli")
		if err := os.MkdirAll(cliDir, 0o700); err != nil {
			t.Fatal(err)
		}

		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"cli-user@example.com","exp":2000000000}`))
		tok := &parsedOAuth{
			AccessToken:  "ya29.a0cli",
			RefreshToken: "1//0ecli",
			IDToken:      "eyJhbGciOiJub25lIn0." + payload + ".sig",
			Expiry:       time.Now().Add(45 * time.Minute),
		}
		if err := writeJetskiOAuthFile(tok); err != nil {
			t.Fatal(err)
		}

		cliTokPath := filepath.Join(cliDir, "antigravity-oauth-token")
		raw, err := os.ReadFile(cliTokPath)
		if err != nil {
			t.Fatalf("expected %s to be written: %v", cliTokPath, err)
		}
		var doc jetskiOAuthFile
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		if doc.Token.AccessToken != "ya29.a0cli" || doc.IDTokenEmail != "cli-user@example.com" {
			t.Fatalf("unexpected cli oauth file: %+v", doc)
		}
		if doc.Token.ExpiresIn < 2000 || doc.Token.ExpiresIn > 3000 {
			t.Fatalf("token.expires_in = %d, want ~2700", doc.Token.ExpiresIn)
		}
		if _, err := os.Stat(filepath.Join(tmp, ".gemini", "antigravity")); !os.IsNotExist(err) {
			t.Fatalf("expected non-existent ~/.gemini/antigravity to stay absent, err=%v", err)
		}
	})

	t.Run("both_dirs_and_custom_override_synced", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("USERPROFILE", tmp)
		customDir := filepath.Join(tmp, "custom-agy")
		t.Setenv("MGY_ANTIGRAVITY_DATA_DIR", customDir)
		t.Setenv("MULTIGRAVITY_ANTIGRAVITY_DATA_DIR", "")
		t.Setenv("ANTIGRAVITY_APP_DATA_DIR", "")

		dirs := []string{
			customDir,
			filepath.Join(tmp, ".gemini", "antigravity"),
			filepath.Join(tmp, ".gemini", "antigravity-cli"),
		}
		for _, d := range dirs {
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
		}

		tok := &parsedOAuth{
			AccessToken:  "ya29.a0both",
			RefreshToken: "1//0eboth",
			Email:        "both@example.com",
			Expiry:       time.Now().Add(30 * time.Minute),
		}
		if err := writeJetskiOAuthFile(tok); err != nil {
			t.Fatal(err)
		}

		for _, d := range dirs {
			p := filepath.Join(d, "antigravity-oauth-token")
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("missing synced token at %s: %v", p, err)
			}
			var doc jetskiOAuthFile
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("unmarshal %s: %v", p, err)
			}
			if doc.Token.AccessToken != "ya29.a0both" || doc.IDTokenEmail != "both@example.com" || doc.Token.ExpiresIn <= 0 {
				t.Fatalf("unexpected doc at %s: %+v", p, doc)
			}
		}
	})

	t.Run("symlinked_dirs_deduplicated", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("USERPROFILE", tmp)
		t.Setenv("MGY_ANTIGRAVITY_DATA_DIR", "")
		t.Setenv("MULTIGRAVITY_ANTIGRAVITY_DATA_DIR", "")
		t.Setenv("ANTIGRAVITY_APP_DATA_DIR", "")

		desktopDir := filepath.Join(tmp, ".gemini", "antigravity")
		cliLink := filepath.Join(tmp, ".gemini", "antigravity-cli")
		if err := os.MkdirAll(desktopDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(desktopDir, cliLink); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}

		targets := antigravityOAuthTargetPaths()
		if len(targets) != 1 {
			t.Fatalf("expected 1 deduplicated target path, got %d: %v", len(targets), targets)
		}
	})
}

func TestInvalidateLanguageServerCache(t *testing.T) {
	cacheFoundLS(45678, "old-csrf")
	lsLookupMu.Lock()
	lastLSFailScan = time.Now()
	lsLookupMu.Unlock()

	invalidateLanguageServerCache()

	lsLookupMu.Lock()
	defer lsLookupMu.Unlock()
	if cachedLSPort != 0 || cachedLSCSRF != "" || !cachedLSUntil.IsZero() || !lastLSFailScan.IsZero() {
		t.Fatalf("expected LS cache cleared, got port=%d csrf=%q until=%v fail=%v", cachedLSPort, cachedLSCSRF, cachedLSUntil, lastLSFailScan)
	}
}
