# 🐳 PHISH & DISCOVER Docker Deployment Guide

## Quick Start

### 1. Build & Run with Docker Compose (Recommended)

```bash
# Clone the repository
git clone https://github.com/your-org/gophish-PHISH & DISCOVER.git
cd gophish-PHISH & DISCOVER

# Build and start all services
docker-compose up -d --build

# Verify everything is running
docker-compose ps

# View logs
docker-compose logs -f phish-core
```

### 2. Access PHISH & DISCOVER

- **Dashboard**: http://localhost:3333
- **Reverse Tunnel Listener**: localhost:9090 (for chisel agents)
- **Phishing HTTP Listener**: http://localhost:80
- **Phishing HTTPS Listener**: https://localhost:443

---

## Architecture Overview

### Multi-Stage Dockerfile

The Dockerfile uses 4 build stages to create an optimized production image:

1. **Asset Builder** (Node.js)
   - Minifies JavaScript and CSS
   - Reduces frontend bundle size

2. **ProjectDiscovery Tools Builder** (Go 1.21)
   - Compiles 11 security tools
   - Subfinder, Httpx, Nuclei, Naabu, DNSx, Katana, TLSx, Asnmap, Uncover, Interactsh, Chisel
   - All tools pinned to stable versions

3. **PHISH & DISCOVER App Builder** (Go 1.21)
   - Compiles Gophish + PHISH & DISCOVER backend
   - Binary stripping for size optimization

4. **Production Runtime** (Debian Bullseye Slim)
   - ~1.2GB final image (compared to 4GB+ without multi-stage)
   - Only runtime dependencies included
   - Linux capabilities configured for network operations

### Docker Compose Services

#### phish-core
- **Role**: Main application server (Gophish + Scanner Engine)
- **Capabilities**: NET_ADMIN, NET_RAW (required for TUN/TAP and raw sockets)
- **Volumes**: 
  - `./phish_data`: SQLite database persistence
  - `nuclei_templates`: Cached Nuclei templates (survives restarts)
- **Ports**: 3333 (Admin), 80 (HTTP), 443 (HTTPS), 9090 (Chisel)
- **Health Check**: HTTP endpoint verification every 30s

#### postfix
- **Role**: SMTP relay for phishing email delivery
- **Isolation**: Internal network only (no external ports)
- **Configuration**: Accepts local relay from phish-core

#### caddy
- **Role**: Reverse proxy with TLS termination
- **Features**: Auto-HTTPS via Let's Encrypt, Basic Auth support
- **Volume**: `caddy_data` for certificate persistence

---

## Docker Networking

All services communicate via internal Docker network `phish-net`:

```
Subnet: 172.20.0.0/16

phish-core (172.20.0.2)
  ├─-> postfix:25 (internal SMTP)
  └─-> caddy (reverse proxy)

postfix (172.20.0.3)
  └─-> phish-core (mail delivery)

caddy (172.20.0.4)
  └─-> phish-core:3333 (backend)
```

---

## Security Considerations

### Linux Capabilities

The container runs with restricted capabilities for security:

```yaml
cap_add:
  - NET_ADMIN    # TUN/TAP creation (Chisel tunneling)
  - NET_RAW      # Raw packet access (Naabu, httpx)
  - SYS_PTRACE   # Process debugging (stress tests)

cap_drop:
  - ALL          # Drop all others
```

### Resource Limits

```yaml
limits:
  cpus: '4'      # Max 4 CPU cores
  memory: 4G     # Max 4GB RAM
```

### Volume Mounts

- `./phish_data`: Read-write (database)
- `nuclei_templates`: Read-write (template cache)
- `./config.json`: Read-only (configuration)

---

## Advanced Configuration

### Custom Configuration File

Create `config.json` in the project root:

```json
{
  "admin_server": {
    "listen_url": "0.0.0.0:3333",
    "use_tls": false
  },
  "phish_server": {
    "listen_url": "0.0.0.0:80",
    "use_tls": false
  },
  "db": {
    "name": "sqlite3",
    "path": "/opt/phish-and-discover/db/phish.db"
  }
}
```

### Environment Variables

```bash
# Database configuration
export DB_TYPE=sqlite3
export DB_PATH=/opt/phish-and-discover/db/phish.db

# SMTP relay
export SMTP_HOST=postfix
export SMTP_PORT=25

# Logging
export LOG_LEVEL=info

# Chisel tunnel server
export CHISEL_SERVER_PORT=9090
```

### Using External SMTP

For production email delivery:

```yaml
  postfix:
    environment:
      - POSTFIX_relayhost=[smtp.gmail.com]:587
      - POSTFIX_sasl_auth_enable=yes
      - POSTFIX_sasl_password_maps=hash:/etc/postfix/sasl_passwd
```

---

## Persistence & Backups

### SQLite Database

Located at: `./phish_data/phish.db`

**Backup**:
```bash
# Create backup
docker-compose exec phish-core cp /opt/phish-and-discover/db/phish.db /opt/phish-and-discover/db/phish.db.backup

# Download to host
docker cp phish_core:/opt/phish-and-discover/db/phish.db ./backup/PHISH & DISCOVER-$(date +%Y%m%d).db.backup
```

### Nuclei Templates

Located at: `nuclei_templates` volume

These are downloaded on first nuclei execution and cached for future runs. No backup needed (can be re-downloaded).

---

## Troubleshooting

### 1. Service Fails to Start

```bash
# Check logs
docker-compose logs phish-core

# Verify capabilities
docker-compose exec phish-core grep CapEff /proc/self/status

# Check if /dev/net/tun is accessible
docker-compose exec phish-core [ -c /dev/net/tun ] && echo "OK" || echo "Missing"
```

### 2. Chisel Tunnel Not Working

Requires NET_ADMIN capability:
```bash
# Verify
docker-compose exec phish-core getpcaps 1 | grep cap_net_admin

# If missing, ensure docker-compose.yml has:
# cap_add:
#   - NET_ADMIN
# devices:
#   - /dev/net/tun:/dev/net/tun
```

### 3. Port Forwarding Issues

```bash
# Check port bindings
netstat -tlnp | grep 3333

# If port already in use, modify docker-compose.yml:
# ports:
#   - "3334:3333"  # Map to different host port
```

### 4. Nuclei Template Download Fails

```bash
# Manually trigger Nuclei to download templates
docker-compose exec phish-core nuclei -update-templates

# Check cached templates
docker-compose exec phish-core ls -la /root/.nuclei-templates/
```

---

## Performance Monitoring

### Resource Usage

```bash
# Monitor container stats in real-time
docker stats phish_core --no-stream

# Or continuously
watch docker stats phish_core
```

### Logs

```bash
# Follow logs with timestamps
docker-compose logs -f --timestamps phish-core

# View last 100 lines
docker-compose logs --tail=100 phish-core

# Export logs for analysis
docker-compose logs phish-core > PHISH & DISCOVER-logs.txt
```

---

## Scaling & Production Deployment

### Kubernetes (Optional)

For enterprise deployments:

```bash
# Convert docker-compose to Kubernetes
kompose convert -f docker-compose.yml -o k8s/

# Deploy to Kubernetes
kubectl apply -f k8s/
```

### Docker Swarm

```bash
# Initialize swarm
docker swarm init

# Deploy stack
docker stack deploy -c docker-compose.yml PHISH & DISCOVER
```

---

## Maintenance

### Update Images

```bash
# Pull latest base images
docker-compose pull

# Rebuild with new images
docker-compose up -d --build
```

### Clean Up

```bash
# Stop services
docker-compose down

# Remove volumes (careful! deletes data)
docker-compose down -v

# Remove unused Docker resources
docker system prune -a
```

---

## Support & Documentation

- **GitHub**: https://github.com/your-org/gophish-PHISH & DISCOVER
- **Issues**: https://github.com/your-org/gophish-PHISH & DISCOVER/issues
- **ProjectDiscovery Docs**: https://docs.projectdiscovery.io
