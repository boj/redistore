# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - 2026-01-13

### Breaking Changes

This is a major version update with breaking API changes. Please see [MIGRATION.md](MIGRATION.md) for detailed migration instructions.

#### Removed

- **`NewRediStore(size, network, address, username, password, keyPairs)`** - Replaced by `NewStore()` with `WithAddress()` option
- **`NewRediStoreWithDB(size, network, address, username, password, db, keyPairs)`** - Replaced by `NewStore()` with `WithAddress()` and `WithDB()` options
- **`NewRediStoreWithPool(pool, keyPairs)`** - Replaced by `NewStore()` with `WithPool()` option
- **`NewRediStoreWithURL(size, url, keyPairs)`** - Replaced by `NewStore()` with `WithURL()` option

### Added

#### New API

- **`NewStore(keyPairs [][]byte, opts...)`** - New unified initialization function using Option Pattern

  ```go
  // Single key
  store, err := NewStore(
      [][]byte{[]byte("secret-key")},
      WithAddress("tcp", ":6379"),
      WithDB("1"),
      WithMaxLength(8192),
  )

  // Multiple keys for key rotation
  store, err := NewStore(
      [][]byte{
          []byte("new-auth-key"),
          []byte("new-encrypt-key"),
          []byte("old-auth-key"),    // For decoding existing sessions
          []byte("old-encrypt-key"),
      },
      WithAddress("tcp", ":6379"),
  )
  ```

#### Key Rotation Support

- **Key pairs parameter changed from `[]byte` to `[][]byte`** - Enables proper support for encryption key rotation
- Keys are provided as byte slice pairs (authentication key, encryption key)
- First key pair is used for encoding new sessions
- All key pairs are tried for decoding, allowing seamless key rotation without invalidating existing sessions

#### Helper Functions

- **`Keys(keys ...[]byte)`** - Convenience function to create key pairs from byte slices

  ```go
  store, err := NewStore(Keys([]byte("key")), WithAddress("tcp", ":6379"))
  ```

- **`KeysFromStrings(keys ...string)`** - Most convenient way to create key pairs from strings

  ```go
  store, err := NewStore(KeysFromStrings("secret-key"), WithAddress("tcp", ":6379"))
  ```

These helpers simplify the creation of key pairs, eliminating the need to write `[][]byte{[]byte(...)}` every time.

#### Connection Options

- **`WithPool(pool)`** - Use custom Redis connection pool
- **`WithAddress(network, address)`** - Connect using network protocol and address
- **`WithURL(url)`** - Connect using Redis URL

#### Authentication Options

- **`WithAuth(username, password)`** - Set username and password for Redis authentication
- **`WithPassword(password)`** - Set password only (convenience function)

#### Redis Configuration Options

- **`WithDB(db)`** - Set Redis database index as string ("0"-"15")
- **`WithDBNum(dbNum)`** - Set Redis database index as integer (0-15)
- **`WithPoolSize(size)`** - Set connection pool size (default: 10)
- **`WithIdleTimeout(timeout)`** - Set connection idle timeout (default: 240s)

#### Store Configuration Options

- **`WithMaxLength(length)`** - Set maximum session data size (default: 4096 bytes)
- **`WithKeyPrefix(prefix)`** - Set Redis key prefix (default: "session\_")
- **`WithDefaultMaxAge(age)`** - Set default TTL in seconds (default: 1200)
- **`WithSerializer(serializer)`** - Set session serializer (default: GobSerializer)
- **`WithSessionOptions(opts)`** - Set full session options
- **`WithPath(path)`** - Set cookie path (default: "/")
- **`WithMaxAge(age)`** - Set cookie MaxAge (default: 30 days)

#### Testing

- **`redistore_options_test.go`** - Comprehensive test suite for option validation
- Added 23+ new tests covering:
  - Configuration validation
  - Error handling
  - Option combinations
  - Default values

### Changed

- Module path updated to `github.com/boj/redistore/v2` for v2 versioning
- Improved error messages with detailed context
- Configuration validation moved to initialization time (fail fast)

### Improved

- **Code Quality**

  - Eliminated code duplication across initialization functions
  - Better separation of concerns with internal configuration structures
  - More maintainable and extensible codebase

- **API Design**

  - Self-documenting option names
  - Compile-time validation of configuration
  - Flexible option combinations
  - Better default values

- **Developer Experience**
  - Single entry point reduces learning curve
  - Options can be applied in any order
  - Clear error messages for invalid configurations
  - Comprehensive documentation

### Fixed

- Connection validation now happens during initialization (not on first use)
- Better handling of nil values in configuration
- Improved database number validation (0-15 range)

### Documentation

- **Added `MIGRATION.md`** - Comprehensive migration guide from v1 to v2
- **Added `CHANGELOG.md`** - Version history and change tracking
- Updated all code examples to use new API
- Improved inline documentation with examples

### Technical Details

#### Internal Changes

- Added `Option` function type for configuration
- Added `storeConfig` struct for internal configuration management
- Added `addressConfig` struct for network address configuration
- Implemented `defaultConfig()` for sensible defaults
- Implemented `validate()` for configuration validation
- Implemented `buildPool()` for pool creation from configuration

#### Compatibility

- **Go Version**: Requires Go 1.23+
- **Dependencies**: No changes to external dependencies
  - `github.com/gomodule/redigo v1.9.3`
  - `github.com/gorilla/securecookie v1.1.2`
  - `github.com/gorilla/sessions v1.4.0`

## [1.0.0] - Previous Releases

For changes prior to v2.0.0, please refer to the git history.

### v1 API (Deprecated)

The v1 API included four initialization functions:

- `NewRediStore()` - Basic initialization
- `NewRediStoreWithDB()` - With database selection
- `NewRediStoreWithPool()` - With custom pool
- `NewRediStoreWithURL()` - With Redis URL

These have been replaced by the unified `NewStore()` function with options in v2.

---

## Migration Guide

See [MIGRATION.md](MIGRATION.md) for step-by-step instructions on upgrading from v1 to v2.

## Support

- **Issues**: https://github.com/boj/redistore/issues
- **Discussions**: https://github.com/boj/redistore/discussions
- **Documentation**: https://pkg.go.dev/github.com/boj/redistore/v2
