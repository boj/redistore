// Copyright 2012 Brian "bojo" Jones. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package redistore

import (
	"encoding/base32"
	"errors"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"net/http"
	"strings"
)

// RediStore stores sessions in a redis backend.
type RediStore struct {
	Conn    redis.Conn
	Codecs  []securecookie.Codec
	Options *sessions.Options // default configuration
}

// NewRediStore returns a new RediStore.
func NewRediStore(keyPairs ...[]byte) *RediStore {
	return &RediStore{
		Codecs: securecookie.CodecsFromPairs(keyPairs...),
		Options: &sessions.Options{
			Path:   "/",
			MaxAge: 86400 * 30,
		},
	}
}

// Dial connects to the redis database.
func (s *RediStore) Dial(network, address string) error {
	c, err := redis.Dial(network, address)
	if err != nil {
		return err
	}
	s.Conn = c
	return nil
}

// Close closes the redis connection.
func (s *RediStore) Close() {
	s.Conn.Close()
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
	session := sessions.NewSession(s, name)
	session.Options = &(*s.Options)
	session.IsNew = true
	var err error
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
	if session.ID == "" {
		session.ID = strings.TrimRight(base32.StdEncoding.EncodeToString(securecookie.GenerateRandomKey(32)), "=")
	}
	encoded, err := securecookie.EncodeMulti(session.Name(), session.Values, s.Codecs...)
	if err != nil {
		return err
	}
	if err := s.save(session, encoded); err != nil {
		return err
	}
	http.SetCookie(w, sessions.NewCookie(session.Name(), encoded, session.Options))
	return nil
}

// save stores the session in redis.
func (s *RediStore) save(session *sessions.Session, encoded string) error {
	s.Conn.Do("SET", "session_"+session.ID, encoded)
	data, err := redis.String(s.Conn.Do("GET", "session_"+session.ID))
	if err != nil {
		return err
	}
	if len(encoded) > 0 && data == "" {
		return errors.New("save: data was not stored")
	}
	return nil
}

// load reads the session from redis.
func (s *RediStore) load(session *sessions.Session) error {
	data, err := redis.String(s.Conn.Do("GET", "session_"+session.ID))
	if err != nil {
		return err
	}
	if err = securecookie.DecodeMulti(session.Name(), data, &session.Values, s.Codecs...); err != nil {
		return err
	}
	return nil
}
