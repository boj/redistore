package redistore

import (
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
)

// TestNewStore_NoConnectionOption tests that NewStore returns error when no connection option is provided
func TestNewStore_NoConnectionOption(t *testing.T) {
	_, err := NewStore(
		[]byte("secret-key"),
		WithMaxLength(8192),
	)
	if err == nil {
		t.Fatal("Expected error when no connection option provided")
	}
	if err.Error() != "invalid configuration: exactly one connection option is required: use WithPool, WithAddress, or WithURL" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestNewStore_MultipleConnectionOptions tests that NewStore returns error when multiple connection options are provided
func TestNewStore_MultipleConnectionOptions(t *testing.T) {
	_, err := NewStore(
		[]byte("secret-key"),
		WithAddress("tcp", ":6379"),
		WithURL("redis://localhost:6379"),
	)
	if err == nil {
		t.Fatal("Expected error when multiple connection options provided")
	}
	if err.Error() != "invalid configuration: only one connection option can be specified: WithPool, WithAddress, or WithURL are mutually exclusive" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// TestNewStore_NoKeyPairs tests that NewStore returns error when no key pairs are provided
func TestNewStore_NoKeyPairs(t *testing.T) {
	_, err := NewStore(
		[]byte{}, // empty key pairs
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
