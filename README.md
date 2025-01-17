# redistore

[![codecov](https://codecov.io/gh/boj/redistore/branch/master/graph/badge.svg)](https://codecov.io/gh/boj/redistore)
[![Go Report Card](https://goreportcard.com/badge/github.com/boj/redistore)](https://goreportcard.com/report/github.com/boj/redistore)
[![GoDoc](https://godoc.org/github.com/boj/redistore?status.svg)](https://godoc.org/github.com/boj/redistore)
[![Run Tests](https://github.com/boj/redistore/actions/workflows/go.yml/badge.svg)](https://github.com/boj/redistore/actions/workflows/go.yml)

A session store backend for [gorilla/sessions](http://www.gorillatoolkit.org/pkg/sessions) - [src](https://github.com/gorilla/sessions).

## Requirements

Depends on the [Redigo](https://github.com/gomodule/redigo) Redis library.

## Installation

    go get github.com/boj/redistore

## Documentation

Available on [godoc.org](https://godoc.org/github.com/boj/redistore).

See the [repository](http://www.gorillatoolkit.org/pkg/sessions) for full documentation on underlying interface.

### Example

```go
// Fetch new store.
store, err := NewRediStore(10, "tcp", ":6379", "", []byte("secret-key"))
if err != nil {
  panic(err)
}
defer store.Close()

// Get a session.
session, err = store.Get(req, "session-key")
if err != nil {
  log.Error(err.Error())
}

// Add a value.
session.Values["foo"] = "bar"

// Save.
if err = sessions.Save(req, rsp); err != nil {
  t.Fatalf("Error saving session: %v", err)
}

// Delete session.
session.Options.MaxAge = -1
if err = sessions.Save(req, rsp); err != nil {
  t.Fatalf("Error saving session: %v", err)
}

// Change session storage configuration for MaxAge = 10 days.
store.SetMaxAge(10 * 24 * 3600)
```
