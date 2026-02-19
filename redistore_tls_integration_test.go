package redistore

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// TestRedisTLSIntegration is an optional integration test that requires a
// TLS-enabled Redis instance. It is skipped unless the environment variable
// TLS_REDIS_ADDR is set (for example: "localhost:6380").
//
// Optional environment variables:
// - TLS_REDIS_ADDR: host:port of TLS-enabled Redis (required to run)
// - TLS_SKIP_VERIFY: set to "1" to skip server cert verification (testing only)
// - TLS_REDIS_CA: path to PEM file containing CA(s) to trust for the server cert
//
// NOTE:
// The current github action used for Redis does not support TLS.
// This one does but it does not support password authentication.
// https://github.com/marketplace/actions/actions-setup-redis
//
//	So there is a script that can be run locally to run the Redis TLS integration test.
//	See scripts/run-redis-tls-and-test.sh
func TestRedisTLSIntegration(t *testing.T) {
	addr := os.Getenv("TLS_REDIS_ADDR")
	if addr == "" {
		t.Skip("TLS integration test skipped; set TLS_REDIS_ADDR to run")
	}

	caPath := os.Getenv("TLS_REDIS_CA")
	skipVerify := os.Getenv("TLS_SKIP_VERIFY") == "1"

	var tlsCfg *tls.Config
	if caPath != "" {
		// #nosec g703
		data, err := os.ReadFile(caPath)
		if err != nil {
			t.Fatalf("failed to read CA file: %v", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(data) {
			t.Fatalf("failed to parse CA file: %s", caPath)
		}
		tlsCfg = &tls.Config{RootCAs: pool}
	}

	opts := []Option{
		WithAddress("tcp", addr),
		WithPoolSize(5),
	}
	if tlsCfg != nil {
		opts = append(opts, WithTLS(true, false, tlsCfg))
	} else if skipVerify {
		opts = append(opts, WithTLS(true, true, nil))
	} else {
		// default to skip-verify when no CA is provided
		opts = append(opts, WithTLS(true, true, nil))
	}

	store, err := NewStore(
		[][]byte{[]byte("integration-key")},
		opts...,
	)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	ok, err := store.ping()
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected PONG from Redis")
	}

	// Also exercise Save to ensure TLS-backed SETEX works
	req, _ := http.NewRequest("GET", "https://example.local/", nil)
	rr := httptest.NewRecorder()
	sess, err := store.New(req, "tls-integration")
	if err != nil {
		t.Fatalf("store.New failed: %v", err)
	}
	sess.Values["k"] = "v"
	if err := store.Save(req, rr, sess); err != nil {
		t.Fatalf("store.Save failed: %v", err)
	}
}
