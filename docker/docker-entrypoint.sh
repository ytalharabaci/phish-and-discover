#!/bin/bash

# ═════════════════════════════════════════════════════════════════════════════════
# PHISH & DISCOVER Docker Entrypoint Script
# Handles initialization, configuration, and graceful startup
# ═════════════════════════════════════════════════════════════════════════════════

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# ─────────────────────────────────────────────────────────────────────────────
# Logging Functions
# ─────────────────────────────────────────────────────────────────────────────
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# ─────────────────────────────────────────────────────────────────────────────
# Initialize Configuration
# ─────────────────────────────────────────────────────────────────────────────
initialize_config() {
    log_info "Initializing configuration..."
    APP_CONFIG_PATH="/tmp/config.json"
    
    # Check if config.json exists, fallback to config.json.example
    local src_config="/opt/phish-and-discover/config.json"
    if [ ! -f "$src_config" ]; then
        log_warn "config.json not found at $src_config, falling back to config.json.example"
        src_config="/opt/phish-and-discover/config.json.example"
    fi
    
    # Read environment variables with fallbacks
    local db_path="${DB_PATH:-/opt/phish-and-discover/db/phish.db}"
    local admin_listen="${ADMIN_LISTEN:-0.0.0.0:3333}"
    local phish_listen="${PHISH_LISTEN:-0.0.0.0:8080}"
    local migrations_prefix="${MIGRATIONS_PREFIX:-/opt/phish-and-discover/migrations/db_}"
    local chisel_port="${CHISEL_SERVER_PORT:-9090}"
    # gorilla/handlers' ProxyHeaders sets r.URL.Scheme from X-Forwarded-Proto
    # but never sets r.URL.Host, so gorilla/csrf's same-origin check on HTTPS
    # requests always fails behind a reverse proxy (Caddy) — every POST to the
    # admin panel gets "Forbidden - referer invalid". trusted_origins is the
    # library's own escape hatch for exactly this case.
    local admin_domain="${ADMIN_DOMAIN:-localhost}"

    # Use jq to securely and correctly update keys in the config JSON
    jq \
      --arg db_path "$db_path" \
      --arg migrations_prefix "$migrations_prefix" \
      --arg admin_listen "$admin_listen" \
      --arg phish_listen "$phish_listen" \
      --arg chisel_port "$chisel_port" \
      --arg admin_domain "$admin_domain" \
      '.db_path = $db_path |
       .migrations_prefix = $migrations_prefix |
       .admin_server.listen_url = $admin_listen |
       .admin_server.trusted_origins = [$admin_domain] |
       .phish_server.listen_url = $phish_listen |
       .chisel_server_port = $chisel_port' \
      "$src_config" > "$APP_CONFIG_PATH"
    
    log_success "Configuration initialized (at $APP_CONFIG_PATH)"
}

# ─────────────────────────────────────────────────────────────────────────────
# Verify Capabilities
# ─────────────────────────────────────────────────────────────────────────────
verify_capabilities() {
    log_info "Verifying Linux capabilities..."
    
    # Use capsh to check capabilities if available, otherwise fallback to status grep
    if command -v capsh &> /dev/null; then
        CAPS=$(capsh --print)
        if echo "$CAPS" | grep -q "cap_net_admin"; then
            log_success "NET_ADMIN capability verified"
        else
            log_warn "NET_ADMIN capability not available - TUN/TAP tunneling may not work"
        fi
        
        if echo "$CAPS" | grep -q "cap_net_raw"; then
            log_success "NET_RAW capability verified"
        else
            log_warn "NET_RAW capability not available - naabu/httpx port scanning may not work"
        fi
    else
        # Fallback to /proc/self/status bits check (simplified)
        log_info "capsh not found, skipping advanced capability verification"
    fi
    
    # Check for /dev/net/tun device
    if [ ! -c /dev/net/tun ]; then
        log_warn "/dev/net/tun device not available - TUN/TAP not available"
    fi
    
    log_success "Capability verification complete"
}


# ─────────────────────────────────────────────────────────────────────────────
# Initialize Database
# ─────────────────────────────────────────────────────────────────────────────
initialize_database() {
    log_info "Initializing database..."
    
    DB_DIR=$(dirname "${DB_PATH:-/opt/phish-and-discover/db/phish.db}")
    mkdir -p "$DB_DIR"
    
    if [ -f "$DB_DIR/phish.db" ]; then
        log_info "Database file already exists, skipping initialization"
    else
        log_info "Creating new database..."
        # Database will be auto-created by Phish & Discover on first run
        # This is just ensuring the directory exists
        touch "$DB_DIR/phish.db"
    fi
    
    log_success "Database initialized"
}

# ─────────────────────────────────────────────────────────────────────────────
# Initialize Nuclei Templates
# ─────────────────────────────────────────────────────────────────────────────
initialize_nuclei() {
    log_info "Checking Nuclei templates..."
    
    NUCLEI_TEMPLATES_DIR="/home/phish/.nuclei-templates"
    mkdir -p "$NUCLEI_TEMPLATES_DIR"
    
    # Check if templates already cached
    if [ -d "$NUCLEI_TEMPLATES_DIR/nuclei-templates" ] && [ -f "$NUCLEI_TEMPLATES_DIR/nuclei-templates/.gitkeep" ]; then
        TEMPLATE_COUNT=$(find "$NUCLEI_TEMPLATES_DIR" -name "*.yaml" 2>/dev/null | wc -l)
        log_info "Found $TEMPLATE_COUNT cached Nuclei templates"
    else
        log_warn "Nuclei templates not cached - will download on first scan"
        log_info "This may take 5-10 minutes. Templates will be cached for future runs."
    fi
    
    log_success "Nuclei initialization complete"
}

# ─────────────────────────────────────────────────────────────────────────────
# Verify Tool Installation
# ─────────────────────────────────────────────────────────────────────────────
verify_tools() {
    log_info "Verifying ProjectDiscovery tools..."
    
    TOOLS=(
        "subfinder"
        "httpx"
        "nuclei"
        "naabu"
        "dnsx"
        "katana"
        "tlsx"
        "asnmap"
        "uncover"
        "interactsh-client"
        "chisel"
    )
    
    MISSING=0
    for TOOL in "${TOOLS[@]}"; do
        if command -v "$TOOL" &> /dev/null; then
            VERSION=$("$TOOL" -version 2>&1 | head -1 || echo "version unknown")
            log_info "✓ $TOOL ($VERSION)"
        else
            log_warn "✗ $TOOL NOT FOUND"
            MISSING=$((MISSING + 1))
        fi
    done
    
    if [ $MISSING -gt 0 ]; then
        log_warn "Missing $MISSING tools - some scanning features may not work"
    else
        log_success "All tools verified"
    fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Setup Signal Handlers for Graceful Shutdown
# ─────────────────────────────────────────────────────────────────────────────
setup_signal_handlers() {
    # Trap SIGTERM and SIGINT for graceful shutdown
    trap 'log_warn "Received shutdown signal, stopping gracefully..."; kill -TERM $SERVER_PID 2>/dev/null; exit 130' SIGTERM SIGINT
}

# ─────────────────────────────────────────────────────────────────────────────
# Health Check
# ─────────────────────────────────────────────────────────────────────────────
wait_for_service() {
    log_info "Waiting for Phish & Discover to become ready..."
    
    TIMEOUT=120
    ELAPSED=0
    INTERVAL=5
    
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if curl -sf http://localhost:3333/ > /dev/null 2>&1; then
            log_success "Phish & Discover is ready!"
            return 0
        fi
        
        log_info "Waiting... ($((ELAPSED))s)"
        sleep $INTERVAL
        ELAPSED=$((ELAPSED + INTERVAL))
    done
    
    log_error "Phish & Discover failed to start within ${TIMEOUT}s"
}

# ─────────────────────────────────────────────────────────────────────────────
# Banner
# ─────────────────────────────────────────────────────────────────────────────
show_banner() {
    cat <<EOF

${BLUE}╔════════════════════════════════════════════════════════════════╗${NC}
${BLUE}║                                                                ║${NC}
${BLUE}║  PHISH & DISCOVER - Unified Security Operations Platform               ║${NC}
${BLUE}║  Gophish + ProjectDiscovery Tools + Reverse Tunneling         ║${NC}
${BLUE}║                                                                ║${NC}
${BLUE}║  Starting Docker Container...                                 ║${NC}
${BLUE}║                                                                ║${NC}
${BLUE}╚════════════════════════════════════════════════════════════════╝${NC}

EOF
}

# ─────────────────────────────────────────────────────────────────────────────
# Main Startup
# ─────────────────────────────────────────────────────────────────────────────
main() {
    show_banner
    
    log_info "Starting Phish & Discover initialization sequence..."
    
    # Run initialization steps
    initialize_config
    verify_capabilities
    initialize_database
    initialize_nuclei
    verify_tools
    
    log_success "Initialization complete!"
    log_info ""
    log_info "Starting Phish & Discover Server..."
    log_info ""
    log_info "Available ports:"
    log_info "  - Admin Dashboard: http://localhost:3333"
    log_info "  - HTTP Listener: http://localhost:80"
    log_info "  - HTTPS Listener: https://localhost:443"
    log_info "  - Chisel Tunnel: :8080 (for reverse tunnel agents)"
    log_info ""
    
    # Setup signal handlers
    setup_signal_handlers
    
    # Execute the command passed to entrypoint with the working config
    exec "$@" --config "$APP_CONFIG_PATH"
}

# Run main
main "$@"
