#!/bin/sh
# Fails if anything that must never be published would be published.
# Run before every commit while the source is still being cut down, and again
# before the first push.
#
#   ./tools/secret-scan.sh
#
# Inside a git repository this inspects tracked files only, so ignored local
# files such as .env are correctly left alone. Outside one it falls back to
# walking the working tree.
#
# POSIX sh on purpose: this has to run on a stock macOS shell as well as CI.
set -u

cd "$(dirname "$0")/.." || exit 1
failed=0

if git rev-parse --git-dir >/dev/null 2>&1; then
	files=$(git ls-files)
	scope="tracked files"
else
	files=$(find . -path ./.git -prune -o -path ./portal/node_modules -prune -o -type f -print)
	scope="working tree"
fi

report() {
	failed=1
	printf '\n✗ %s\n' "$1"
	printf '%s\n' "$2" | sed 's/^/    /'
}

# Files that must never be published at all.
forbidden=$(printf '%s\n' "$files" | grep -E \
	'(\.key|\.key\.pub|\.pem|\.p12|\.log|/\.env|^\.env)$|\.backup\.|/(cover|coverage)\.out$' || true)
[ -n "$forbidden" ] && report "Forbidden files:" "$forbidden"

# Internal planning documents and operational tooling.
internal=$(printf '%s\n' "$files" | grep -E \
	'(notification-plan\.md|payment-gateway-plan\.md|plan-optimize.*\.md|sonar-project\.properties|ce-denylist\.txt)$' || true)
[ -n "$internal" ] && report "Internal documents:" "$internal"

# Key material pasted into a file that is not named like a key. The pattern is
# written as a character class so this line does not match itself.
inline_keys=$(printf '%s\n' "$files" | tr '\n' '\0' \
	| xargs -0 grep -lIE 'BEGIN [A-Z ]*PRIVATE KEY' 2>/dev/null || true)
[ -n "$inline_keys" ] && report "Private key material inside files:" "$inline_keys"

if [ "$failed" -eq 0 ]; then
	echo "✓ secret scan clean ($scope)"
fi
exit "$failed"
