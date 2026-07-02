package redistore

import (
	"strings"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
)

const (
	// Error messages for Redis connection failures in tests
	errRedisRefused     = "failed to connect to Redis: dial tcp :6379: connect: connection refused"
	errRedisRefusedIPv6 = "failed to connect to Redis: dial tcp [::1]:6379: connect: connection refused"
)

// TestNewStore_NoConnectionOption tests that NewStore returns error
// when no connection option is provided
func TestNewStore_NoConnectionOption(t *testing.T) {
	_, err := NewStore(
		[][]byte{[]byte("secret-key")},
		WithMaxLength(8192),
	)
	if err == nil {
		t.Fatal("Expected error when no connection option provided")
	}
	expectedErr := "invalid configuration: exactly one connection option is required: " +
		"use WithPool, WithAddress, or WithURL"
	if err.Error() != expectedErr {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestNewStore_MultipleConnectionOptions tests that NewStore returns error
// when multiple connection options are provided
func TestNewStore_MultipleConnectionOptions(t *testing.T) {
	_, err := NewStore(
		[][]byte{[]byte("secret-key")},
		WithAddress("tcp", ":6379"),
		WithURL("redis://localhost:6379"),
	)
	if err == nil {
		t.Fatal("Expected error when multiple connection options provided")
	}
	expectedErr := "invalid configuration: only one connection option can be specified: " +
		"WithPool, WithAddress, or WithURL are mutually exclusive"
	if err.Error() != expectedErr {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// expectedURLFieldErrPrefix is the prefix shared by every URL/option
// per-field conflict error. The validator appends the specific field
// name, so tests only assert on the prefix to stay decoupled from the
// exact wording of each message.
const expectedURLFieldErrPrefix = "invalid configuration: WithURL already specifies a "

// TestParseRedisURL exercises the URL parser used by both validate() and
// buildPool(). It does not need a running Redis.
func TestParseRedisURL(t *testing.T) {
	t.Run("Valid URLs", func(t *testing.T) {
		cases := []struct {
			raw      string
			network  string
			address  string
			username string
			password string
			db       string
		}{
			{"redis://localhost:6379", "tcp", "localhost:6379", "", "", ""},
			{"redis://localhost:6379/0", "tcp", "localhost:6379", "", "", "0"},
			{"redis://localhost:6379/5", "tcp", "localhost:6379", "", "", "5"},
			{"redis://:pw@localhost:6379", "tcp", "localhost:6379", "", "pw", ""},
			{"redis://user@localhost:6379", "tcp", "localhost:6379", "user", "", ""},
			{"redis://user:pw@localhost:6379/3", "tcp", "localhost:6379", "user", "pw", "3"},
			{"rediss://localhost:6379", "tcp", "localhost:6379", "", "", ""},
			{"unix:///tmp/redis.sock", "unix", "/tmp/redis.sock", "", "", ""},
		}

		for _, tc := range cases {
			t.Run(tc.raw, func(t *testing.T) {
				parts, err := parseRedisURL(tc.raw)
				if err != nil {
					t.Fatalf("parseRedisURL(%q) returned error: %v", tc.raw, err)
				}
				if parts.network != tc.network {
					t.Errorf("network: want %q, got %q", tc.network, parts.network)
				}
				if parts.address != tc.address {
					t.Errorf("address: want %q, got %q", tc.address, parts.address)
				}
				if parts.username != tc.username {
					t.Errorf("username: want %q, got %q", tc.username, parts.username)
				}
				if parts.password != tc.password {
					t.Errorf("password: want %q, got %q", tc.password, parts.password)
				}
				if parts.db != tc.db {
					t.Errorf("db: want %q, got %q", tc.db, parts.db)
				}
			})
		}
	})

	t.Run("Invalid scheme", func(t *testing.T) {
		_, err := parseRedisURL("http://localhost:6379")
		if err == nil {
			t.Fatal("Expected error for unsupported scheme")
		}
	})
}

// TestNewStore_URLFieldConflict covers the per-field conflicts that the
// validator must reject: URL+option both set the username, the password,
// or a non-default database. For cases that produce two simultaneous
// conflicts we accept either field name in the error message.
func TestNewStore_URLFieldConflict(t *testing.T) {
	cases := []struct {
		name        string
		url         string
		opt         Option
		mustMention string   // error must mention this fragment
		orMention   []string // OR one of these (for ambiguous cases)
	}{
		{
			name:        "URL user + WithAuth user",
			url:         "redis://user@localhost:6379",
			opt:         WithAuth("user", "pw"),
			mustMention: "username",
		},
		{
			name:        "URL user:pw + WithAuth user",
			url:         "redis://user:pw@localhost:6379",
			opt:         WithAuth("user", "pw"),
			mustMention: "username",
		},
		{
			name:        "URL password + WithPassword",
			url:         "redis://:pw@localhost:6379",
			opt:         WithPassword("pw"),
			mustMention: "password",
		},
		{
			name:        "URL user:pw + WithPassword",
			url:         "redis://user:pw@localhost:6379",
			opt:         WithPassword("pw"),
			mustMention: "password",
		},
		{
			name:        "URL user:pw + WithAuth mismatched",
			url:         "redis://user:pw@localhost:6379",
			opt:         WithAuth("user", "other"),
			mustMention: "username",
			orMention:   []string{"password"},
		},
		{
			name:        "URL db /5 + WithDB(\"5\")",
			url:         "redis://localhost:6379/5",
			opt:         WithDB("5"),
			mustMention: "database",
		},
		{
			name:        "URL db /5 + WithDB(\"7\")",
			url:         "redis://localhost:6379/5",
			opt:         WithDB("7"),
			mustMention: "database",
		},
		{
			name:        "URL db /0 + WithDB(\"5\")",
			url:         "redis://localhost:6379/0",
			opt:         WithDB("5"),
			mustMention: "database",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewStore(
				[][]byte{[]byte("secret-key")},
				WithURL(tc.url),
				tc.opt,
			)
			if err == nil {
				t.Fatalf("Expected conflict error, got nil")
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, expectedURLFieldErrPrefix) {
				t.Errorf("Unexpected error prefix: %v", err)
			}
			if !strings.Contains(msg, tc.mustMention) &&
				!anyContains(msg, tc.orMention) {
				t.Errorf("Error should mention %q (or one of %v), got: %v",
					tc.mustMention, tc.orMention, err)
			}
		})
	}
}

func anyContains(s string, fragments []string) bool {
	for _, f := range fragments {
		if strings.Contains(s, f) {
			return true
		}
	}
	return false
}

// TestNewStore_URLFieldMerge covers the cases the user can reasonably
// expect to work: the URL leaves a field empty, and the matching option
// fills it in. These must NOT trigger a conflict error during validation.
// We skip the test if no Redis is available (validation will then fail
// later with a connection error, which is expected and unrelated).
func TestNewStore_URLFieldMerge(t *testing.T) {
	cases := []struct {
		name string
		opts []Option
	}{
		{
			name: "URL no auth + WithAuth",
			opts: []Option{WithURL("redis://localhost:6379"), WithAuth("u", "p")},
		},
		{
			name: "URL no auth + WithPassword",
			opts: []Option{WithURL("redis://localhost:6379"), WithPassword("p")},
		},
		{
			name: "URL no db + WithDB",
			opts: []Option{WithURL("redis://localhost:6379"), WithDB("5")},
		},
		{
			name: "URL username only + WithPassword",
			opts: []Option{WithURL("redis://user@localhost:6379"), WithPassword("p")},
		},
		{
			name: "URL password only + WithAuth username only",
			// WithAuth("u", "") keeps cfg.password empty so the URL's
			// password does not collide; the option's username fills in.
			opts: []Option{WithURL("redis://:pw@localhost:6379"), WithAuth("u", "")},
		},
		{
			name: "URL no db /0 + WithDB(\"0\") default",
			opts: []Option{WithURL("redis://localhost:6379/0"), WithDB("0")},
		},
		{
			name: "URL db /5 + WithDB(\"0\") default",
			opts: []Option{WithURL("redis://localhost:6379/5"), WithDB("0")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewStore(
				[][]byte{[]byte("secret-key")},
				tc.opts...,
			)
			// The validation must not have fired. Any error here is a
			// downstream connection error (no Redis in test env), which
			// we tolerate.
			if err != nil && strings.HasPrefix(err.Error(), expectedURLFieldErrPrefix) {
				t.Fatalf("Validator should have accepted the merge, got: %v", err)
			}
		})
	}
}

// TestNewStore_NoKeyPairs tests that NewStore returns error when no key pairs are provided
func TestNewStore_NoKeyPairs(t *testing.T) {
	_, err := NewStore(
		[][]byte{}, // empty key pairs
		WithAddress("tcp", ":6379"),
	)
	if err == nil {
		t.Fatal("Expected error when no key pairs provided")
	}
	if err.Error() != "at least one key pair is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestWithDB_InvalidDB tests database number validation
func TestWithDB_InvalidDB(t *testing.T) {
	cfg := defaultConfig()

	// Test invalid string
	err := WithDB("invalid")(cfg)
	if err == nil {
		t.Error("Expected error for invalid DB string")
	}

	// Test out of range
	err = WithDB("16")(cfg)
	if err == nil {
		t.Error("Expected error for DB number out of range")
	}

	err = WithDB("-1")(cfg)
	if err == nil {
		t.Error("Expected error for negative DB number")
	}
}

// TestWithDBNum_InvalidDB tests database number validation with integer
func TestWithDBNum_InvalidDB(t *testing.T) {
	cfg := defaultConfig()

	err := WithDBNum(16)(cfg)
	if err == nil {
		t.Error("Expected error for DB number 16")
	}

	err = WithDBNum(-1)(cfg)
	if err == nil {
		t.Error("Expected error for negative DB number")
	}
}

// TestWithPoolSize_InvalidSize tests pool size validation
func TestWithPoolSize_InvalidSize(t *testing.T) {
	cfg := defaultConfig()

	err := WithPoolSize(0)(cfg)
	if err == nil {
		t.Error("Expected error for zero pool size")
	}

	err = WithPoolSize(-1)(cfg)
	if err == nil {
		t.Error("Expected error for negative pool size")
	}
}

// TestWithMaxLength_InvalidLength tests max length validation
func TestWithMaxLength_InvalidLength(t *testing.T) {
	cfg := defaultConfig()

	err := WithMaxLength(-1)(cfg)
	if err == nil {
		t.Error("Expected error for negative max length")
	}
}

// TestWithIdleTimeout_Negative tests idle timeout validation
func TestWithIdleTimeout_Negative(t *testing.T) {
	cfg := defaultConfig()

	err := WithIdleTimeout(-1 * time.Second)(cfg)
	if err == nil {
		t.Error("Expected error for negative idle timeout")
	}
}

// TestWithDefaultMaxAge_Negative tests default max age validation
func TestWithDefaultMaxAge_Negative(t *testing.T) {
	cfg := defaultConfig()

	err := WithDefaultMaxAge(-1)(cfg)
	if err == nil {
		t.Error("Expected error for negative default max age")
	}
}

// TestWithPool_Nil tests that nil pool is rejected
func TestWithPool_Nil(t *testing.T) {
	cfg := defaultConfig()

	err := WithPool(nil)(cfg)
	if err == nil {
		t.Error("Expected error for nil pool")
	}
}

// TestWithAddress_Empty tests that empty network/address is rejected
func TestWithAddress_Empty(t *testing.T) {
	cfg := defaultConfig()

	err := WithAddress("", "address")(cfg)
	if err == nil {
		t.Error("Expected error for empty network")
	}

	err = WithAddress("tcp", "")(cfg)
	if err == nil {
		t.Error("Expected error for empty address")
	}
}

// TestWithURL_Empty tests that empty URL is rejected
func TestWithURL_Empty(t *testing.T) {
	cfg := defaultConfig()

	err := WithURL("")(cfg)
	if err == nil {
		t.Error("Expected error for empty URL")
	}
}

// TestWithSerializer_Nil tests that nil serializer is rejected
func TestWithSerializer_Nil(t *testing.T) {
	cfg := defaultConfig()

	err := WithSerializer(nil)(cfg)
	if err == nil {
		t.Error("Expected error for nil serializer")
	}
}

// TestDefaultConfig tests default configuration values
func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.db != "0" {
		t.Errorf("Expected default db to be '0', got '%s'", cfg.db)
	}
	if cfg.poolSize != 10 {
		t.Errorf("Expected default poolSize to be 10, got %d", cfg.poolSize)
	}
	if cfg.idleTimeout != 240*time.Second {
		t.Errorf("Expected default idleTimeout to be 240s, got %v", cfg.idleTimeout)
	}
	if cfg.maxLength != 4096 {
		t.Errorf("Expected default maxLength to be 4096, got %d", cfg.maxLength)
	}
	if cfg.keyPrefix != "session_" {
		t.Errorf("Expected default keyPrefix to be 'session_', got '%s'", cfg.keyPrefix)
	}
	if cfg.defaultMaxAge != 1200 {
		t.Errorf("Expected default defaultMaxAge to be 1200, got %d", cfg.defaultMaxAge)
	}
	if cfg.sessionOpts == nil {
		t.Error("Expected sessionOpts to be initialized")
	}
	if cfg.sessionOpts.Path != "/" {
		t.Errorf("Expected default path to be '/', got '%s'", cfg.sessionOpts.Path)
	}
	if cfg.sessionOpts.MaxAge != sessionExpire {
		t.Errorf("Expected default MaxAge to be %d, got %d", sessionExpire, cfg.sessionOpts.MaxAge)
	}
}

// TestWithPool_Configuration tests WithPool option
func TestWithPool_Configuration(t *testing.T) {
	pool := &redis.Pool{
		MaxIdle: 5,
	}

	cfg := defaultConfig()
	err := WithPool(pool)(cfg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if cfg.pool != pool {
		t.Error("Pool not set correctly")
	}
}

// TestWithAddress_Configuration tests WithAddress option
func TestWithAddress_Configuration(t *testing.T) {
	cfg := defaultConfig()
	err := WithAddress("tcp", "localhost:6379")(cfg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if cfg.address == nil {
		t.Fatal("Address not set")
	}
	if cfg.address.network != "tcp" {
		t.Errorf("Expected network 'tcp', got '%s'", cfg.address.network)
	}
	if cfg.address.address != "localhost:6379" {
		t.Errorf("Expected address 'localhost:6379', got '%s'", cfg.address.address)
	}
}

// TestWithURL_Configuration tests WithURL option
func TestWithURL_Configuration(t *testing.T) {
	cfg := defaultConfig()
	err := WithURL("redis://localhost:6379")(cfg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if cfg.url != "redis://localhost:6379" {
		t.Errorf("Expected URL 'redis://localhost:6379', got '%s'", cfg.url)
	}
}

// TestWithAuth_Configuration tests WithAuth option
func TestWithAuth_Configuration(t *testing.T) {
	cfg := defaultConfig()
	err := WithAuth("user", "pass")(cfg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if cfg.username != "user" {
		t.Errorf("Expected username 'user', got '%s'", cfg.username)
	}
	if cfg.password != "pass" {
		t.Errorf("Expected password 'pass', got '%s'", cfg.password)
	}
}

// TestWithDB_Configuration tests WithDB option
func TestWithDB_Configuration(t *testing.T) {
	cfg := defaultConfig()
	err := WithDB("5")(cfg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if cfg.db != "5" {
		t.Errorf("Expected db '5', got '%s'", cfg.db)
	}
}

// TestWithMaxLength_Configuration tests WithMaxLength option
func TestWithMaxLength_Configuration(t *testing.T) {
	cfg := defaultConfig()
	err := WithMaxLength(8192)(cfg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if cfg.maxLength != 8192 {
		t.Errorf("Expected maxLength 8192, got %d", cfg.maxLength)
	}
}

// TestWithKeyPrefix_Configuration tests WithKeyPrefix option
func TestWithKeyPrefix_Configuration(t *testing.T) {
	cfg := defaultConfig()
	err := WithKeyPrefix("myapp_")(cfg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if cfg.keyPrefix != "myapp_" {
		t.Errorf("Expected keyPrefix 'myapp_', got '%s'", cfg.keyPrefix)
	}
}

// TestWithSerializer_Configuration tests WithSerializer option
func TestWithSerializer_Configuration(t *testing.T) {
	cfg := defaultConfig()
	serializer := JSONSerializer{}
	err := WithSerializer(serializer)(cfg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if _, ok := cfg.serializer.(JSONSerializer); !ok {
		t.Error("Expected JSONSerializer, got different type")
	}
}

// TestNewStore_KeyRotation tests key rotation with multiple key pairs
func TestNewStore_KeyRotation(t *testing.T) {
	// Test with multiple key pairs for rotation
	_, err := NewStore(
		[][]byte{
			[]byte("new-authentication-key-32-bytes!"),
			[]byte("new-encrypt-key-32-bytes-long!"),
			[]byte("old-authentication-key-32-bytes!"),
			[]byte("old-encrypt-key-32-bytes-long!"),
		},
		WithAddress("tcp", ":6379"),
	)

	// Error expected if Redis is not running, but the signature should work
	if err != nil && err.Error() != errRedisRefused && err.Error() != errRedisRefusedIPv6 {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// TestNewStore_SingleKey tests single key configuration
func TestNewStore_SingleKey(t *testing.T) {
	// Test with a single key
	_, err := NewStore(
		[][]byte{[]byte("single-secret-key")},
		WithAddress("tcp", ":6379"),
	)

	// Error expected if Redis is not running, but the signature should work
	if err != nil && err.Error() != errRedisRefused && err.Error() != errRedisRefusedIPv6 {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// TestKeys tests the Keys helper function
func TestKeys(t *testing.T) {
	// Test Keys with single key
	keys := Keys([]byte("secret-key"))
	if len(keys) != 1 {
		t.Errorf("Expected 1 key, got %d", len(keys))
	}
	if string(keys[0]) != "secret-key" {
		t.Errorf("Expected 'secret-key', got '%s'", string(keys[0]))
	}

	// Test Keys with multiple keys
	keys = Keys(
		[]byte("auth-key"),
		[]byte("encrypt-key"),
		[]byte("old-auth-key"),
		[]byte("old-encrypt-key"),
	)
	if len(keys) != 4 {
		t.Errorf("Expected 4 keys, got %d", len(keys))
	}

	// Use Keys with NewStore
	_, err := NewStore(
		Keys([]byte("test-key")),
		WithAddress("tcp", ":6379"),
	)
	// Error expected if Redis is not running
	if err != nil && err.Error() != errRedisRefused && err.Error() != errRedisRefusedIPv6 {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// TestKeysFromStrings tests the KeysFromStrings helper function
func TestKeysFromStrings(t *testing.T) {
	// Test KeysFromStrings with single key
	keys := KeysFromStrings("secret-key")
	if len(keys) != 1 {
		t.Errorf("Expected 1 key, got %d", len(keys))
	}
	if string(keys[0]) != "secret-key" {
		t.Errorf("Expected 'secret-key', got '%s'", string(keys[0]))
	}

	// Test KeysFromStrings with multiple keys
	keys = KeysFromStrings(
		"new-auth-key",
		"new-encrypt-key",
		"old-auth-key",
		"old-encrypt-key",
	)
	if len(keys) != 4 {
		t.Errorf("Expected 4 keys, got %d", len(keys))
	}
	if string(keys[0]) != "new-auth-key" {
		t.Errorf("Expected 'new-auth-key', got '%s'", string(keys[0]))
	}

	// Use KeysFromStrings with NewStore
	_, err := NewStore(
		KeysFromStrings("test-key"),
		WithAddress("tcp", ":6379"),
	)
	// Error expected if Redis is not running
	if err != nil && err.Error() != errRedisRefused && err.Error() != errRedisRefusedIPv6 {
		t.Fatalf("Unexpected error: %v", err)
	}
}

// TestKeysFromStrings_EmptyKeys tests KeysFromStrings with empty input
func TestKeysFromStrings_EmptyKeys(t *testing.T) {
	keys := KeysFromStrings()
	if len(keys) != 0 {
		t.Errorf("Expected 0 keys, got %d", len(keys))
	}

	// Should fail validation in NewStore
	_, err := NewStore(
		KeysFromStrings(),
		WithAddress("tcp", ":6379"),
	)
	if err == nil {
		t.Fatal("Expected error for empty keys")
	}
	if err.Error() != "at least one key pair is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}
