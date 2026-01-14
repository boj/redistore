# redistore

[![codecov](https://codecov.io/gh/boj/redistore/branch/master/graph/badge.svg)](https://codecov.io/gh/boj/redistore)
[![Go Report Card](https://goreportcard.com/badge/github.com/boj/redistore)](https://goreportcard.com/report/github.com/boj/redistore)
[![GoDoc](https://godoc.org/github.com/boj/redistore?status.svg)](https://godoc.org/github.com/boj/redistore)
[![Run Tests](https://github.com/boj/redistore/actions/workflows/go.yml/badge.svg)](https://github.com/boj/redistore/actions/workflows/go.yml)
[![Trivy Security Scan](https://github.com/boj/redistore/actions/workflows/security.yml/badge.svg)](https://github.com/boj/redistore/actions/workflows/security.yml)

A session store backend for [gorilla/sessions](http://www.gorillatoolkit.org/pkg/sessions) with Redis as the storage engine.

## Features

✨ **Clean API** - Single entry point with flexible option pattern
🔧 **Highly Configurable** - 15+ options for fine-grained control
🔒 **Secure** - Built on gorilla/sessions with secure cookie encoding
⚡ **Fast** - Redis-backed for high performance
📦 **Serialization** - Support for Gob and JSON serializers
🧪 **Well Tested** - Comprehensive test coverage

## Requirements

- **Go**: 1.23 or higher
- **Redis**: 6.x or 7.x
- **Dependencies**:
  - [Redigo](https://github.com/gomodule/redigo) - Redis client
  - [gorilla/sessions](https://github.com/gorilla/sessions) - Session management
  - [gorilla/securecookie](https://github.com/gorilla/securecookie) - Secure cookies

## Installation

### For v2 (Recommended)

```sh
go get github.com/boj/redistore/v2
```

### For v1 (Legacy)

```sh
go get github.com/boj/redistore@v1
```

> **Note:** v2 introduces a cleaner API with the Option Pattern. See [MIGRATION.md](MIGRATION.md) for upgrade instructions.

## Quick Start

```go
package main

import (
    "log"
    "net/http"

    "github.com/boj/redistore/v2"
    "github.com/gorilla/sessions"
)

func main() {
    // Create a new store with options
    store, err := redistore.NewStore(
        redistore.KeysFromStrings("secret-key"),
        redistore.WithAddress("tcp", ":6379"),
    )
    if err != nil {
        panic(err)
    }
    defer store.Close()

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Get a session
        session, err := store.Get(r, "session-key")
        if err != nil {
            log.Println(err.Error())
            return
        }

        // Set a value
        session.Values["foo"] = "bar"

        // Save session
        if err = sessions.Save(r, w); err != nil {
            log.Fatalf("Error saving session: %v", err)
        }
    })

    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## Documentation

- **API Reference**: [pkg.go.dev/github.com/boj/redistore/v2](https://pkg.go.dev/github.com/boj/redistore/v2)
- **Migration Guide**: [MIGRATION.md](MIGRATION.md)
- **Changelog**: [CHANGELOG.md](CHANGELOG.md)
- **gorilla/sessions**: [Documentation](http://www.gorillatoolkit.org/pkg/sessions)

## Usage Examples

### Basic Connection

```go
store, err := redistore.NewStore(
    redistore.KeysFromStrings("secret-key"),
    redistore.WithAddress("tcp", "localhost:6379"),
)
```

### With Authentication

```go
store, err := redistore.NewStore(
    redistore.KeysFromStrings("secret-key"),
    redistore.WithAddress("tcp", "localhost:6379"),
    redistore.WithAuth("username", "password"),
)
```

### Specific Database

```go
store, err := redistore.NewStore(
    redistore.KeysFromStrings("secret-key"),
    redistore.WithAddress("tcp", "localhost:6379"),
    redistore.WithDB("5"), // Use database 5
)
```

### Using Redis URL

```go
store, err := redistore.NewStore(
    redistore.KeysFromStrings("secret-key"),
    redistore.WithURL("redis://:password@localhost:6379/0"),
)
```

### Custom Configuration

```go
store, err := redistore.NewStore(
    redistore.KeysFromStrings("secret-key"),
    redistore.WithAddress("tcp", "localhost:6379"),
    redistore.WithMaxLength(8192),          // Max session size: 8KB
    redistore.WithKeyPrefix("myapp_"),      // Key prefix
    redistore.WithDefaultMaxAge(3600),      // Default TTL: 1 hour
    redistore.WithSerializer(redistore.JSONSerializer{}), // JSON serializer
)
```

### Using a Custom Pool

```go
import "github.com/gomodule/redigo/redis"

pool := &redis.Pool{
    MaxIdle:     100,
    IdleTimeout: 5 * time.Minute,
    Dial: func() (redis.Conn, error) {
        return redis.Dial("tcp", "localhost:6379")
    },
}

store, err := redistore.NewStore(
    redistore.KeysFromStrings("secret-key"),
    redistore.WithPool(pool),
)
```

### Key Rotation

Support for encryption key rotation allows you to change keys without invalidating existing sessions:

```go
// Keys are provided in pairs: authentication key, encryption key
// The first pair is used for encoding new sessions
// All pairs are tried for decoding existing sessions
store, err := redistore.NewStore(
    redistore.KeysFromStrings(
        "new-authentication-key", // 32 or 64 bytes recommended
        "new-encryption-key",     // 16, 24, or 32 bytes for AES
        "old-authentication-key", // Keep for existing sessions
        "old-encryption-key",     // Keep for existing sessions
    ),
    redistore.WithAddress("tcp", "localhost:6379"),
)

// Using Keys() with byte slices for production
authKey, _ := loadKeyFromSecureStorage("auth-key")
encryptKey, _ := loadKeyFromSecureStorage("encrypt-key")
store, err := redistore.NewStore(
    redistore.Keys(authKey, encryptKey),
    redistore.WithAddress("tcp", "localhost:6379"),
)
```

**Key Sizes:**

- Authentication key: 32 or 64 bytes (HMAC)
- Encryption key: 16 (AES-128), 24 (AES-192), or 32 bytes (AES-256)

**Rotation Process:**

1. Add new key pair at the beginning
2. Keep old keys for a transition period
3. Remove old keys once all sessions have been renewed

**Helper Functions:**

- `KeysFromStrings(keys ...string)` - Simplest way to provide keys from strings
- `Keys(keys ...[]byte)` - For keys already as byte slices
- Direct slice: `[][]byte{[]byte("key")}` - Original syntax still supported

### Complete Example

```go
package main

import (
    "log"
    "net/http"

    "github.com/boj/redistore/v2"
    "github.com/gorilla/sessions"
)

func main() {
    // Initialize store with custom configuration
    store, err := redistore.NewStore(
        redistore.KeysFromStrings("secret-key-123"),
        redistore.WithAddress("tcp", "localhost:6379"),
        redistore.WithDB("1"),
        redistore.WithMaxLength(8192),
        redistore.WithKeyPrefix("webapp_"),
        redistore.WithDefaultMaxAge(3600), // 1 hour
    )
    if err != nil {
        log.Fatal(err)
    }
    defer store.Close()

    http.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
        session, _ := store.Get(r, "my-session")
        session.Values["user"] = "john_doe"
        session.Values["authenticated"] = true
        sessions.Save(r, w)
        w.Write([]byte("Session saved!"))
    })

    http.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
        session, _ := store.Get(r, "my-session")
        user := session.Values["user"]
        if user != nil {
            w.Write([]byte("User: " + user.(string)))
        } else {
            w.Write([]byte("No user in session"))
        }
    })

    http.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request) {
        session, _ := store.Get(r, "my-session")
        session.Options.MaxAge = -1
        sessions.Save(r, w)
        w.Write([]byte("Session deleted!"))
    })

    log.Println("Server started on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

## Configuration Options

### Connection Options (Required - Choose ONE)

| Option                          | Description                                              |
| ------------------------------- | -------------------------------------------------------- |
| `WithPool(pool)`                | Use a custom Redis connection pool                       |
| `WithAddress(network, address)` | Connect via network and address (e.g., "tcp", ":6379")   |
| `WithURL(url)`                  | Connect via Redis URL (e.g., "redis://localhost:6379/0") |

### Authentication Options

| Option                         | Description               |
| ------------------------------ | ------------------------- |
| `WithAuth(username, password)` | Set username and password |
| `WithPassword(password)`       | Set password only         |

### Redis Configuration

| Option                     | Default | Description               |
| -------------------------- | ------- | ------------------------- |
| `WithDB(db)`               | "0"     | Database index ("0"-"15") |
| `WithDBNum(dbNum)`         | 0       | Database index as integer |
| `WithPoolSize(size)`       | 10      | Connection pool size      |
| `WithIdleTimeout(timeout)` | 240s    | Connection idle timeout   |

### Store Configuration

| Option                     | Default       | Description                               |
| -------------------------- | ------------- | ----------------------------------------- |
| `WithMaxLength(length)`    | 4096          | Max session size in bytes (0 = unlimited) |
| `WithKeyPrefix(prefix)`    | "session\_"   | Redis key prefix                          |
| `WithDefaultMaxAge(age)`   | 1200          | Default TTL in seconds (20 minutes)       |
| `WithSerializer(s)`        | GobSerializer | Session serializer                        |
| `WithSessionOptions(opts)` | -             | Full gorilla/sessions options             |
| `WithPath(path)`           | "/"           | Cookie path                               |
| `WithMaxAge(age)`          | 30 days       | Cookie MaxAge                             |

## Serializers

### Gob Serializer (Default)

Uses Go's `encoding/gob` package. Efficient binary format, suitable for complex Go types.

```go
store, err := redistore.NewStore(
    [][]byte{[]byte("secret-key")},
    redistore.WithAddress("tcp", ":6379"),
    // GobSerializer is the default, no need to specify
)
```

### JSON Serializer

Uses `encoding/json` package. Human-readable, cross-language compatible.

```go
store, err := redistore.NewStore(
    [][]byte{[]byte("secret-key")},
    redistore.WithAddress("tcp", ":6379"),
    redistore.WithSerializer(redistore.JSONSerializer{}),
)
```

### Custom Serializer

Implement the `SessionSerializer` interface:

```go
type SessionSerializer interface {
    Serialize(ss *sessions.Session) ([]byte, error)
    Deserialize(d []byte, ss *sessions.Session) error
}

type MySerializer struct{}

func (s MySerializer) Serialize(ss *sessions.Session) ([]byte, error) {
    // Your implementation
}

func (s MySerializer) Deserialize(d []byte, ss *sessions.Session) error {
    // Your implementation
}

// Use it
store, err := redistore.NewStore(
    [][]byte{[]byte("secret-key")},
    redistore.WithAddress("tcp", ":6379"),
    redistore.WithSerializer(MySerializer{}),
)
```

## Session Management

### Setting Values

```go
session, _ := store.Get(r, "session-key")
session.Values["username"] = "john"
session.Values["role"] = "admin"
sessions.Save(r, w)
```

### Getting Values

```go
session, _ := store.Get(r, "session-key")
username := session.Values["username"]
if username != nil {
    fmt.Println(username.(string))
}
```

### Flash Messages

```go
// Add flash message
session.AddFlash("Welcome back!")
sessions.Save(r, w)

// Retrieve and clear flash messages
flashes := session.Flashes()
for _, flash := range flashes {
    fmt.Println(flash)
}
sessions.Save(r, w) // Save to clear flashes
```

### Deleting Sessions

```go
session, _ := store.Get(r, "session-key")
session.Options.MaxAge = -1
sessions.Save(r, w)
```

## Post-Initialization Configuration

While the Option Pattern is recommended, you can still modify settings after creation:

```go
store, err := redistore.NewStore(
    [][]byte{[]byte("secret-key")},
    redistore.WithAddress("tcp", ":6379"),
)

// Modify after creation
store.SetMaxLength(16384)
store.SetKeyPrefix("app2_")
store.SetSerializer(redistore.JSONSerializer{})
store.SetMaxAge(86400 * 7) // 7 days
```

## Error Handling

### Configuration Errors

```go
store, err := redistore.NewStore(
    [][]byte{[]byte("secret-key")},
    redistore.WithAddress("tcp", ":6379"),
    redistore.WithURL("redis://localhost"), // ❌ Error: multiple connection options
)
if err != nil {
    // Error: "only one connection option can be specified"
    log.Fatal(err)
}
```

### Connection Errors

```go
store, err := redistore.NewStore(
    [][]byte{[]byte("secret-key")},
    redistore.WithAddress("tcp", "invalid:9999"),
)
if err != nil {
    // Error: "failed to connect to Redis: ..."
    log.Fatal(err)
}
```

## Testing

Run the full test suite:

```bash
# Start Redis (required)
redis-server

# Run tests
go test -v

# With coverage
go test -v -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Performance

- **Session Retrieval**: ~1ms (local Redis)
- **Session Save**: ~1-2ms (local Redis)
- **Memory**: Minimal overhead, Redis handles storage
- **Concurrent Requests**: Scales with Redis and connection pool size

## Migration from v1

If you're using v1, please see [MIGRATION.md](MIGRATION.md) for detailed upgrade instructions.

**Quick comparison:**

```go
// v1
store, err := redistore.NewRediStore(10, "tcp", ":6379", "", "", []byte("key"))

// v2
store, err := redistore.NewStore(
    []byte("key"),
    redistore.WithAddress("tcp", ":6379"),
)
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new features
4. Ensure all tests pass
5. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Credits

- Original author: Brian "boj" Jones
- Based on [gorilla/sessions](https://github.com/gorilla/sessions)
- Redis client: [Redigo](https://github.com/gomodule/redigo)

## Support

- **Issues**: [GitHub Issues](https://github.com/boj/redistore/issues)
- **Discussions**: [GitHub Discussions](https://github.com/boj/redistore/discussions)
- **Documentation**: [pkg.go.dev](https://pkg.go.dev/github.com/boj/redistore/v2)
