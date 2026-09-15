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

	tok := &parsedOAuth{
		AccessToken:  "ya29.a0abc",
		RefreshToken: "1//0eref",
		IDToken:      "eyJ.e30.sig",
		Email:        "new@example.com",
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
	if !strings.Contains(doc.Token.Expiry, "2033") && !strings.Contains(doc.Token.Expiry, "2000000000") {
		// RFC3339 of unix 2000000000 is 2033-05-18
		if !strings.HasPrefix(doc.Token.Expiry, "2033-") {
			t.Fatalf("expiry = %s", doc.Token.Expiry)
		}
	}
}
