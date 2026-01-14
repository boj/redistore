package redistore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gorilla/sessions"
)

const (
	defaultRedisHost = "127.0.0.1"
	defaultRedisPort = "6379"
	testFlashFoo     = "foo"
	testFlashBar     = "bar"
	testFlashBaz     = "baz"
)

func setup() string {
	addr := os.Getenv("REDIS_HOST")
	if addr == "" {
		addr = defaultRedisHost
	}

	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = defaultRedisPort
	}

	return fmt.Sprintf("%s:%s", addr, port)
}

// ----------------------------------------------------------------------------
// ResponseRecorder
// ----------------------------------------------------------------------------
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// ResponseRecorder is an implementation of http.ResponseWriter that
// records its mutations for later inspection in tests.
type ResponseRecorder struct {
	Code      int           // the HTTP response code from WriteHeader
	HeaderMap http.Header   // the HTTP response headers
	Body      *bytes.Buffer // if non-nil, the bytes.Buffer to append written data to
	Flushed   bool
}

// NewRecorder returns an initialized ResponseRecorder.
func NewRecorder() *ResponseRecorder {
	return &ResponseRecorder{
		HeaderMap: make(http.Header),
		Body:      new(bytes.Buffer),
	}
}

// DefaultRemoteAddr is the default remote address to return in RemoteAddr if
// an explicit DefaultRemoteAddr isn't set on ResponseRecorder.
const DefaultRemoteAddr = "1.2.3.4"

// Header returns the response headers.
func (rw *ResponseRecorder) Header() http.Header {
	return rw.HeaderMap
}

// Write always succeeds and writes to rw.Body, if not nil.
func (rw *ResponseRecorder) Write(buf []byte) (int, error) {
	if rw.Body != nil {
		rw.Body.Write(buf)
	}
	if rw.Code == 0 {
		rw.Code = http.StatusOK
	}
	return len(buf), nil
}

// WriteHeader sets rw.Code.
func (rw *ResponseRecorder) WriteHeader(code int) {
	rw.Code = code
}

// Flush sets rw.Flushed to true.
func (rw *ResponseRecorder) Flush() {
	rw.Flushed = true
}

// ----------------------------------------------------------------------------

type FlashMessage struct {
	Type    int
	Message string
}

func createTestStore(t *testing.T, addr string) *RediStore {
	t.Helper()
	store, err := NewStore(
		[][]byte{[]byte("secret-key")},
		WithAddress("tcp", addr),
		WithPoolSize(10),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	return store
}

func createTestStoreWithDB(t *testing.T, addr, db string) *RediStore {
	t.Helper()
	store, err := NewStore(
		[][]byte{[]byte("secret-key")},
		WithAddress("tcp", addr),
		WithDB(db),
		WithPoolSize(10),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	return store
}

func getSession(t *testing.T, store *RediStore, req *http.Request) *sessions.Session {
	t.Helper()
	session, err := store.Get(req, "session-key")
	if err != nil {
		t.Fatalf("Error getting session: %v", err)
	}
	return session
}

func saveSession(t *testing.T, req *http.Request, rsp http.ResponseWriter) {
	t.Helper()
	if err := sessions.Save(req, rsp); err != nil {
		t.Fatalf("Error saving session: %v", err)
	}
}

func getCookies(t *testing.T, rsp *ResponseRecorder) []string {
	t.Helper()
	hdr := rsp.Header()
	cookies, ok := hdr["Set-Cookie"]
	if !ok || len(cookies) != 1 {
		t.Fatalf("No cookies. Header: %s", hdr)
	}
	return cookies
}

func TestRediStore(t *testing.T) {
	gob.Register(FlashMessage{})
	var cookies []string
	t.Run("Round 1", func(t *testing.T) {
		addr := setup()
		store := createTestStore(t, addr)
		defer func() {
			if err := store.Close(); err != nil {
				fmt.Printf("Error closing store: %v\n", err)
			}
		}()

		req, _ := http.NewRequestWithContext(
			context.Background(), "GET", "http://localhost:8080/", nil)
		rsp := NewRecorder()
		session := getSession(t, store, req)
		flashes := session.Flashes()
		if len(flashes) != 0 {
			t.Errorf("Expected empty flashes; Got %v", flashes)
		}
		session.AddFlash(testFlashFoo)
		session.AddFlash(testFlashBar)
		session.AddFlash(testFlashBaz, "custom_key")
		saveSession(t, req, rsp)
		cookies = getCookies(t, rsp)
	})

	t.Run("Round 2", func(t *testing.T) {
		addr := setup()
		store := createTestStore(t, addr)
		defer func() {
			if err := store.Close(); err != nil {
				fmt.Printf("Error closing store: %v\n", err)
			}
		}()

		req, _ := http.NewRequestWithContext(
			context.Background(), "GET", "http://localhost:8080/", nil)
		req.Header.Add("Cookie", cookies[0])
		rsp := NewRecorder()
		session := getSession(t, store, req)
		flashes := session.Flashes()
		if len(flashes) != 2 {
			t.Fatalf("Expected flashes; Got %v", flashes)
		}
		if flashes[0] != testFlashFoo || flashes[1] != testFlashBar {
			t.Errorf("Expected foo,bar; Got %v", flashes)
		}
		flashes = session.Flashes()
		if len(flashes) != 0 {
			t.Errorf("Expected dumped flashes; Got %v", flashes)
		}
		flashes = session.Flashes("custom_key")
		if len(flashes) != 1 {
			t.Errorf("Expected flashes; Got %v", flashes)
		} else if flashes[0] != testFlashBaz {
			t.Errorf("Expected baz; Got %v", flashes)
		}
		flashes = session.Flashes("custom_key")
		if len(flashes) != 0 {
			t.Errorf("Expected dumped flashes; Got %v", flashes)
		}
		session.Options.MaxAge = -1
		saveSession(t, req, rsp)
	})

	t.Run("Round 3", func(t *testing.T) {
		addr := setup()
		store := createTestStore(t, addr)
		defer func() {
			if err := store.Close(); err != nil {
				fmt.Printf("Error closing store: %v\n", err)
			}
		}()

		req, _ := http.NewRequestWithContext(
			context.Background(), "GET", "http://localhost:8080/", nil)
		rsp := NewRecorder()
		session := getSession(t, store, req)
		flashes := session.Flashes()
		if len(flashes) != 0 {
			t.Errorf("Expected empty flashes; Got %v", flashes)
		}
		session.AddFlash(&FlashMessage{42, testFlashFoo})
		saveSession(t, req, rsp)
		cookies = getCookies(t, rsp)
	})

	t.Run("Round 4", func(t *testing.T) {
		addr := setup()
		store := createTestStore(t, addr)
		defer func() {
			if err := store.Close(); err != nil {
				fmt.Printf("Error closing store: %v\n", err)
			}
		}()

		req, _ := http.NewRequestWithContext(
			context.Background(), "GET", "http://localhost:8080/", nil)
		req.Header.Add("Cookie", cookies[0])
		rsp := NewRecorder()
		session := getSession(t, store, req)
		flashes := session.Flashes()
		if len(flashes) != 1 {
			t.Fatalf("Expected flashes; Got %v", flashes)
		}
		custom := flashes[0].(FlashMessage)
		if custom.Type != 42 || custom.Message != testFlashFoo {
			t.Errorf("Expected %#v, got %#v", FlashMessage{42, testFlashFoo}, custom)
		}
		session.Options.MaxAge = -1
		saveSession(t, req, rsp)
	})

	t.Run("Round 6", func(t *testing.T) {
		addr := setup()
		store := createTestStore(t, addr)
		req, err := http.NewRequestWithContext(
			context.Background(), "GET", "http://www.example.com", nil)
		if err != nil {
			t.Fatal("failed to create request", err)
		}
		w := httptest.NewRecorder()

		session, err := store.New(req, "my session")
		if err != nil {
			t.Fatal("failed to create session", err)
		}
		session.Values["big"] = make([]byte, base64.StdEncoding.DecodedLen(4096*2))
		err = session.Save(req, w)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}

		store.SetMaxLength(4096 * 3)
		err = session.Save(req, w)
		if err != nil {
			t.Fatal("failed to Save:", err)
		}
	})

	t.Run("Round 7", func(t *testing.T) {
		addr := setup()
		store := createTestStoreWithDB(t, addr, "1")
		defer func() {
			if err := store.Close(); err != nil {
				fmt.Printf("Error closing store: %v\n", err)
			}
		}()

		req, _ := http.NewRequestWithContext(
			context.Background(), "GET", "http://localhost:8080/", nil)
		rsp := NewRecorder()
		session := getSession(t, store, req)
		flashes := session.Flashes()
		if len(flashes) != 0 {
			t.Errorf("Expected empty flashes; Got %v", flashes)
		}
		session.AddFlash(testFlashFoo)
		saveSession(t, req, rsp)
		cookies = getCookies(t, rsp)

		req.Header.Add("Cookie", cookies[0])
		session = getSession(t, store, req)
		flashes = session.Flashes()
		if len(flashes) != 1 {
			t.Fatalf("Expected flashes; Got %v", flashes)
		}
		if flashes[0] != testFlashFoo {
			t.Errorf("Expected foo,bar; Got %v", flashes)
		}
	})

	t.Run("Round 8", func(t *testing.T) {
		addr := setup()
		store := createTestStore(t, addr)
		store.SetSerializer(JSONSerializer{})
		defer func() {
			if err := store.Close(); err != nil {
				fmt.Printf("Error closing store: %v\n", err)
			}
		}()

		req, _ := http.NewRequestWithContext(
			context.Background(), "GET", "http://localhost:8080/", nil)
		rsp := NewRecorder()
		session := getSession(t, store, req)
		flashes := session.Flashes()
		if len(flashes) != 0 {
			t.Errorf("Expected empty flashes; Got %v", flashes)
		}
		session.AddFlash(testFlashFoo)
		saveSession(t, req, rsp)
		cookies = getCookies(t, rsp)

		req.Header.Add("Cookie", cookies[0])
		session = getSession(t, store, req)
		flashes = session.Flashes()
		if len(flashes) != 1 {
			t.Fatalf("Expected flashes; Got %v", flashes)
		}
		if flashes[0] != testFlashFoo {
			t.Errorf("Expected foo,bar; Got %v", flashes)
		}
	})
}

func TestPingGoodPort(t *testing.T) {
	store, err := NewStore(
		[][]byte{[]byte("secret-key")},
		WithAddress("tcp", ":6379"),
		WithPoolSize(10),
	)
	if err != nil {
		t.Skip("Skipping test: Redis not available:", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			fmt.Printf("Error closing store: %v\n", err)
		}
	}()
	ok, err := store.ping()
	if err != nil {
		t.Error(err.Error())
	}
	if !ok {
		t.Error("Expected server to PONG")
	}
}

func TestPingBadPort(t *testing.T) {
	store, err := NewStore(
		[][]byte{[]byte("secret-key")},
		WithAddress("tcp", ":6378"),
		WithPoolSize(10),
	)
	// This should fail because the port is wrong
	if err == nil {
		defer func() {
			if err := store.Close(); err != nil {
				fmt.Printf("Error closing store: %v\n", err)
			}
		}()
		_, pingErr := store.ping()
		if pingErr == nil {
			t.Error("Expected error connecting to bad port")
		}
	}
}

func TestNewStore_WithURL(t *testing.T) {
	t.Run("Valid URL", func(t *testing.T) {
		store, err := NewStore(
			[][]byte{[]byte("secret-key")},
			WithURL("redis://localhost:6379"),
			WithPoolSize(10),
		)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		defer func() {
			if err := store.Close(); err != nil {
				fmt.Printf("Error closing store: %v\n", err)
			}
		}()

		req, _ := http.NewRequestWithContext(
			context.Background(), "GET", "http://localhost:8080/", nil)
		rsp := NewRecorder()
		session := getSession(t, store, req)
		flashes := session.Flashes()
		if len(flashes) != 0 {
			t.Errorf("Expected empty flashes; Got %v", flashes)
		}
		session.AddFlash(testFlashFoo)
		saveSession(t, req, rsp)
		_ = getCookies(t, rsp)
	})

	t.Run("Invalid URL", func(t *testing.T) {
		_, err := NewStore(
			[][]byte{[]byte("secret-key")},
			WithURL("invalid-url"),
			WithPoolSize(10),
		)
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})
}

// TestSessionCookieMaxAgeZero tests that MaxAge == 0 creates a session cookie
// (no Max-Age attribute) and saves to Redis with DefaultMaxAge TTL.
// This is a regression test for issue #53.
func TestSessionCookieMaxAgeZero(t *testing.T) {
	addr := setup()
	store := createTestStore(t, addr)
	defer func() {
		if err := store.Close(); err != nil {
			fmt.Printf("Error closing store: %v\n", err)
		}
	}()

	var cookies []string

	// Round 1: Create a session with MaxAge = 0 (session cookie)
	t.Run("Create session cookie with MaxAge=0", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(
			context.Background(), "GET", "http://localhost:8080/", nil)
		rsp := NewRecorder()
		session := getSession(t, store, req)

		// Set MaxAge to 0 to create a session cookie
		session.Options.MaxAge = 0
		session.Values["user"] = "testuser"
		session.Values["authenticated"] = true

		saveSession(t, req, rsp)
		cookies = getCookies(t, rsp)

		// Verify cookie is set
		if len(cookies) == 0 {
			t.Fatal("Expected cookie to be set")
		}

		// Verify the cookie doesn't contain an explicit Max-Age=0
		// (session cookies should not have Max-Age attribute set to 0)
		cookieStr := cookies[0]
		if bytes.Contains([]byte(cookieStr), []byte("Max-Age=0")) {
			t.Errorf("Session cookie should not have Max-Age=0, got: %s", cookieStr)
		}

		// Verify session was saved to Redis by checking if we can retrieve it
		conn := store.Pool.Get()
		defer conn.Close()

		// Get the session ID from the session
		if session.ID == "" {
			t.Fatal("Session ID should not be empty after save")
		}

		// Check if the key exists in Redis
		exists, err := conn.Do("EXISTS", store.keyPrefix+session.ID)
		if err != nil {
			t.Fatalf("Error checking Redis key existence: %v", err)
		}
		if exists == int64(0) {
			t.Error("Session should be saved to Redis when MaxAge=0")
		}

		// Verify the TTL is set to DefaultMaxAge (not 0)
		ttl, err := conn.Do("TTL", store.keyPrefix+session.ID)
		if err != nil {
			t.Fatalf("Error getting TTL from Redis: %v", err)
		}
		ttlInt := ttl.(int64)
		if ttlInt <= 0 {
			t.Errorf("Expected positive TTL (DefaultMaxAge), got %d", ttlInt)
		}
		// TTL should be close to DefaultMaxAge (1200 seconds / 20 minutes)
		// Allow some margin for test execution time
		if ttlInt < 1190 || ttlInt > 1200 {
			t.Errorf("Expected TTL close to DefaultMaxAge (1200), got %d", ttlInt)
		}
	})

	// Round 2: Verify the session can be retrieved
	t.Run("Retrieve session cookie", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(
			context.Background(), "GET", "http://localhost:8080/", nil)
		req.Header.Add("Cookie", cookies[0])
		session := getSession(t, store, req)

		// Verify session is not new (it was loaded from Redis)
		if session.IsNew {
			t.Error("Session should not be new, it should be loaded from Redis")
		}

		// Verify session values are preserved
		user, ok := session.Values["user"]
		if !ok || user != "testuser" {
			t.Errorf("Expected user='testuser', got %v", user)
		}

		authenticated, ok := session.Values["authenticated"]
		if !ok || authenticated != true {
			t.Errorf("Expected authenticated=true, got %v", authenticated)
		}
	})

	// Round 3: Verify MaxAge < 0 deletes the session (existing behavior)
	t.Run("Delete session with MaxAge=-1", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(
			context.Background(), "GET", "http://localhost:8080/", nil)
		req.Header.Add("Cookie", cookies[0])
		rsp := NewRecorder()
		session := getSession(t, store, req)

		sessionID := session.ID

		// Set MaxAge to -1 to delete the session
		session.Options.MaxAge = -1
		saveSession(t, req, rsp)

		// Verify session was deleted from Redis
		conn := store.Pool.Get()
		defer conn.Close()

		exists, err := conn.Do("EXISTS", store.keyPrefix+sessionID)
		if err != nil {
			t.Fatalf("Error checking Redis key existence: %v", err)
		}
		if exists != int64(0) {
			t.Error("Session should be deleted from Redis when MaxAge=-1")
		}
	})
}

func ExampleRediStore() {
	// RedisStore
	store, err := NewStore(
		[][]byte{[]byte("secret-key")},
		WithAddress("tcp", ":6379"),
		WithPoolSize(10),
	)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			fmt.Printf("Error closing store: %v\n", err)
		}
	}()
}
