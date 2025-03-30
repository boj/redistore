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

// NewRediStore creates a new RediStore with a connection pool to a Redis server.
// The size parameter specifies the maximum number of idle connections in the pool.
// The network and address parameters specify the network type and address of the Redis server.
// The username and password parameters are used for authentication with the Redis server.
// The keyPairs parameter is a variadic argument that allows passing multiple key pairs for cookie encryption.
// It returns a pointer to a RediStore and an error if the connection to the Redis server fails.
func NewRediStore(size int, network, address, username, password string, keyPairs ...[]byte) (*RediStore, error) {
	return NewRediStoreWithPool(&redis.Pool{
		MaxIdle:     size,
		IdleTimeout: 240 * time.Second,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
		Dial: func() (redis.Conn, error) {
			return dialClient(network, address, username, password, "")
		},
	}, keyPairs...)
}

func dialClient(network, address, username, password, DB string) (redis.Conn, error) {
	// check DB and convert to int
	if DB == "" {
		DB = "0"
	}

	// convert DB to int
	db, err := strconv.Atoi(DB)
	if err != nil {
		return nil, err
	}

	return redis.Dial(
		network,
		address,
		redis.DialUsername(username),
		redis.DialPassword(password),
		redis.DialDatabase(db),
	)
}

// NewRediStoreWithDB creates a new RediStore with a Redis connection pool.
// The pool is configured with the provided size, network, address, username, password, and database (DB).
// The keyPairs are used for cookie encryption.
//
// Parameters:
//   - size: The maximum number of idle connections in the pool.
//   - network: The network type (e.g., "tcp").
//   - address: The address of the Redis server.
//   - username: The username for Redis authentication.
//   - password: The password for Redis authentication.
//   - DB: The Redis database to be selected after connecting.
//   - keyPairs: Variadic parameter for cookie encryption keys.
//
// Returns:
//   - *RediStore: A pointer to the newly created RediStore.
//   - error: An error if the RediStore could not be created.
func NewRediStoreWithDB(size int, network, address, username, password, DB string, keyPairs ...[]byte) (*RediStore, error) {
	return NewRediStoreWithPool(&redis.Pool{
		MaxIdle:     size,
		IdleTimeout: 240 * time.Second,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
		Dial: func() (redis.Conn, error) {
			return dialClient(network, address, username, password, DB)
		},
	}, keyPairs...)
}

// NewRediStoreWithPool creates a new RediStore instance using the provided
// Redis connection pool and key pairs for secure cookie encoding.
//
// Parameters:
//   - pool: A Redis connection pool.
//   - keyPairs: Variadic parameter for secure cookie encoding key pairs.
//
// Returns:
//   - *RediStore: A pointer to the newly created RediStore instance.
//   - error: An error if the RediStore could not be created.
//
// The RediStore is configured with default options including a session path
// of "/", a default maximum age of 20 minutes, a maximum length of 4096 bytes,
// a key prefix of "session_", and a Gob serializer.
func NewRediStoreWithPool(pool *redis.Pool, keyPairs ...[]byte) (*RediStore, error) {
	rs := &RediStore{
		// http://godoc.org/github.com/gomodule/redigo/redis#Pool
		Pool:   pool,
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   "/",
			MaxAge: sessionExpire,
		},
		DefaultMaxAge: 60 * 20, // 20 minutes seems like a reasonable default
		maxLength:     4096,
		keyPrefix:     "session_",
		serializer:    GobSerializer{},
	}
	_, err := rs.ping()
	return rs, err
}

// NewRediStoreWithURL creates a new RediStore with a Redis connection pool configured
// using the provided URL. The pool has a maximum number of idle connections specified
// by the size parameter, and an idle timeout of 240 seconds. The function also accepts
// optional key pairs for secure cookie encoding.
//
// Parameters:
//   - size: The maximum number of idle connections in the pool.
//   - url: The Redis server URL.
//   - keyPairs: Optional variadic parameter for secure cookie encoding.
//
// Returns:
//   - *RediStore: A pointer to the newly created RediStore.
//   - error: An error if the connection to the Redis server fails.
func NewRediStoreWithURL(size int, url string, keyPairs ...[]byte) (*RediStore, error) {
	return NewRediStoreWithPool(&redis.Pool{
		MaxIdle:     size,
		IdleTimeout: 240 * time.Second,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
		Dial: func() (redis.Conn, error) {
			return redis.DialURL(url)
		},
	}, keyPairs...)
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
func (s *RediStore) Delete(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
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
