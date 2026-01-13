# Migration Guide: v1 to v2

This guide will help you migrate from redistore v1 to v2, which introduces the Option Pattern for cleaner and more flexible initialization.

## Overview

Version 2.0.0 is a **breaking change** that removes the old initialization functions in favor of a single, flexible `NewStore()` function with options.

### What Changed

**Removed:**
- `NewRediStore(size, network, address, username, password, keyPairs)`
- `NewRediStoreWithDB(size, network, address, username, password, db, keyPairs)`
- `NewRediStoreWithPool(pool, keyPairs)`
- `NewRediStoreWithURL(size, url, keyPairs)`

**Added:**
- `NewStore(keyPairs, opts...)` - Unified initialization with option pattern
- 15+ configuration options (see [Options Reference](#options-reference))

## Quick Migration Table

| v1 API | v2 API |
|--------|--------|
| `NewRediStore(10, "tcp", ":6379", "", "", key)` | `NewStore(key, WithAddress("tcp", ":6379"))` |
| `NewRediStoreWithDB(10, "tcp", ":6379", "", "", "1", key)` | `NewStore(key, WithAddress("tcp", ":6379"), WithDB("1"))` |
| `NewRediStoreWithURL(10, url, key)` | `NewStore(key, WithURL(url))` |
| `NewRediStoreWithPool(pool, key)` | `NewStore(key, WithPool(pool))` |

## Step-by-Step Migration

### 1. Update Import

The import path changes to include `/v2`:

```go
// v1
import "github.com/boj/redistore"

// v2
import "github.com/boj/redistore/v2"
```

Update your `go.mod`:

```bash
go get github.com/boj/redistore/v2
```

### 2. Basic Connection

**v1:**
```go
store, err := redistore.NewRediStore(
    10,           // pool size
    "tcp",        // network
    ":6379",      // address
    "",           // username
    "",           // password
    []byte("secret-key"),
)
```

**v2:**
```go
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithAddress("tcp", ":6379"),
    // WithPoolSize is optional (defaults to 10)
)
```

### 3. Connection with Authentication

**v1:**
```go
store, err := redistore.NewRediStore(
    10,
    "tcp",
    ":6379",
    "myuser",     // username
    "mypass",     // password
    []byte("secret-key"),
)
```

**v2:**
```go
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithAddress("tcp", ":6379"),
    redistore.WithAuth("myuser", "mypass"),
)

// Or just password:
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithAddress("tcp", ":6379"),
    redistore.WithPassword("mypass"),
)
```

### 4. Specific Database

**v1:**
```go
store, err := redistore.NewRediStoreWithDB(
    10,
    "tcp",
    ":6379",
    "",
    "",
    "5",          // database
    []byte("secret-key"),
)
```

**v2:**
```go
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithAddress("tcp", ":6379"),
    redistore.WithDB("5"),
)

// Or using integer:
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithAddress("tcp", ":6379"),
    redistore.WithDBNum(5),
)
```

### 5. Using Redis URL

**v1:**
```go
store, err := redistore.NewRediStoreWithURL(
    10,
    "redis://:password@localhost:6379/0",
    []byte("secret-key"),
)
```

**v2:**
```go
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithURL("redis://:password@localhost:6379/0"),
)
```

### 6. Custom Connection Pool

**v1:**
```go
pool := &redis.Pool{
    MaxIdle: 100,
    IdleTimeout: 5 * time.Minute,
    Dial: func() (redis.Conn, error) {
        return redis.Dial("tcp", ":6379")
    },
}
store, err := redistore.NewRediStoreWithPool(pool, []byte("secret-key"))
```

**v2 - Option A (Use existing pool):**
```go
pool := &redis.Pool{
    MaxIdle: 100,
    IdleTimeout: 5 * time.Minute,
    Dial: func() (redis.Conn, error) {
        return redis.Dial("tcp", ":6379")
    },
}
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithPool(pool),
)
```

**v2 - Option B (Configure pool parameters):**
```go
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithAddress("tcp", ":6379"),
    redistore.WithPoolSize(100),
    redistore.WithIdleTimeout(5 * time.Minute),
)
```

### 7. Custom Configuration (Post-Creation)

**v1:**
```go
store, err := redistore.NewRediStore(10, "tcp", ":6379", "", "", []byte("secret-key"))
store.SetMaxLength(8192)
store.SetKeyPrefix("myapp_")
store.SetSerializer(redistore.JSONSerializer{})
```

**v2 (All at once):**
```go
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithAddress("tcp", ":6379"),
    redistore.WithMaxLength(8192),
    redistore.WithKeyPrefix("myapp_"),
    redistore.WithSerializer(redistore.JSONSerializer{}),
)
```

> **Note:** You can still use `SetMaxLength()`, `SetKeyPrefix()`, and `SetSerializer()` methods after creation in v2 if needed.

### 8. Complete Migration Example

**v1:**
```go
package main

import (
    "github.com/boj/redistore"
    "github.com/gorilla/sessions"
)

func main() {
    store, err := redistore.NewRediStoreWithDB(
        10,
        "tcp",
        "localhost:6379",
        "user",
        "password",
        "1",
        []byte("secret-key-123"),
    )
    if err != nil {
        panic(err)
    }
    defer store.Close()

    // Configure after creation
    store.SetMaxLength(8192)
    store.SetKeyPrefix("myapp_")
    store.SetMaxAge(86400 * 7) // 7 days

    // Use store...
}
```

**v2:**
```go
package main

import (
    "github.com/boj/redistore/v2"
    "github.com/gorilla/sessions"
)

func main() {
    store, err := redistore.NewStore(
        []byte("secret-key-123"),
        redistore.WithAddress("tcp", "localhost:6379"),
        redistore.WithAuth("user", "password"),
        redistore.WithDB("1"),
        redistore.WithMaxLength(8192),
        redistore.WithKeyPrefix("myapp_"),
        redistore.WithMaxAge(86400 * 7), // 7 days
    )
    if err != nil {
        panic(err)
    }
    defer store.Close()

    // Use store...
}
```

## Options Reference

### Connection Options (Required - Choose ONE)

```go
WithPool(pool *redis.Pool)              // Use custom pool
WithAddress(network, address string)    // Connect via network + address
WithURL(url string)                     // Connect via Redis URL
```

### Authentication Options

```go
WithAuth(username, password string)     // Set both username and password
WithPassword(password string)           // Set password only
```

### Redis Configuration Options

```go
WithDB(db string)                       // Database index as string ("0"-"15")
WithDBNum(dbNum int)                    // Database index as integer (0-15)
WithPoolSize(size int)                  // Connection pool size (default: 10)
WithIdleTimeout(timeout time.Duration)  // Idle timeout (default: 240s)
```

### Store Configuration Options

```go
WithMaxLength(length int)               // Max session size (default: 4096)
WithKeyPrefix(prefix string)            // Redis key prefix (default: "session_")
WithDefaultMaxAge(age int)              // Default TTL in seconds (default: 1200)
WithSerializer(s SessionSerializer)     // Serializer (default: GobSerializer)
WithSessionOptions(opts *sessions.Options) // Full session options
WithPath(path string)                   // Cookie path (default: "/")
WithMaxAge(age int)                     // Cookie MaxAge (default: 30 days)
```

## Common Pitfalls

### 1. Empty Key Pairs

**Error:**
```go
store, err := redistore.NewStore(
    []byte{}, // ❌ Empty key pairs
    redistore.WithAddress("tcp", ":6379"),
)
// Error: "at least one key pair is required"
```

**Fix:**
```go
store, err := redistore.NewStore(
    []byte("secret-key"), // ✅ Provide key
    redistore.WithAddress("tcp", ":6379"),
)
```

### 2. No Connection Option

**Error:**
```go
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithMaxLength(8192), // ❌ No connection option
)
// Error: "exactly one connection option is required"
```

**Fix:**
```go
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithAddress("tcp", ":6379"), // ✅ Add connection option
    redistore.WithMaxLength(8192),
)
```

### 3. Multiple Connection Options

**Error:**
```go
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithAddress("tcp", ":6379"),    // ❌
    redistore.WithURL("redis://localhost"), // ❌ Multiple connections
)
// Error: "only one connection option can be specified"
```

**Fix:**
```go
// Choose ONE connection method
store, err := redistore.NewStore(
    []byte("secret-key"),
    redistore.WithAddress("tcp", ":6379"), // ✅ Only one
)
```

## Testing Your Migration

After migrating, verify your application works correctly:

```bash
# Run your tests
go test ./...

# Check for compilation errors
go build ./...

# Run your application in development
go run main.go
```

## Rollback Plan

If you need to rollback to v1:

1. Revert your `go.mod`:
   ```bash
   go get github.com/boj/redistore@v1
   ```

2. Revert your code changes

3. Run `go mod tidy`

## Getting Help

- **Issues:** https://github.com/boj/redistore/issues
- **Discussions:** https://github.com/boj/redistore/discussions
- **Documentation:** https://pkg.go.dev/github.com/boj/redistore/v2

## Benefits of v2

✅ **Cleaner API** - Single entry point, easier to understand
✅ **More Flexible** - Mix and match any configuration options
✅ **Better Defaults** - Sensible defaults for all settings
✅ **Type Safety** - Compile-time validation of options
✅ **Extensible** - Easy to add new options without breaking changes
✅ **Self-Documenting** - Option names clearly indicate what they configure
