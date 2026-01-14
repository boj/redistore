// Copyright 2012 Brian "bojo" Jones. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package redistore

import (
	"bytes"
	"encoding/base32"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

// sessionExpire defines the duration (in seconds) for which a session will remain valid.
// The current value represents 30 days (86400 seconds per day).
var sessionExpire = 86400 * 30

// SessionSerializer is an interface that defines methods for serializing
// and deserializing session data. Implementations of this interface
// should provide mechanisms to convert session data to and from byte slices.
type SessionSerializer interface {
	Deserialize(d []byte, ss *sessions.Session) error
	Serialize(ss *sessions.Session) ([]byte, error)
}

// JSONSerializer is a struct that provides methods for serializing and
// deserializing data to and from JSON format. It can be used to convert
// Go data structures into JSON strings and vice versa.
type JSONSerializer struct{}

// Serialize converts the session's values into a JSON-encoded byte slice.
// It returns an error if any of the session keys are not strings.
//
// Parameters:
//
//	ss - A pointer to the session to be serialized.
//
// Returns:
//
//	A byte slice containing the JSON-encoded session values, or an error if
//	serialization fails.
func (s JSONSerializer) Serialize(ss *sessions.Session) ([]byte, error) {
	m := make(map[string]interface{}, len(ss.Values))
	for k, v := range ss.Values {
		ks, ok := k.(string)
		if !ok {
			err := fmt.Errorf("non-string key value, cannot serialize session to JSON: %v", k)
			fmt.Printf("redistore.JSONSerializer.serialize() Error: %v", err)
			return nil, err
		}
		m[ks] = v
	}
	return json.Marshal(m)
}

// Deserialize takes a byte slice and a pointer to a sessions.Session,
// and attempts to deserialize the byte slice into the session's Values map.
// It returns an error if the deserialization process fails.
//
// Parameters:
// - d: A byte slice containing the serialized session data.
// - ss: A pointer to the sessions.Session where the deserialized data will be stored.
//
// Returns:
// - An error if the deserialization process fails, otherwise nil.
func (s JSONSerializer) Deserialize(d []byte, ss *sessions.Session) error {
	m := make(map[string]interface{})
	err := json.Unmarshal(d, &m)
	if err != nil {
		fmt.Printf("redistore.JSONSerializer.deserialize() Error: %v", err)
		return err
	}
	for k, v := range m {
		ss.Values[k] = v
	}
	return nil
}

// GobSerializer is a struct that provides methods for serializing and
// deserializing data using the Gob encoding format. Gob is a binary
// serialization format that is efficient and compact, making it suitable
// for encoding complex data structures in Go.
type GobSerializer struct{}

// Serialize encodes the session values using gob encoding and returns the
// serialized byte slice. If the encoding process encounters an error, it
// returns nil and the error.
//
// Parameters:
//
//	ss - A pointer to the session to be serialized.
//
// Returns:
//
//	A byte slice containing the serialized session values, or nil if an
//	error occurred during encoding. The error encountered during encoding
//	is also returned.
func (s GobSerializer) Serialize(ss *sessions.Session) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	err := enc.Encode(ss.Values)
	if err == nil {
		return buf.Bytes(), nil
	}
	return nil, err
}

// Deserialize decodes the given byte slice into the session's Values field.
// It uses the gob package to perform the decoding.
//
// Parameters:
//
//	d - The byte slice to be deserialized.
//	ss - The session object where the deserialized data will be stored.
//
// Returns:
//
//	An error if the deserialization fails, otherwise nil.
func (s GobSerializer) Deserialize(d []byte, ss *sessions.Session) error {
	dec := gob.NewDecoder(bytes.NewBuffer(d))
	return dec.Decode(&ss.Values)
}

// Option is a function type for configuring a RediStore.
type Option func(*storeConfig) error

// addressConfig holds network and address configuration for Redis connection.
type addressConfig struct {
	network string
	address string
}

// storeConfig is an internal configuration structure used during RediStore initialization.
// It collects all configuration parameters before creating the final RediStore instance.
type storeConfig struct {
	// Connection configuration (exactly one must be set)
	pool    *redis.Pool
	address *addressConfig
	url     string

	// Authentication
	username string
	password string

	// Redis configuration
	db          string
	poolSize    int
	idleTimeout time.Duration

	// Store configuration
	maxLength     int
	keyPrefix     string
	defaultMaxAge int
	serializer    SessionSerializer
	sessionOpts   *sessions.Options
}

// RediStore represents a session store backed by a Redis database.
// It provides methods to manage session data using Redis as the storage backend.
//
// Fields:
//
//	Pool: A connection pool for Redis.
//	Codecs: A list of securecookie.Codec used to encode and decode session data.
//	Options: Default configuration options for sessions.
//	DefaultMaxAge: Default TTL (Time To Live) for sessions with MaxAge == 0.
//	maxLength: Maximum length of session data.
//	keyPrefix: Prefix to be added to all Redis keys used by this store.
//	serializer: Serializer used to encode and decode session data.
type RediStore struct {
	Pool          *redis.Pool
	Codecs        []securecookie.Codec
	Options       *sessions.Options // default configuration
	DefaultMaxAge int               // default Redis TTL for a MaxAge == 0 session
	maxLength     int
	keyPrefix     string
	serializer    SessionSerializer
}

// WithPool configures the RediStore to use a custom Redis connection pool.
// This option is mutually exclusive with WithAddress and WithURL.
func WithPool(pool *redis.Pool) Option {
	return func(cfg *storeConfig) error {
		if pool == nil {
			return errors.New("pool cannot be nil")
		}
		cfg.pool = pool
		return nil
	}
}

// WithAddress configures the RediStore to connect to Redis using network and address.
// This option is mutually exclusive with WithPool and WithURL.
//
// Example:
//
//	WithAddress("tcp", "localhost:6379")
//	WithAddress("unix", "/tmp/redis.sock")
func WithAddress(network, address string) Option {
	return func(cfg *storeConfig) error {
		if network == "" || address == "" {
			return errors.New("network and address cannot be empty")
		}
		cfg.address = &addressConfig{
			network: network,
			address: address,
		}
		return nil
	}
}

// WithURL configures the RediStore to connect to Redis using a URL.
// This option is mutually exclusive with WithPool and WithAddress.
//
// Example:
//
//	WithURL("redis://localhost:6379/0")
//	WithURL("redis://:password@localhost:6379/1")
func WithURL(url string) Option {
	return func(cfg *storeConfig) error {
		if url == "" {
			return errors.New("url cannot be empty")
		}
		cfg.url = url
		return nil
	}
}

// WithAuth sets the username and password for Redis authentication.
// Both username and password can be empty strings if not required.
func WithAuth(username, password string) Option {
	return func(cfg *storeConfig) error {
		cfg.username = username
		cfg.password = password
		return nil
	}
}

// WithPassword sets only the password for Redis authentication.
// This is a convenience function for Redis instances that don't use username.
func WithPassword(password string) Option {
	return func(cfg *storeConfig) error {
		cfg.password = password
		return nil
	}
}

// WithDB sets the Redis database index to use.
// The db parameter should be a string representation of a number between 0 and 15.
// If empty, defaults to "0".
func WithDB(db string) Option {
	return func(cfg *storeConfig) error {
		if db == "" {
			db = "0"
		}
		dbNum, err := strconv.Atoi(db)
		if err != nil {
			return fmt.Errorf("invalid database number %q: %w", db, err)
		}
		if dbNum < 0 || dbNum > 15 {
			return fmt.Errorf("database number must be between 0 and 15, got %d", dbNum)
		}
		cfg.db = db
		return nil
	}
}

// WithDBNum sets the Redis database index using an integer.
// This is a convenience function equivalent to WithDB(strconv.Itoa(dbNum)).
func WithDBNum(dbNum int) Option {
	return func(cfg *storeConfig) error {
		if dbNum < 0 || dbNum > 15 {
			return fmt.Errorf("database number must be between 0 and 15, got %d", dbNum)
		}
		cfg.db = strconv.Itoa(dbNum)
		return nil
	}
}

// WithPoolSize sets the maximum number of idle connections in the pool.
// Default is 10. Only applies when using WithAddress or WithURL.
func WithPoolSize(size int) Option {
	return func(cfg *storeConfig) error {
		if size <= 0 {
			return fmt.Errorf("pool size must be positive, got %d", size)
		}
		cfg.poolSize = size
		return nil
	}
}

// WithIdleTimeout sets the idle timeout for connections in the pool.
// Default is 240 seconds. Only applies when using WithAddress or WithURL.
func WithIdleTimeout(timeout time.Duration) Option {
	return func(cfg *storeConfig) error {
		if timeout < 0 {
			return fmt.Errorf("idle timeout cannot be negative")
		}
		cfg.idleTimeout = timeout
		return nil
	}
}

// WithMaxLength sets the maximum size of session data in bytes.
// Default is 4096 bytes. Set to 0 for no limit (use with caution).
// Redis allows values up to 512MB.
func WithMaxLength(length int) Option {
	return func(cfg *storeConfig) error {
		if length < 0 {
			return fmt.Errorf("max length cannot be negative")
		}
		cfg.maxLength = length
		return nil
	}
}

// WithKeyPrefix sets the prefix for all Redis keys used by this store.
// Default is "session_". This is useful to avoid key collisions when using
// a single Redis instance for multiple applications.
func WithKeyPrefix(prefix string) Option {
	return func(cfg *storeConfig) error {
		cfg.keyPrefix = prefix
		return nil
	}
}

// WithDefaultMaxAge sets the default TTL (time-to-live) in seconds for sessions.
// This is used when session.Options.MaxAge is 0.
// Default is 1200 seconds (20 minutes).
func WithDefaultMaxAge(age int) Option {
	return func(cfg *storeConfig) error {
		if age < 0 {
			return fmt.Errorf("default max age cannot be negative")
		}
		cfg.defaultMaxAge = age
		return nil
	}
}

// WithSerializer sets the session serializer.
// Default is GobSerializer. You can also use JSONSerializer or implement
// your own SessionSerializer.
func WithSerializer(serializer SessionSerializer) Option {
	return func(cfg *storeConfig) error {
		if serializer == nil {
			return errors.New("serializer cannot be nil")
		}
		cfg.serializer = serializer
		return nil
	}
}

// WithSessionOptions sets the default session options.
// This allows fine-grained control over cookie behavior.
func WithSessionOptions(opts *sessions.Options) Option {
	return func(cfg *storeConfig) error {
		if opts == nil {
			return errors.New("session options cannot be nil")
		}
		// Copy user-provided options into an internal instance to avoid
		// aliasing and unintended mutations when other options are applied.
		if cfg.sessionOpts == nil {
			cfg.sessionOpts = &sessions.Options{}
		}
		*cfg.sessionOpts = *opts
		return nil
	}
}

// WithPath sets the cookie path for sessions.
// Default is "/". This is a convenience function that modifies session options.
func WithPath(path string) Option {
	return func(cfg *storeConfig) error {
		if cfg.sessionOpts == nil {
			cfg.sessionOpts = &sessions.Options{}
		}
		cfg.sessionOpts.Path = path
		return nil
	}
}

// WithMaxAge sets the MaxAge for session cookies in seconds.
// Default is sessionExpire (86400 * 30 = 30 days).
// This is a convenience function that modifies session options.
func WithMaxAge(age int) Option {
	return func(cfg *storeConfig) error {
		if cfg.sessionOpts == nil {
			cfg.sessionOpts = &sessions.Options{}
		}
		cfg.sessionOpts.MaxAge = age
		return nil
	}
}

// defaultConfig returns a storeConfig with default values.
func defaultConfig() *storeConfig {
	return &storeConfig{
		db:            "0",
		poolSize:      10,
		idleTimeout:   240 * time.Second,
		maxLength:     4096,
		keyPrefix:     "session_",
		defaultMaxAge: 60 * 20, // 20 minutes
		serializer:    GobSerializer{},
		sessionOpts: &sessions.Options{
			Path:   "/",
			MaxAge: sessionExpire,
		},
	}
}

// validate checks that the configuration is valid.
// It ensures that exactly one connection option is specified and all values are sensible.
func (cfg *storeConfig) validate() error {
	// Count connection options
	connectionOptions := 0
	if cfg.pool != nil {
		connectionOptions++
	}
	if cfg.address != nil {
		connectionOptions++
	}
	if cfg.url != "" {
		connectionOptions++
	}

	if connectionOptions == 0 {
		return errors.New(
			"exactly one connection option is required: use WithPool, WithAddress, or WithURL",
		)
	}
	if connectionOptions > 1 {
		return errors.New(
			"only one connection option can be specified: " +
				"WithPool, WithAddress, or WithURL are mutually exclusive",
		)
	}

	return nil
}

// buildPool creates a Redis connection pool based on the configuration.
// Returns the existing pool if cfg.pool is set, otherwise creates a new one.
func (cfg *storeConfig) buildPool() (*redis.Pool, error) {
	// If pool is already provided, use it
	if cfg.pool != nil {
		return cfg.pool, nil
	}

	// Create dial function based on address or URL
	var dialFunc func() (redis.Conn, error)

	switch {
	case cfg.address != nil:
		// Use address-based connection
		dialFunc = func() (redis.Conn, error) {
			return dialClient(
				cfg.address.network,
				cfg.address.address,
				cfg.username,
				cfg.password,
				cfg.db,
			)
		}
	case cfg.url != "":
		// Use URL-based connection
		dialFunc = func() (redis.Conn, error) {
			return redis.DialURL(cfg.url)
		}
	default:
		return nil, errors.New("no connection method specified")
	}

	// Create the pool
	pool := &redis.Pool{
		MaxIdle:     cfg.poolSize,
		IdleTimeout: cfg.idleTimeout,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
		Dial: dialFunc,
	}

	return pool, nil
}

// Keys creates a key pairs slice from individual byte slices.
// This is a convenience function to simplify the creation of key pairs
// without having to write [][]byte{...}.
//
// Example:
//
//	store, err := NewStore(
//	    Keys([]byte("auth-key"), []byte("encrypt-key")),
//	    WithAddress("tcp", ":6379"),
//	)
func Keys(keys ...[]byte) [][]byte {
	return keys
}

// KeysFromStrings creates a key pairs slice from strings.
// This is the most convenient way to provide keys for development and testing.
//
// Warning: For production use with sensitive keys, consider using Keys() with
// byte slices loaded from secure storage instead of hardcoded strings.
//
// Example:
//
//	// Single key
//	store, err := NewStore(
//	    KeysFromStrings("secret-key"),
//	    WithAddress("tcp", ":6379"),
//	)
//
//	// Multiple keys for rotation
//	store, err := NewStore(
//	    KeysFromStrings(
//	        "new-auth-key",
//	        "new-encrypt-key",
//	        "old-auth-key",
//	        "old-encrypt-key",
//	    ),
//	    WithAddress("tcp", ":6379"),
//	)
func KeysFromStrings(keys ...string) [][]byte {
	result := make([][]byte, len(keys))
	for i, k := range keys {
		result[i] = []byte(k)
	}
	return result
}

// NewStore creates a new RediStore with the given options.
//
// Parameters:
//
//	keyPairs - One or more key pairs for cookie encryption and authentication.
//	           Each key should be 16, 24, or 32 bytes for AES-128, AES-192, or AES-256.
//	           Keys are used in pairs: authentication key and encryption key.
//	           Provide multiple pairs for key rotation (first pair is used for encoding,
//	           remaining pairs are used for decoding only).
//	opts - Configuration options. At least one connection option is required.
//
// Connection Options (required, exactly one):
//   - WithPool(pool) - Use a custom Redis connection pool
//   - WithAddress(network, address) - Connect using network protocol and address
//   - WithURL(url) - Connect using a Redis URL
//
// Authentication Options:
//   - WithAuth(username, password) - Set username and password
//   - WithPassword(password) - Set password only
//
// Redis Configuration Options:
//   - WithDB(db) - Set database index (default "0")
//   - WithDBNum(n) - Set database index as integer
//   - WithPoolSize(size) - Set connection pool size (default 10)
//   - WithIdleTimeout(timeout) - Set idle timeout (default 240s)
//
// Store Configuration Options:
//   - WithMaxLength(length) - Set max session size (default 4096)
//   - WithKeyPrefix(prefix) - Set Redis key prefix (default "session_")
//   - WithDefaultMaxAge(age) - Set default TTL (default 1200)
//   - WithSerializer(s) - Set serializer (default GobSerializer)
//   - WithSessionOptions(opts) - Set session options
//   - WithPath(path) - Set cookie path (default "/")
//   - WithMaxAge(age) - Set cookie MaxAge (default 30 days)
//
// Example:
//
//	// Basic usage with single key (using helper function)
//	store, err := NewStore(
//	    KeysFromStrings("secret-key"),
//	    WithAddress("tcp", ":6379"),
//	)
//
//	// Using Keys() with byte slices
//	store, err := NewStore(
//	    Keys(
//	        []byte("authentication-key"), // 32 or 64 bytes
//	        []byte("encryption-key"),     // 16, 24, or 32 bytes
//	    ),
//	    WithAddress("tcp", ":6379"),
//	)
//
//	// With key rotation (old keys for decoding only)
//	store, err := NewStore(
//	    KeysFromStrings(
//	        "new-auth-key",
//	        "new-encrypt-key",
//	        "old-auth-key",    // For decoding existing sessions
//	        "old-encrypt-key",
//	    ),
//	    WithAddress("tcp", "localhost:6379"),
//	    WithDB("1"),
//	)
//
//	// With multiple options
//	store, err := NewStore(
//	    KeysFromStrings("secret-key"),
//	    WithAddress("tcp", "localhost:6379"),
//	    WithDB("1"),
//	    WithMaxLength(8192),
//	    WithKeyPrefix("myapp_"),
//	    WithSerializer(JSONSerializer{}),
//	)
//
//	// Using URL
//	store, err := NewStore(
//	    KeysFromStrings("secret-key"),
//	    WithURL("redis://:password@localhost:6379/0"),
//	)
//
//	// Without helper functions (direct slice)
//	store, err := NewStore(
//	    [][]byte{[]byte("secret-key")},
//	    WithAddress("tcp", ":6379"),
//	)
func NewStore(keyPairs [][]byte, opts ...Option) (*RediStore, error) {
	// Validate key pairs
	if len(keyPairs) == 0 {
		return nil, errors.New("at least one key pair is required")
	}

	// Start with default configuration
	cfg := defaultConfig()

	// Apply all options
	for i, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply option %d: %w", i, err)
		}
	}

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Build connection pool
	pool, err := cfg.buildPool()
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Create RediStore instance
	rs := &RediStore{
		Pool:          pool,
		Codecs:        securecookie.CodecsFromPairs(keyPairs...),
		Options:       cfg.sessionOpts,
		DefaultMaxAge: cfg.defaultMaxAge,
		maxLength:     cfg.maxLength,
		keyPrefix:     cfg.keyPrefix,
		serializer:    cfg.serializer,
	}

	// Test connection
	if _, err := rs.ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return rs, nil
}

// SetMaxLength sets RediStore.maxLength if the `l` argument is greater or equal 0
// maxLength restricts the maximum length of new sessions to l.
// If l is 0 there is no limit to the size of a session, use with caution.
// The default for a new RediStore is 4096. Redis allows for max.
// value sizes of up to 512MB (http://redis.io/topics/data-types)
// Default: 4096,
func (s *RediStore) SetMaxLength(l int) {
	if l >= 0 {
		s.maxLength = l
	}
}

// SetKeyPrefix sets the key prefix for all keys used in the RediStore.
// This is useful to avoid key name collisions when using a single Redis
// instance for multiple applications.
func (s *RediStore) SetKeyPrefix(p string) {
	s.keyPrefix = p
}

// SetSerializer sets the session serializer for the RediStore.
// The serializer is responsible for encoding and decoding session data.
//
// Parameters:
//
//	ss - The session serializer to be used.
func (s *RediStore) SetSerializer(ss SessionSerializer) {
	s.serializer = ss
}

// SetMaxAge restricts the maximum age, in seconds, of the session record
// both in database and a browser. This is to change session storage configuration.
// If you want just to remove session use your session `s` object and change it's
// `Options.MaxAge` to -1, as specified in
//
//	http://godoc.org/github.com/gorilla/sessions#Options
//
// Default is the one provided by this package value - `sessionExpire`.
// Set it to 0 for no restriction.
// Because we use `MaxAge` also in SecureCookie crypting algorithm you should
// use this function to change `MaxAge` value.
func (s *RediStore) SetMaxAge(v int) {
	var c *securecookie.SecureCookie
	var ok bool
	s.Options.MaxAge = v
	for i := range s.Codecs {
		if c, ok = s.Codecs[i].(*securecookie.SecureCookie); ok {
			c.MaxAge(v)
		} else {
			fmt.Printf("Can't change MaxAge on codec %v\n", s.Codecs[i])
		}
	}
}

func dialClient(network, address, username, password, db string) (redis.Conn, error) {
	// check db and convert to int
	if db == "" {
		db = "0"
	}

	// convert db to int
	dbNum, err := strconv.Atoi(db)
	if err != nil {
		return nil, err
	}

	// If there is no password, the redis. DialPassword
	if password == "" {
		// Only the database index is passed.
		return redis.Dial(
			network,
			address,
			redis.DialUsername(username),
			redis.DialDatabase(dbNum),
		)
	}

	return redis.Dial(
		network,
		address,
		redis.DialUsername(username),
		redis.DialPassword(password),
		redis.DialDatabase(dbNum),
	)
}

// Close closes the underlying *redis.Pool
func (s *RediStore) Close() error {
	return s.Pool.Close()
}

// Get returns a session for the given name after adding it to the registry.
//
// See gorilla/sessions FilesystemStore.Get().
func (s *RediStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(s, name)
}

// New returns a session for the given name without adding it to the registry.
//
// See gorilla/sessions FilesystemStore.New().
func (s *RediStore) New(r *http.Request, name string) (*sessions.Session, error) {
	var (
		err error
		ok  bool
	)
	session := sessions.NewSession(s, name)
	// make a copy
	options := *s.Options
	session.Options = &options
	session.IsNew = true
	if c, errCookie := r.Cookie(name); errCookie == nil {
		err = securecookie.DecodeMulti(name, c.Value, &session.ID, s.Codecs...)
		if err == nil {
			ok, err = s.load(session)
			session.IsNew = err != nil || !ok // not new if no error and data available
		}
	}
	return session, err
}

// Save adds a single session to the response.
func (s *RediStore) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	// Marked for deletion.
	if session.Options.MaxAge <= 0 {
		if err := s.delete(session); err != nil {
			return err
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), "", session.Options))
	} else {
		// Build an alphanumeric key for the redis store.
		if session.ID == "" {
			session.ID = strings.TrimRight(base32.StdEncoding.EncodeToString(securecookie.GenerateRandomKey(32)), "=")
		}
		if err := s.save(session); err != nil {
			return err
		}
		encoded, err := securecookie.EncodeMulti(session.Name(), session.ID, s.Codecs...)
		if err != nil {
			return err
		}
		http.SetCookie(w, sessions.NewCookie(session.Name(), encoded, session.Options))
	}
	return nil
}

// Delete removes the session from redis, and sets the cookie to expire.
//
// WARNING: This method should be considered deprecated since it is not exposed via the gorilla/sessions interface.
// Set session.Options.MaxAge = -1 and call Save instead. - July 18th, 2013
func (s *RediStore) Delete(
	r *http.Request,
	w http.ResponseWriter,
	session *sessions.Session,
) error {
	conn := s.Pool.Get()
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("Error closing connection: %v\n", err)
		}
	}()
	if _, err := conn.Do("DEL", s.keyPrefix+session.ID); err != nil {
		return err
	}
	// Set cookie to expire.
	options := *session.Options
	options.MaxAge = -1
	http.SetCookie(w, sessions.NewCookie(session.Name(), "", &options))
	// Clear session values.
	for k := range session.Values {
		delete(session.Values, k)
	}
	return nil
}

// ping does an internal ping against a server to check if it is alive.
func (s *RediStore) ping() (bool, error) {
	conn := s.Pool.Get()
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("Error closing connection: %v\n", err)
		}
	}()
	data, err := conn.Do("PING")
	if err != nil || data == nil {
		return false, err
	}
	return (data == "PONG"), nil
}

// save stores the session in redis.
func (s *RediStore) save(session *sessions.Session) error {
	b, err := s.serializer.Serialize(session)
	if err != nil {
		return err
	}
	if s.maxLength != 0 && len(b) > s.maxLength {
		return errors.New("SessionStore: the value to store is too big")
	}
	conn := s.Pool.Get()
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("Error closing connection: %v\n", err)
		}
	}()
	if err = conn.Err(); err != nil {
		return err
	}
	age := session.Options.MaxAge
	if age == 0 {
		age = s.DefaultMaxAge
	}
	_, err = conn.Do("SETEX", s.keyPrefix+session.ID, age, b)
	return err
}

// load reads the session from redis.
// returns true if there is a sessoin data in DB
func (s *RediStore) load(session *sessions.Session) (bool, error) {
	conn := s.Pool.Get()
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("Error closing connection: %v\n", err)
		}
	}()
	if err := conn.Err(); err != nil {
		return false, err
	}
	data, err := conn.Do("GET", s.keyPrefix+session.ID)
	if err != nil {
		return false, err
	}
	if data == nil {
		return false, nil // no data was associated with this key
	}
	b, err := redis.Bytes(data, err)
	if err != nil {
		return false, err
	}
	return true, s.serializer.Deserialize(b, session)
}

// delete removes keys from redis if MaxAge<0
func (s *RediStore) delete(session *sessions.Session) error {
	conn := s.Pool.Get()
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("Error closing connection: %v\n", err)
		}
	}()
	if _, err := conn.Do("DEL", s.keyPrefix+session.ID); err != nil {
		return err
	}
	return nil
}
