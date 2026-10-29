#!/bin/bash
# setup.sh — One-command bootstrap for deploying Phish & Discover with Docker Compose.
#
# Creates .env from .env.example if missing, fills in the values that have
# real security consequences if left at their defaults (admin path secret,
# basic-auth password hash, initial admin password), then builds and starts
# the stack.
#
# Usage:
#   ./scripts/setup.sh      interactive prompts for domain/email/mail settings
#
# Re-running is safe: it never overwrites an existing .env value that's
# already been customized away from the .env.example default.

set -euo pipefail
cd "$(dirname "$0")/.."

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BLUE='\033[0;34m'; NC='\033[0m'
info()  { echo -e "${BLUE}[setup]${NC} $1"; }
ok()    { echo -e "${GREEN}[setup]${NC} $1"; }
warn()  { echo -e "${YELLOW}[setup]${NC} $1"; }
fail()  { echo -e "${RED}[setup]${NC} $1"; exit 1; }

command -v docker >/dev/null 2>&1 || fail "docker not found. Install it first: https://get.docker.com"
if docker compose version >/dev/null 2>&1; then
    COMPOSE="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
    COMPOSE="docker-compose"
else
    fail "Neither 'docker compose' nor 'docker-compose' found."
fi

random_token() {
    # $1 = byte length before hex-encoding
    head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'
}

if [ ! -f .env ]; then
    cp .env.example .env
    ok "Created .env from .env.example"
else
    info ".env already exists — only filling in missing/default values"
fi

# shellcheck disable=SC1091
set -a; source .env; set +a

set_env() {
    # set_env KEY VALUE — updates KEY=... in .env (creates it if absent)
    local key="$1" value="$2"
    if grep -q "^${key}=" .env; then
        sed -i.bak "s#^${key}=.*#${key}=${value}#" .env && rm -f .env.bak
    else
        echo "${key}=${value}" >> .env
    fi
}

if [ "${ADMIN_PATH_SECRET:-phish-ops-secret-777}" = "phish-ops-secret-777" ]; then
    NEW_SECRET="phish-ops-$(random_token 12)"
    set_env ADMIN_PATH_SECRET "$NEW_SECRET"
    ADMIN_PATH_SECRET="$NEW_SECRET"
    ok "Generated a random ADMIN_PATH_SECRET (the default value is public — anyone who reads this repo knows it)"
fi

if [ "${DOMAIN:-localhost}" = "localhost" ]; then
    read -rp "Public domain for phishing/landing pages (e.g. phish.example.com): " DOMAIN_INPUT
    [ -n "$DOMAIN_INPUT" ] || fail "A real domain is required (Let's Encrypt needs it to issue a certificate)."
    set_env DOMAIN "$DOMAIN_INPUT"
    DOMAIN="$DOMAIN_INPUT"
fi
if [ "${ADMIN_DOMAIN:-localhost}" = "localhost" ]; then
    read -rp "Admin dashboard domain, must differ from the one above (e.g. admin.example.com): " ADMIN_DOMAIN_INPUT
    [ -n "$ADMIN_DOMAIN_INPUT" ] || fail "A real admin domain is required (Let's Encrypt needs it to issue a certificate)."
    set_env ADMIN_DOMAIN "$ADMIN_DOMAIN_INPUT"
    ADMIN_DOMAIN="$ADMIN_DOMAIN_INPUT"
fi
if [ "${CADDY_EMAIL:-admin@phish.local}" = "admin@phish.local" ]; then
    read -rp "Contact email for Let's Encrypt (e.g. you@example.com): " EMAIL_INPUT
    [ -n "$EMAIL_INPUT" ] || fail "A contact email is required."
    set_env CADDY_EMAIL "$EMAIL_INPUT"
fi
if [ "${MAIL_HOSTNAME:-phish.local}" = "phish.local" ]; then
    warn "MAIL_HOSTNAME/MAIL_DOMAIN are at their default — Postfix will send as 'phish.local', which most mail servers will reject."
    read -rp "Mail server hostname for Postfix, must match this VPS's PTR/rDNS record (e.g. mail.example.com), or leave blank to skip: " MAIL_HOSTNAME_INPUT
    if [ -n "$MAIL_HOSTNAME_INPUT" ]; then
        set_env MAIL_HOSTNAME "$MAIL_HOSTNAME_INPUT"
        read -rp "Sending domain for DKIM signing, matches the Gophish 'From' address domain (e.g. example.com): " MAIL_DOMAIN_INPUT
        [ -n "$MAIL_DOMAIN_INPUT" ] || fail "A mail domain is required when MAIL_HOSTNAME is set."
        set_env MAIL_DOMAIN "$MAIL_DOMAIN_INPUT"
    else
        warn "Skipping mail setup — direct sending will likely be rejected/spam-flagged until MAIL_HOSTNAME/MAIL_DOMAIN are set."
    fi
fi
if [ -z "${ADMIN_PASS_HASH:-}" ]; then
    warn "ADMIN_PASS_HASH is empty — Caddy's Basic Auth layer in front of the admin dashboard will be DISABLED."
    read -rp "Set a Basic Auth password now? [Y/n] " ANSWER
    if [ "${ANSWER:-Y}" != "n" ] && [ "${ANSWER:-Y}" != "N" ]; then
        read -rsp "Password: " ADMIN_PASSWORD; echo
        HASH=$(docker run --rm caddy caddy hash-password --plaintext "$ADMIN_PASSWORD")
        set_env ADMIN_PASS_HASH "$HASH"
        ok "Basic Auth password set"
    else
        warn "Continuing without Basic Auth. The Gophish login screen is still behind the secret admin path and its own auth, but consider setting ADMIN_PASS_HASH later."
    fi
fi

if [ -z "${GOPHISH_INITIAL_ADMIN_PASSWORD:-}" ]; then
    GENERATED_PASSWORD=$(random_token 16)
    set_env GOPHISH_INITIAL_ADMIN_PASSWORD "$GENERATED_PASSWORD"
    ok "Generated the initial Gophish admin password (shown again at the end of this script)"
fi

info "Building images (this includes compiling the ProjectDiscovery toolchain — can take several minutes the first time)..."
$COMPOSE build

# phish-core runs as a non-root user; the bind-mounted ./phish_data dir is
# created by Docker as root on first run, which makes the container fail with
# "Permission denied" writing the sqlite db. Pre-create it owned by the same
# uid:gid the image's 'phish' user has.
mkdir -p ./phish_data
PHISH_UID_GID=$(docker run --rm --entrypoint id ghcr.io/ytalharabaci/phish-and-discover:latest -u phish):$(docker run --rm --entrypoint id ghcr.io/ytalharabaci/phish-and-discover:latest -g phish)
chown -R "$PHISH_UID_GID" ./phish_data 2>/dev/null || warn "Could not chown ./phish_data (not running as root?) — phish-core may fail to write its database."

info "Starting services..."
$COMPOSE up -d

info "Waiting for phish-core to report healthy..."
for _ in $(seq 1 30); do
    STATUS=$(docker inspect -f '{{.State.Health.Status}}' phish_core 2>/dev/null || true)
    if [ "$STATUS" = "healthy" ]; then
        break
    fi
    sleep 5
done
if [ "$STATUS" != "healthy" ]; then
    warn "phish-core did not report healthy within 150s — check '$COMPOSE logs phish-core'"
fi

# shellcheck disable=SC1091
set -a; source .env; set +a

echo
ok "Phish & Discover is up."
echo "  URL:      https://${ADMIN_DOMAIN}/${ADMIN_PATH_SECRET}"
echo "  Username: admin"
echo "  Password: ${GOPHISH_INITIAL_ADMIN_PASSWORD}"
echo
warn "Save the password above now — it is only generated once and is needed for first login (you will be asked to change it)."
