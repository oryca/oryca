#!/bin/sh
# End-to-end smoke test against a running `docker compose up` stack.
#
#   ./tools/smoke-test.sh
#
# Walks the path a real user takes — sign in, register a service, hand out an API
# key, call through the gateway — and then checks that the request shows up in the
# dashboard with the right owner. Exits non-zero on the first failure.
#
# Reads the generated root password out of the control-plane log, so the stack has
# to have been started from an empty database at least once.
#
# Each run names the things it creates after its own run id, so running it twice
# does not collide. They are left behind on purpose: when something fails, the
# half-built state is what tells you where it stopped.

set -eu

RUN_ID="${RUN_ID:-$(date +%s)}"

CP="${CP:-http://localhost:9001/control-plane/api/v1}"
GW="${GW:-http://localhost:9002/gateway/api}"

# The upstream the gateway proxies to. It defaults to the control plane's own
# health endpoint, reachable on the compose network, so the test never depends on
# a site on the internet being up. Point it anywhere else with UPSTREAM_URL.
#
# UPSTREAM_URL is the whole destination address, not a prefix: a source URL is
# the full upstream endpoint, and UPSTREAM_PATH is only the route the gateway
# publishes it under.
UPSTREAM_URL="${UPSTREAM_URL:-http://control-plane:9001/control-plane/api/v1/health}"
UPSTREAM_PATH="${UPSTREAM_PATH:-/health}"
ROOT_EMAIL="${ORYCA_API_ROOT_EMAIL:-admin@localhost}"

pass=0
step() { printf '\n\033[1m%s\033[0m\n' "$1"; }
ok() { pass=$((pass + 1)); printf '  \033[32mok\033[0m   %s\n' "$1"; }
fail() { printf '  \033[31mFAIL\033[0m %s\n' "$1"; exit 1; }

need() {
	[ -n "$1" ] && [ "$1" != "null" ] || fail "$2"
}

json() { # json <field> — pulls one top-level field out of stdin
	python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('$1',''))"
}

# ---------------------------------------------------------------- sign in

step "1. sign in as the seeded root user"

ROOT_PASSWORD="${ORYCA_API_ROOT_PASSWORD:-}"
if [ -z "$ROOT_PASSWORD" ]; then
	ROOT_PASSWORD=$(docker compose logs control-plane 2>/dev/null |
		grep 'password:' | tail -1 | sed 's/.*password: *//' | tr -d '\r')
fi
need "$ROOT_PASSWORD" "no root password: set ORYCA_API_ROOT_PASSWORD or start from an empty database"

TOKEN=$(curl -sS -X POST "$CP/auth/login" -H 'Content-Type: application/json' \
	-d "{\"username\":\"$ROOT_EMAIL\",\"password\":\"$ROOT_PASSWORD\"}" | json accessToken)
need "$TOKEN" "login failed"
ok "signed in"

AUTH="Authorization: Bearer $TOKEN"
JSON='Content-Type: application/json'

# ---------------------------------------------------------------- publish an API

step "2. publish an upstream as a gateway service"

curl -sS -X POST "$CP/sources" -H "$AUTH" -H "$JSON" -d '{
  "alias":"smoke-upstream-'"$RUN_ID"'","name":"Smoke upstream","type":"api",
  "protocol":"http","url":"'"$UPSTREAM_URL"'","contentType":"application/json"
}' >/dev/null || fail "could not create the source"
ok "source created"

SERVICE_ID=$(curl -sS -X POST "$CP/services" -H "$AUTH" -H "$JSON" -d '{
  "name":"Smoke '"$RUN_ID"'","type":"General","basePath":"/smoke-'"$RUN_ID"'","enabled":true,"isPublic":false,
  "resourcePaths":[{"path":"'"$UPSTREAM_PATH"'","methods":["GET"],"sourceAlias":"smoke-upstream-'"$RUN_ID"'"}]
}' | json id)
need "$SERVICE_ID" "could not create the service"
ok "service created"

# ---------------------------------------------------------------- a real user

step "3. a user signs up and gets an API key"

curl -sS -X POST "$CP/auth/register" -H "$JSON" -d '{
  "email":"smoke-'"$RUN_ID"'@example.com","password":"SmokeTest123!","firstName":"Smoke","lastName":"Test"
}' >/dev/null || fail "registration failed"

USER_ID=$(curl -sS "$CP/users?search=smoke-$RUN_ID@example.com" -H "$AUTH" |
	python3 -c "import sys,json; print((json.load(sys.stdin).get('items') or [{}])[0].get('id',''))")
need "$USER_ID" "the registered user was not found"

PACKAGE_ID=$(curl -sS "$CP/users/$USER_ID" -H "$AUTH" | json packageId)
need "$PACKAGE_ID" "the default package was not assigned on sign-up"
ok "signed up and assigned the default package"

# No mail server in a default install, so the administrator confirms the account.
curl -sS -X PUT "$CP/users/$USER_ID" -H "$AUTH" -H "$JSON" -d '{
  "firstName":"Smoke","lastName":"Test","role":"user","verified":true,"enabled":true,
  "packageId":"'"$PACKAGE_ID"'"
}' >/dev/null || fail "could not verify the user"
ok "account verified by the administrator"

curl -sS -X POST "$CP/packages/$PACKAGE_ID/services" -H "$AUTH" -H "$JSON" -d '{
  "serviceId":"'"$SERVICE_ID"'",
  "paths":[{"path":"'"$UPSTREAM_PATH"'","methods":["GET"],
            "policies":{"rateLimit":{"enabled":true,"tiers":[{"limit":5,"windowSec":10}]}}}]
}' >/dev/null || fail "could not grant the package access to the service"
ok "package granted access, rate limited to 5 per 10s"

USER_TOKEN=$(curl -sS -X POST "$CP/auth/login" -H "$JSON" \
	-d "{\"username\":\"smoke-$RUN_ID@example.com\",\"password\":\"SmokeTest123!\"}" | json accessToken)
need "$USER_TOKEN" "the user could not sign in"

API_KEY=$(curl -sS -X POST "$CP/api-keys" -H "Authorization: Bearer $USER_TOKEN" -H "$JSON" \
	-d '{"name":"smoke-key"}' | json apiKey)
need "$API_KEY" "could not create an API key"
ok "API key issued"

# ---------------------------------------------------------------- through the gateway

step "4. call the API through the gateway"

sleep 2 # the gateway applies the change from the sync channel

code=$(curl -sS -o /dev/null -w '%{http_code}' "$GW/resources/smoke-$RUN_ID$UPSTREAM_PATH")
[ "$code" = "401" ] || fail "a call with no key returned $code, expected 401"
ok "unauthenticated call rejected"

code=$(curl -sS -o /dev/null -w '%{http_code}' "$GW/resources/smoke-$RUN_ID$UPSTREAM_PATH" -H "X-API-Key: $API_KEY")
[ "$code" = "200" ] || fail "an authenticated call returned $code, expected 200"
ok "authenticated call proxied"

limited=0
i=0
while [ $i -lt 8 ]; do
	code=$(curl -sS -o /dev/null -w '%{http_code}' "$GW/resources/smoke-$RUN_ID$UPSTREAM_PATH" -H "X-API-Key: $API_KEY")
	[ "$code" = "429" ] && limited=1 && break
	i=$((i + 1))
done
[ $limited -eq 1 ] || fail "the rate limit never kicked in over 8 rapid calls"
ok "rate limit enforced"

# ---------------------------------------------------------------- dashboard

step "5. the traffic reaches the dashboard"

sleep 3 # the consumer batches for up to a second before writing

total=$(curl -sS "$CP/dashboard/summary" -H "$AUTH" | json total)
[ "${total:-0}" -gt 0 ] || fail "the dashboard shows no requests at all"
ok "requests recorded ($total in the last 24h)"

own=$(curl -sS "$CP/dashboard/summary" -H "Authorization: Bearer $USER_TOKEN" | json total)
[ "${own:-0}" -gt 0 ] || fail "the user cannot see their own requests"

# Asking for someone else's traffic must not widen what a plain user can see.
peeked=$(curl -sS "$CP/dashboard/summary?userId=000000000000000000000000" \
	-H "Authorization: Bearer $USER_TOKEN" | json total)
[ "$peeked" = "$own" ] || fail "a user changed their scope with a userId filter ($peeked vs $own)"
ok "a user sees their own traffic and only their own"

printf '\n\033[32m%s checks passed\033[0m\n' "$pass"
