package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/relay"
)

func TestRunTokenCmd_OfflineSignAndVerify(t *testing.T) {
	secret := "byo-relay-test-secret-0123456789"

	var stdout, stderr bytes.Buffer
	err := runTokenCmd([]string{
		"--domain", "dev.example.com",
		"--secret", secret,
		"--endpoint", "wss://relay.example.com/_mgy_tunnel",
		"--region", "home",
		"--json",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runTokenCmd failed: %v", err)
	}

	var parsed struct {
		Token     string            `json:"token"`
		Domain    string            `json:"domain"`
		PublicURL string            `json:"public_url"`
		ShortID   string            `json:"short_id"`
		OwnerHash string            `json:"owner_hash"`
		Region    string            `json:"region"`
		Env       map[string]string `json:"env"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal json output: %v (raw=%s)", err, stdout.String())
	}
	if !strings.HasPrefix(parsed.Token, "mgyr1.") {
		t.Fatalf("expected mgyr1 token, got %q", parsed.Token)
	}
	if parsed.Domain != "dev.example.com" || parsed.PublicURL != "https://dev.example.com" {
		t.Fatalf("unexpected domain/url: %+v", parsed)
	}
	if parsed.Env["MGY_CUSTOM_RELAY_ENDPOINT"] != "wss://relay.example.com/_mgy_tunnel" {
		t.Fatalf("unexpected env endpoint: %+v", parsed.Env)
	}

	// Verify token against a self-hosted relay.Server configured for .example.com
	claims, err := relay.VerifyToken(secret, parsed.Token, time.Now(), 0)
	if err != nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}
	if claims.Domain != "dev.example.com" || claims.Region != "home" {
		t.Fatalf("unexpected verified claims: %+v", claims)
	}

	peeked, err := relay.PeekTokenClaims(parsed.Token)
	if err != nil || peeked.Domain != "dev.example.com" {
		t.Fatalf("PeekTokenClaims failed: err=%v peeked=%+v", err, peeked)
	}
}

func TestRunTokenCmd_ValidationErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runTokenCmd([]string{"--secret", "abc"}, &stdout, &stderr); err == nil {
		t.Fatal("expected error when --domain missing")
	}
	if err := runTokenCmd([]string{"--domain", "dev.example.com", "--secret", ""}, &stdout, &stderr); err == nil {
		t.Fatal("expected error when secret missing")
	}
	if err := runTokenCmd([]string{"--domain", "invalid_host", "--secret", "sec"}, &stdout, &stderr); err == nil {
		t.Fatal("expected error for invalid hostname")
	}
}
