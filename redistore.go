// Copyright 2012 Brian "bojo" Jones. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package redistore

import (
	"encoding/base32"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"net/http"
	"strings"
	"time"
)

// Amount of time for cookies/redis keys to expire.
var sessionExpire int = 86400 * 30

// RediStore stores sessions in a redis backend.
type RediStore struct {
	Pool    *redis.Pool
	Codecs  []securecookie.Codec
	Options *sessions.Options // default configuration
}

// NewRediStore returns a new RediStore.
func NewRediStore(size int, network, address, password string, keyPairs ...[]byte) *RediStore {
	return &RediStore{
		// http://godoc.org/github.com/garyburd/redigo/redis#Pool
		Pool: &redis.Pool{
			MaxIdle:     size,
			IdleTimeout: 240 * time.Second,
			Dial: func() (redis.Conn, error) {
				c, err := redis.Dial(network, address)
				if err != nil {
					return nil, err
				}
				if password != "" {
					if _, err := c.Do("AUTH", password); err != nil {
						c.Close()
						return nil, err
					}
				}
				return c, err
			},
			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				_, err := c.Do("PING")
				return err
			},
		},
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   "/",
			MaxAge: sessionExpire,
		},
	}
}

// Close cleans up the redis connections.
func (s *RediStore) Close() {
	s.Pool.Close()
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
	var err error
	session := sessions.NewSession(s, name)
	session.Options = &(*s.Options)
	session.IsNew = true
	if c, errCookie := r.Cookie(name); errCookie == nil {
		err = securecookie.DecodeMulti(name, c.Value, &session.ID, s.Codecs...)
		if err == nil {
			err = s.load(session)
			if err == nil {
				session.IsNew = false
			}
		}
	}
	return session, err
}

// Save adds a single session to the response.
func (s *RediStore) Save(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
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
	return nil
}

// Delete removes the session from redis, and sets the cookie to expire.
func (s *RediStore) Delete(r *http.Request, w http.ResponseWriter, session *sessions.Session) error {
	conn := s.Pool.Get()
	defer conn.Close()
	if _, err := conn.Do("DEL", "session_"+session.ID); err != nil {
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

// save stores the session in redis.
func (s *RediStore) save(session *sessions.Session) error {
	encoded, err := securecookie.EncodeMulti(session.Name(), session.Values, s.Codecs...)
	if err != nil {
		return err
	}
	conn := s.Pool.Get()
	defer conn.Close()
	conn.Send("SET", "session_"+session.ID, encoded)
	conn.Send("EXPIRE", "session_"+session.ID, sessionExpire)
	conn.Flush()
	if _, err := conn.Receive(); err != nil { // SET
		return err
	}
	if _, err := conn.Receive(); err != nil { // EXPIRE
		return err
	}
	return nil
}

// load reads the session from redis.
func (s *RediStore) load(session *sessions.Session) error {
	conn := s.Pool.Get()
	defer conn.Close()
	if err := conn.Err(); err != nil {
		return err
	}
	data, err := conn.Do("GET", "session_"+session.ID)
	if err != nil {
		return err
	}
	if data == nil {
		return nil // no data was associated with this key
	}
	str, err := redis.String(data, err)
	if err != nil {
		return err
	}
	if err = securecookie.DecodeMulti(session.Name(), str, &session.Values, s.Codecs...); err != nil {
		return err
	}
	return nil
}
