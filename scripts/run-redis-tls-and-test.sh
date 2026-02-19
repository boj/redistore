#!/usr/bin/env bash
set -euo pipefail

# Run a TLS-enabled Redis in Docker, run the TLS integration test, then cleanup.
# Usage:
#   ./scripts/run-redis-tls-and-test.sh        # create artifacts, run test, cleanup
#   KEEP=1 ./scripts/run-redis-tls-and-test.sh # keep container and files for inspection

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKDIR="$HERE/redis-tls-test"
mkdir -p "$WORKDIR/certs"
cd "$WORKDIR"

echo "Working dir: $WORKDIR"

if [ ! -f certs/ca.crt ]; then
  echo "Generating CA and server certificates..."
  openssl genrsa -out certs/ca.key 4096
  openssl req -x509 -new -nodes -key certs/ca.key -sha256 -days 3650 -subj "/CN=Redis Test CA" -out certs/ca.crt

  openssl genrsa -out certs/redis.key 4096
  openssl req -new -key certs/redis.key -subj "/CN=redis.local" -out certs/redis.csr
  printf "subjectAltName = IP:127.0.0.1, DNS:redis.local\n" > certs/redis.ext
  openssl x509 -req -in certs/redis.csr -CA certs/ca.crt -CAkey certs/ca.key -CAcreateserial -out certs/redis.crt -days 365 -sha256 -extfile certs/redis.ext
fi

# Ensure correct permissions: private key must be owner-readable only so Redis can load it
chmod 644 certs/redis.key || true
chmod 644 certs/redis.crt certs/ca.crt || true

cat > redis.conf <<'EOF'
tls-port 6379
port 0
bind 0.0.0.0
tls-cert-file /certs/redis.crt
tls-key-file  /certs/redis.key
tls-ca-cert-file /certs/ca.crt
tls-auth-clients no
protected-mode no
EOF

echo "Removing any existing container named redis-tls..."
if docker ps -a --format '{{.Names}}' | grep -q '^redis-tls$'; then
  docker rm -f redis-tls >/dev/null || true
fi

echo "Starting redis:7 container with TLS enabled (host port 6380 -> container 6379)..."
docker run -d --name redis-tls -p 6380:6379 -v "$PWD/redis.conf":/usr/local/etc/redis/redis.conf:ro -v "$PWD/certs":/certs:ro redis:7 redis-server /usr/local/etc/redis/redis.conf

echo "Waiting for TLS Redis to respond to PING..."
for i in {1..30}; do
  echo "Waiting for TLS Redis to respond to PING...attempt $i/30"
  if docker run --rm -v "$PWD/certs":/certs --link redis-tls:redis redis:7 redis-cli --tls -h redis -p 6379 --cacert /certs/ca.crt PING 2>/dev/null | grep -q PONG; then
    echo "Redis TLS is up"
    break
  fi
  sleep 1
done

echo "Running Go TLS integration test..."
export TLS_REDIS_ADDR=127.0.0.1:6380
export TLS_REDIS_CA="$PWD/certs/ca.crt"
(
  cd "$HERE"
  TLS_REDIS_ADDR=$TLS_REDIS_ADDR TLS_REDIS_CA=$TLS_REDIS_CA go test -run TestRedisTLSIntegration -v
)
TEST_EXIT=$?

if [ "${KEEP:-}" = "1" ]; then
  echo "KEEP=1; leaving container and artifacts in $WORKDIR"
  exit $TEST_EXIT
fi

echo "Cleaning up..."
docker rm -f redis-tls >/dev/null || true
rm -rf "$WORKDIR"

exit $TEST_EXIT
