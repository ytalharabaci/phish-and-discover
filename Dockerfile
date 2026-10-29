# ========================================================================================
# PHISH & DISCOVER: Enterprise Multi-Stage Dockerfile
# Gophish + ProjectDiscovery Tools + Reverse Tunnel Infrastructure
# Final production image: Minimal Debian with compiled binaries only
# ========================================================================================

# ========================================================================================
# STAGE 1: Asset Minification (JavaScript/CSS)
# ========================================================================================
FROM node:20-alpine AS asset-builder
RUN npm install -g gulp gulp-cli
WORKDIR /build
COPY package.json ./
RUN npm install --only=dev
COPY . .
RUN gulp

# ========================================================================================
# STAGE 2: ProjectDiscovery Tools Builder
# ========================================================================================
FROM golang:1.25-bookworm AS pd-tools-builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpcap-dev libdumbnet-dev gcc g++ make && \
    rm -rf /var/lib/apt/lists/*

# GOTOOLCHAIN=auto lets `go install ...@latest` fetch a newer Go toolchain
# on demand (e.g. ProjectDiscovery tools now require >= go 1.26), so this
# stage keeps building as upstream tools bump their minimum Go version.
ENV GOPROXY=https://proxy.golang.org,direct CGO_ENABLED=1 GOTOOLCHAIN=auto
RUN set -eux; \
    # All required tools for Phish & Discover platform
    go install -v github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest; \
    go install -v github.com/projectdiscovery/httpx/cmd/httpx@latest; \
    go install -v github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest; \
    go install -v github.com/projectdiscovery/naabu/v2/cmd/naabu@latest; \
    go install -v github.com/projectdiscovery/dnsx/cmd/dnsx@latest; \
    go install -v github.com/projectdiscovery/katana/cmd/katana@latest; \
    go install -v github.com/projectdiscovery/tlsx/cmd/tlsx@latest; \
    go install -v github.com/projectdiscovery/asnmap/cmd/asnmap@latest; \
    go install -v github.com/projectdiscovery/uncover/cmd/uncover@latest; \
    go install -v github.com/projectdiscovery/interactsh/cmd/interactsh-client@latest; \
    go install -v github.com/jpillora/chisel@latest; \
    true

# ========================================================================================
# STAGE 3: Phish & Discover Backend Builder
# ========================================================================================
FROM golang:1.25-bookworm AS app-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w" \
    -trimpath \
    -o phish-server .

# ========================================================================================
# STAGE 4: Production Runtime
# ========================================================================================
FROM debian:bookworm-slim

LABEL maintainer="Phish & Discover Security Platform" \
      description="Unified Gophish + ProjectDiscovery Security Operations Hub v2.0"

# Install minimal runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpcap0.8 ca-certificates iproute2 iptables libcap2-bin \
    libgomp1 libdumbnet1 curl wget jq dnsmasq && \
    rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Create application user with home directory
RUN groupadd -r phish && useradd -r -g phish -d /home/phish -m phish
ENV HOME=/home/phish

WORKDIR /opt/phish-and-discover

# Copy ProjectDiscovery tools
COPY --from=pd-tools-builder /go/bin/* /usr/local/bin/

# Copy Phish & Discover application
COPY --from=app-builder /app/phish-server /opt/phish-and-discover/
COPY --from=app-builder /app/templates /opt/phish-and-discover/templates
COPY --from=app-builder /app/static /opt/phish-and-discover/static
COPY --from=asset-builder /build/static/js/dist /opt/phish-and-discover/static/js/dist
COPY --from=asset-builder /build/static/css/dist /opt/phish-and-discover/static/css/dist
COPY --from=app-builder /app/db /opt/phish-and-discover/migrations
COPY --from=app-builder /app/config.json /opt/phish-and-discover/config.json.example
COPY --from=app-builder /app/VERSION /opt/phish-and-discover/VERSION
COPY docker/docker-entrypoint.sh /opt/phish-and-discover/docker-entrypoint.sh

# Prepare directories
RUN mkdir -p /opt/phish-and-discover/db /home/phish/.nuclei-templates /home/phish/.config && \
    mkdir -p /home/phish/.config/subfinder /home/phish/.config/uncover /home/phish/.config/asnmap /home/phish/.config/interactsh-client && \
    chown -R phish:phish /opt/phish-and-discover /home/phish && \
    chmod +x /opt/phish-and-discover/docker-entrypoint.sh /opt/phish-and-discover/phish-server


# Set Linux capabilities for network operations
RUN set -eux; \
    [ -f /usr/local/bin/naabu ] && setcap cap_net_raw,cap_net_admin=eip /usr/local/bin/naabu || true; \
    [ -f /usr/local/bin/httpx ] && setcap cap_net_raw=eip /usr/local/bin/httpx || true; \
    [ -f /usr/local/bin/dnsx ] && setcap cap_net_raw=eip /usr/local/bin/dnsx || true; \
    [ -f /usr/local/bin/chisel ] && setcap cap_net_admin=eip /usr/local/bin/chisel || true; \
    [ -f /opt/phish-and-discover/phish-server ] && setcap cap_net_raw,cap_net_admin=eip /opt/phish-and-discover/phish-server || true

# Verify capabilities (non-fatal)
RUN set -eux; \
    [ -f /usr/local/bin/naabu ] && getcap /usr/local/bin/naabu || true; \
    [ -f /usr/local/bin/httpx ] && getcap /usr/local/bin/httpx || true; \
    [ -f /usr/local/bin/chisel ] && getcap /usr/local/bin/chisel || true; \
    [ -f /opt/phish-and-discover/phish-server ] && getcap /opt/phish-and-discover/phish-server || true

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD curl -f http://localhost:3333/ || exit 1

# Expose ports
# 3333: Gophish Admin & Phish & Discover Dashboard
# 80/443: HTTP(S) Phishing Redirector
# 8080: Chisel Reverse Tunnel Listener
# 9090: Reverse TUN Server
EXPOSE 3333 80 443 8080 9090

# Security context
USER phish
ENTRYPOINT ["/opt/phish-and-discover/docker-entrypoint.sh"]
CMD ["./phish-server"]

