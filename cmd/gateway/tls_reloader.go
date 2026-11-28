package main

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

const tlsCertStatInterval = 30 * time.Second

type certReloader struct {
	certFile string
	keyFile  string
	interval time.Duration

	mu          sync.RWMutex
	cert        *tls.Certificate
	certModTime time.Time
	certSize    int64
	keyModTime  time.Time
	keySize     int64
	lastCheck   time.Time
}

func newCertReloader(certFile, keyFile string, checkInterval time.Duration) (*certReloader, error) {
	if checkInterval <= 0 {
		checkInterval = tlsCertStatInterval
	}
	r := &certReloader{
		certFile: certFile,
		keyFile:  keyFile,
		interval: checkInterval,
	}
	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *certReloader) loadLocked() error {
	certInfo, err := os.Stat(r.certFile)
	if err != nil {
		return fmt.Errorf("stat TLS cert %q: %w", r.certFile, err)
	}
	keyInfo, err := os.Stat(r.keyFile)
	if err != nil {
		return fmt.Errorf("stat TLS key %q: %w", r.keyFile, err)
	}
	loaded, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return fmt.Errorf("load TLS key pair (%q, %q): %w", r.certFile, r.keyFile, err)
	}
	r.cert = &loaded
	r.certModTime = certInfo.ModTime()
	r.certSize = certInfo.Size()
	r.keyModTime = keyInfo.ModTime()
	r.keySize = keyInfo.Size()
	r.lastCheck = time.Now()
	return nil
}

func (r *certReloader) getCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	now := time.Now()
	r.mu.RLock()
	cached := r.cert
	stale := now.Sub(r.lastCheck) >= r.interval
	r.mu.RUnlock()

	if !stale && cached != nil {
		return cached, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.lastCheck) < r.interval && r.cert != nil {
		return r.cert, nil
	}
	r.lastCheck = time.Now()

	certInfo, certErr := os.Stat(r.certFile)
	keyInfo, keyErr := os.Stat(r.keyFile)
	if certErr != nil || keyErr != nil {
		slog.Warn("[TLS] Failed to stat certificate files for hot-reload; keeping active certificate",
			"cert_err", certErr, "key_err", keyErr)
		return r.cert, nil
	}
	if certInfo.ModTime().Equal(r.certModTime) && certInfo.Size() == r.certSize &&
		keyInfo.ModTime().Equal(r.keyModTime) && keyInfo.Size() == r.keySize {
		return r.cert, nil
	}

	loaded, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		slog.Warn("[TLS] Failed to hot-reload updated TLS certificate; keeping previous certificate", "err", err)
		return r.cert, nil
	}
	r.cert = &loaded
	r.certModTime = certInfo.ModTime()
	r.certSize = certInfo.Size()
	r.keyModTime = keyInfo.ModTime()
	r.keySize = keyInfo.Size()
	slog.Info("[TLS] Reloaded updated TLS certificate from disk", "cert", r.certFile)
	return r.cert, nil
}

func newReloadingTLSConfig(certFile, keyFile string, checkInterval time.Duration) (*tls.Config, error) {
	reloader, err := newCertReloader(certFile, keyFile, checkInterval)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1"},
		GetCertificate: reloader.getCertificate,
	}, nil
}
