#!/bin/sh
set -e

# The control plane signs session tokens with an RSA key pair read from disk.
# Generate one on first start so a fresh clone runs without any manual setup;
# the key lives on a volume, so tokens stay valid across restarts.
if [ ! -f "$ORYCA_API_JWT_PRIVATE_KEY" ]; then
	echo "No signing key at $ORYCA_API_JWT_PRIVATE_KEY — generating one."
	mkdir -p "$(dirname "$ORYCA_API_JWT_PRIVATE_KEY")"
	openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 \
		-out "$ORYCA_API_JWT_PRIVATE_KEY" 2>/dev/null
	openssl rsa -in "$ORYCA_API_JWT_PRIVATE_KEY" -pubout \
		-out "$ORYCA_API_JWT_PUBLIC_KEY" 2>/dev/null
	chmod 600 "$ORYCA_API_JWT_PRIVATE_KEY"
fi

exec oryca-control-plane "$@"
