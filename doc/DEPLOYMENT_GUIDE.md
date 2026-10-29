# PHISH & DISCOVER Deployment & Operations Guide

## ⚡ Quick Start (Recommended)

Once Docker is installed (see Step 1) and DNS points at your server (Step 2):

```bash
cd /opt
sudo git clone https://github.com/ytalharabaci/phish-and-discover.git
sudo chown -R $USER:$USER PHISH & DISCOVER && cd PHISH & DISCOVER
./scripts/setup.sh
```

The script creates `.env` from `.env.example`, prompts for your two domains/Let's Encrypt email,
an optional Basic Auth password, and optional direct-mail-sending settings (`MAIL_HOSTNAME`/
`MAIL_DOMAIN` — see Step 6.5), generates the admin path secret and the initial Gophish admin
password, then runs `docker compose build && up -d` and prints the login URL/credentials when
done. Re-running it is safe — it only fills in values still left at their `.env.example` default.

PHISH & DISCOVER always runs targeting a real domain/VPS deployment — there is no separate "test mode";
all configuration happens through `.env` (see [.env.example](../.env.example)).

The rest of this guide covers the same steps manually, plus day-2 operations.

## 🚀 Initial Deployment (VPS/Cloud)

### Step 1: Server Preparation

**Minimum Requirements:**
- OS: Ubuntu 22.04 LTS or Debian 12
- CPU: 4 cores
- RAM: 8GB
- Storage: 100GB SSD
- Network: Public IP + domain

**System Setup:**

Official Docker Engine installation ([docs.docker.com/engine/install/ubuntu](https://docs.docker.com/engine/install/ubuntu/) —
use `/debian/` instead of `/ubuntu/` in the URLs below if you're on Debian).
The `get.docker.com` convenience script is fine for quick testing, but Docker's own docs say
not to rely on it for production — this is the supported path:

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Remove any conflicting older packages
sudo apt remove -y $(dpkg --get-selections docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc 2>/dev/null | cut -f1)

# Add Docker's official GPG key
sudo apt update
sudo apt install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

# Add the Docker apt repository (deb822 format)
sudo tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}")
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF
sudo apt update

# Install Docker Engine + the Compose v2 plugin
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# Let your user run docker without sudo (log out/in, or `newgrp docker`, to apply)
sudo usermod -aG docker $USER

# Verify installation
sudo docker run hello-world
docker compose version
```

`scripts/setup.sh` and the rest of this guide use `docker compose` (the v2 plugin, space, no
hyphen) — that's what `docker-compose-plugin` installs above.

### Step 2: Configure DNS

This is the complete list of every DNS (and rDNS) record PHISH & DISCOVER needs. Set them all up front —
the **App** records are always required; the **Mail** records only apply if you plan to send
phishing email directly from this server instead of through an external SMTP relay
(Gmail/SendGrid/etc.).

#### App records (always required)

PHISH & DISCOVER uses **two** domain roles, set independently in `.env` ([.env.example](../.env.example)):
- `ADMIN_DOMAIN` — the Gophish admin dashboard/API, hidden behind the `ADMIN_PATH_SECRET` path.
- `DOMAIN` (mapped to `PHISH_DOMAIN` in Caddy) — public-facing phishing/landing pages, no auth.

Use **two different subdomains** for these — Caddy defines them as separate site blocks, and
pointing both at the exact same hostname makes Caddy fail to start with an "ambiguous site
definition" error:

| Record | Type | Value | Notes |
| --- | --- | --- | --- |
| `admin.example.com` | A | `<vps-ip>` | Admin dashboard / API — `ADMIN_DOMAIN` |
| `phish.example.com` | A | `<vps-ip>` | Phishing & landing pages — `DOMAIN` |

If you're behind Cloudflare (or another proxying DNS host), set both records to **DNS only**
(grey cloud, not proxied). Caddy issues its own Let's Encrypt certificate via the HTTP-01
challenge and needs to reach your server directly — an orange-cloud proxy in front of it adds
a second TLS hop that isn't required and complicates certificate issuance.

#### Mail records (only if sending mail directly, no external relay)

| Record | Type | Value | Where it's set | Notes |
| --- | --- | --- | --- | --- |
| `<vps-ip>` reverse | PTR/rDNS | `mail.example.com` | VPS provider's panel, **not** Cloudflare | Must match `MAIL_HOSTNAME` |
| `mail.example.com` | A | `<vps-ip>` | DNS provider, **DNS only** | Matches `MAIL_HOSTNAME` |
| `example.com` | TXT | `v=spf1 ip4:<vps-ip> ~all` | DNS provider, **DNS only** | SPF |
| `_dmarc.example.com` | TXT | `v=DMARC1; p=none; rua=mailto:postmaster@example.com` | DNS provider, **DNS only** | `p=none` for testing, tighten later |
| `mail._domainkey.example.com` | TXT | *(generated after the stack is up — see Step 6.5)* | DNS provider, **DNS only** | DKIM |

Outbound port 25 must also be open on the VPS for direct sending to work — many providers block
it by default; see Step 6.5 for how to test it. Full walkthrough of all five mail records,
including how to fetch the DKIM value, is in **Step 6.5: Mail (Direct Sending)** below.

### Step 3: Clone & Prepare

```bash
cd /opt
sudo git clone https://github.com/ytalharabaci/phish-and-discover.git
cd PHISH & DISCOVER

# Set proper permissions
sudo chown -R $USER:$USER .

# Copy environment template
cp .env.example .env
```

### Step 4: Generate Caddy Password Hash

```bash
# Option 1: Online (quick)
# Visit: https://caddyserver.com/docs/caddyfile/directives/basicauth

# Option 2: Local Docker
docker run --rm caddy caddy hash-password "your-secure-password"
# Output: $2a$14$R9h/cIPz0gi.URNNX3kh2OPST9/PgBkqquzi.Ee3D6xVxQo0kK7dm
```

### Step 5: Configure .env

```bash
nano .env
```

Edit accordingly:
```bash
# Public-facing phishing/landing pages and the admin dashboard — use two
# different subdomains (see Step 2) to avoid a Caddy "ambiguous site" conflict.
DOMAIN=phish.example.com
ADMIN_DOMAIN=admin.example.com

CADDY_EMAIL=you@example.com
ADMIN_PASS_HASH=$2a$14$R9h/cIPz0gi.URNNX3kh2O...

# Direct mail sending (no external relay) — see "Step 6.5: Mail (Direct Sending)" below
MAIL_HOSTNAME=mail.example.com
MAIL_DOMAIN=example.com
```

> PHISH & DISCOVER doesn't ship a built-in Tailscale sidecar — `docker-compose.yml` has no Tailscale
> service to uncomment. If you need VPN-reachable internal scanning, use the Chisel-based
> reverse tunnel that's already built in (Step 8 below / [REVERSE_TUNNEL_GUIDE.md](REVERSE_TUNNEL_GUIDE.md)),
> or add your own Tailscale sidecar container.

### Step 6: Start Services

```bash
# Build image (includes ProjectDiscovery tools)
docker compose build

# Start all services
docker compose up -d

# Wait for health checks
sleep 10
docker compose ps

# Expected output:
# phish_core    running (healthy)
# phish_postfix running
# phish_caddy   running
```

### Step 6.5: Mail (Direct Sending)

If `MAIL_HOSTNAME`/`MAIL_DOMAIN` are set, Postfix sends mail straight to the internet — no
external relay (Gmail/SendGrid/etc.) involved. The PTR, A, SPF, and DMARC records from the
**Mail records** table in Step 2 should already be in place; this needs a couple more things
to actually land in inboxes instead of spam folders or being rejected outright:

1. **Outbound port 25 must be open.** Many VPS providers block it by default to fight spam.
   Check with your provider's control panel/support, or test from the host once it's up:
   ```bash
   docker compose exec postfix sh -c "echo QUIT | nc -w5 smtp.gmail.com 25"
   ```

2. **DKIM**: after the stack is up, fetch the generated public key and add it as the
   `mail._domainkey.example.com` TXT record from Step 2's table:
   ```bash
   docker compose exec postfix sh -c 'cat /etc/opendkim/keys/*/*.txt'
   ```
   This prints a record like `mail._domainkey IN TXT ( "v=DKIM1; ..." )` — create a TXT record
   named `mail._domainkey.example.com` in Cloudflare with that value (DNS only). The keys are
   stored in the `opendkim_keys` Docker volume, so they survive restarts and the DNS record
   stays valid.

3. In Gophish, set the SMTP sending profile's "From" address to `...@example.com` (matching
   `MAIL_DOMAIN`) — DKIM only signs/passes if the From domain matches.

### Step 7: Initial Access

```bash
# Check Caddy status
docker compose logs caddy -f --tail=20
# Wait for "Serving HTTPS..." message
```

`ADMIN_DOMAIN` is hidden behind `ADMIN_PATH_SECRET` (a WAF rule in the Caddyfile 404s every
request that doesn't carry the cookie set by that path). To log in:

1. Visit `https://admin.example.com/<ADMIN_PATH_SECRET>` (value from your `.env`) once in a
   browser — this sets a 30-day cookie and redirects you to `/`.
2. If `ADMIN_PASS_HASH` is set (Step 4), the browser will also prompt for **Caddy** Basic Auth:
   username is the fixed `phish-admin` (not `admin` — set in `Caddyfile`), password is what
   you hashed.
3. You'll land on the Gophish login page. Username: `admin`. Password: the
   `GOPHISH_INITIAL_ADMIN_PASSWORD` value `setup.sh` printed at the end (or
   `docker compose logs phish-core | grep -i password` if you set it up manually).

### Step 8: Reverse Tunnel Configuration (Optional)

PHISH & DISCOVER includes a built-in Chisel-based reverse tunnel server on port **9090**.

1. **Verify exposure**: Port `9090` (`HOST_CHISEL_PORT`) must be reachable for reverse agents,
   or use option 2 to tunnel it through Caddy on 443 instead.
2. **Caddy Proxying (Recommended)**: add the route to the **existing** `{$ADMIN_DOMAIN}` block
   in `Caddyfile` — don't create a new top-level block for `{$DOMAIN}`/`{$ADMIN_DOMAIN}`, Caddy
   already has one for each and a second one with the same host fails to start. You also need
   to exempt `/chisel/*` from the admin-obfuscation WAF rule, otherwise it 404s before reaching
   the proxy:
   ```caddy
   {$ADMIN_DOMAIN:localhost} {
       # ...
       route {
           @unauthorized {
               not path /{$ADMIN_PATH_SECRET}* /chisel/*
               not header Cookie *phish_access=authorized*
           }
           respond @unauthorized "Not Found" 404
       }

       handle /chisel/* {
           reverse_proxy phish-core:9090
       }

       reverse_proxy phish-core:3333 {
           # ... existing headers
       }
   }
   ```

3. **Client Connection**:
   ```bash
   chisel client https://admin.example.com/chisel R:tun0:0.0.0.0
   ```

[Detailed Guide: REVERSE_TUNNEL_GUIDE.md](REVERSE_TUNNEL_GUIDE.md) — note its Caddy snippet
predates the admin/phish domain split above; the steps here are the up-to-date version.

---

## 📋 Post-Deployment Configuration

### 1. Configure Gophish Admin User

See Step 7 above for the secret-path/cookie + login flow. Once logged in, change the admin
password from the generated one (Gophish prompts for this on first login).

### 2. Create SMTP Profile (for phishing emails)

Via UI, or via API once you have the `phish_access` cookie (see Step 7) and the Gophish API
key (Settings → API Key in the dashboard):
```bash
curl -X POST https://admin.example.com/api/smtp/ \
  -b "phish_access=authorized" \
  -H "Content-Type: application/json" \
  -H "X-API-KEY: your-gophish-api-key" \
  -d '{
    "name": "Local Postfix",
    "host": "postfix:25",
    "from_address": "security@yourdomain.com",
    "username": "",
    "password": "",
    "ignore_cert_errors": false
  }'
```

### 3. Import Email Lists & Groups

Upload CSV to Gophish UI for target groups.

### 4. Create Email Templates

Use Gophish UI or import custom HTML.

### 5. Create Landing Pages

Capture credentials or track interactions.

---

## 🔧 Operations & Monitoring

### Health Checks

```bash
# Check all services
docker compose ps

# Verify the WAF rule is hiding the admin domain (no cookie/secret path supplied)
curl -I https://admin.example.com/ -k
# Expected: 404 Not Found

# With the secret path it redirects, and 401s if ADMIN_PASS_HASH (Caddy Basic Auth) is set
curl -I https://admin.example.com/<ADMIN_PATH_SECRET> -k
# Expected: 302 redirect to / (plus Set-Cookie: phish_access=authorized)

# Check scanner connectivity (bypasses Caddy — talks to phish-core directly on the VPS)
curl http://localhost:3333/api/scanner/status \
  -H "X-API-KEY: your-api-key"
# Expected: {"running":false}
```

### Log Monitoring

```bash
# Real-time logs (follow mode)
docker compose logs -f

# Specific service logs
docker compose logs -f phish-core
docker compose logs -f caddy --tail=50
docker compose logs -f postfix

# Date-filtered logs
docker compose logs --since 2026-04-07T10:30:00 phish-core
```

### Database Backup

```bash
# Backup Gophish database
docker compose exec phish-core cp /opt/phish-and-discover/db/phish.db /opt/phish-and-discover/db/PHISH & DISCOVER-backup-$(date +%Y%m%d-%H%M%S).db

# Copy from container to host
docker cp phish_core:/opt/phish-and-discover/db/phish.db ./backup/PHISH & DISCOVER-$(date +%Y%m%d).db

# Restore (if needed)
docker compose down
docker cp ./backup/PHISH & DISCOVER-2026-04-07.db phish_core:/opt/phish-and-discover/db/phish.db
docker compose up -d
```

### Automated Backups (Optional)

```bash
# Create cron job for daily backup
crontab -e

# Add line:
0 2 * * * cd /opt/phish-and-discover && docker compose exec -T phish-core cp /opt/phish-and-discover/db/phish.db ./backups/PHISH & DISCOVER-$(date +\%Y\%m\%d).db
```

---

## 🔄 Scanning Operations

### Start a Scan via Dashboard

1. Navigate to **Scanner** tab
2. Enter target (domain, IP, or CIDR)
3. Select tool or enable Discovery Mode
4. Click **START SCAN**
5. Watch real-time logs

### Start a Scan via API

All examples below go through the public `admin.example.com` domain, so they need the WAF
cookie from Step 7 (`-b "phish_access=authorized"`); add `-u phish-admin:your-caddy-password`
too if `ADMIN_PASS_HASH` is set. Running from the VPS itself, you can skip both and hit
`http://localhost:3333/...` directly instead, as in the Health Checks example above.

```bash
# Single tool scan
curl -X POST https://admin.example.com/api/scanner/start \
  -b "phish_access=authorized" \
  -H "Content-Type: application/json" \
  -H "X-API-KEY: your-api-key" \
  -d '{
    "target": "acme.com",
    "tool": "nuclei",
    "flags": ["-severity", "critical,high"]
  }'

# Discovery mode (full chain)
curl -X POST https://admin.example.com/api/scanner/start \
  -b "phish_access=authorized" \
  -H "Content-Type: application/json" \
  -H "X-API-KEY: your-api-key" \
  -d '{
    "target": "example.com",
    "discovery_mode": true
  }'
```

### Query Findings

```bash
# All findings
curl https://admin.example.com/api/scanner/findings \
  -b "phish_access=authorized" \
  -H "X-API-KEY: your-api-key"

# Critical findings only
curl 'https://admin.example.com/api/scanner/findings?severity=critical' \
  -b "phish_access=authorized" \
  -H "X-API-KEY: your-api-key"

# Filter by tool and limit
curl 'https://admin.example.com/api/scanner/findings?tool=nuclei&severity=high&limit=10' \
  -b "phish_access=authorized" \
  -H "X-API-KEY: your-api-key"
```

### Advanced Scanning Scenarios

#### Scenario 1: Internal Network Scanning

There's no built-in Tailscale sidecar to "uncomment" — `docker-compose.yml` doesn't ship one.
The supported way to reach internal/NATed networks is the Chisel-based reverse tunnel that's
already wired into `phish-core` (Step 8 above): an agent on the internal network connects
out to your VPS, a `tun0` interface appears, and you route internal CIDRs (`10.0.0.0/8`,
`172.16.0.0/12`, etc.) through it. Full walkthrough: [REVERSE_TUNNEL_GUIDE.md](REVERSE_TUNNEL_GUIDE.md).

#### Scenario 2: Bulk Domain Scanning

```bash
# Create domains.txt
cat > domains.txt << EOF
example.com
acme.com
test.io
EOF

# Scan each domain with Nuclei
for domain in $(cat domains.txt); do
  curl -X POST https://admin.example.com/api/scanner/start \
    -b "phish_access=authorized" \
    -H "Content-Type: application/json" \
    -H "X-API-KEY: your-api-key" \
    -d '{
      "target": "'$domain'",
      "tool": "nuclei"
    }'
  sleep 5  # Rate limit
done
```

#### Scenario 3: Export Findings to CSV

```bash
# Query findings as JSON, convert to CSV
curl https://admin.example.com/api/scanner/findings \
  -b "phish_access=authorized" \
  -H "X-API-KEY: your-api-key" | \
  jq -r '.[] | [.severity, .tool_name, .name, .target] | @csv' > findings.csv
```

---

## 🛡️ Security Operations

### Regular Tasks

#### Weekly
- [ ] Review scan logs for anomalies
- [ ] Check Caddy certificate validity (auto-renewed by Let's Encrypt)
- [ ] Monitor disk usage: `df -h`
- [ ] Update ProjectDiscovery tools — the binaries are installed at image build time, not via a
  writable container path. If CI/CD is set up (`.github/workflows/deploy.yml`, builds on GHCR
  and the VPS only pulls — see below), trigger a rebuild from GitHub Actions
  (`workflow_dispatch`) rather than building on the VPS itself. Without CI/CD, build locally on
  the VPS:
  ```bash
  docker compose build --no-cache phish-core && docker compose up -d
  ```

#### Monthly
- [ ] Backup databases
- [ ] Review Gophish campaign results
- [ ] Rotate API keys (regenerate in Gophish UI)
- [ ] Update Docker images: `docker compose pull && docker compose up -d`

#### Quarterly
- [ ] Security audit of configurations
- [ ] Check sending IP against DNSBLs (e.g. mxtoolbox.com blacklist check) and DMARC reports
- [ ] Assess storage usage and retention policy

### Incident Response

**Scan Stuck/Frozen:**
```bash
# Check status
docker compose exec phish-core curl http://localhost:3333/api/scanner/status

# If running=true but appears hung, restart:
docker compose restart phish-core
```

**High Memory Usage:**
```bash
# Monitor memory
docker stats phish_core

# Limit memory in docker-compose.yml:
deploy:
  resources:
    limits:
      memory: 2G  # Add this
```

**Phishing Emails Not Delivering:**
```bash
# Check Postfix queue
docker compose exec postfix postqueue -p

# View Postfix logs
docker compose logs postfix | grep error

# Force queue flush
docker compose exec postfix postfix flush
```

If the queue is clear but mail still isn't arriving, see **Step 6.5: Mail (Direct Sending)** —
most delivery failures with direct sending are outbound port 25 being blocked by the VPS
provider, a missing/mismatched PTR record, or no SPF/DKIM record yet.

---

## 📊 Reporting & Analytics

### Via Dashboard
1. Go to **Findings** tab
2. Filter by severity/tool
3. Export or screenshot

### Via API - Generate Report

```bash
#!/bin/bash
# Generate markdown report

echo "# PHISH & DISCOVER Security Report - $(date +%Y-%m-%d)"
echo ""

# Stats
curl -s https://admin.example.com/api/scanner/stats \
  -b "phish_access=authorized" \
  -H "X-API-KEY: $API_KEY" | \
  jq 'to_entries | .[] | "- \(.key): \(.value)"'

echo ""
echo "## Critical Findings"
curl -s "https://admin.example.com/api/scanner/findings?severity=critical" \
  -b "phish_access=authorized" \
  -H "X-API-KEY: $API_KEY" | \
  jq '.[] | "- [\(.name)](\(.target)) - \(.detail)"'
```

### Custom Webhook for Notifications

Add to Gophish config.json:
```json
"webhooks": {
  "url": "https://your-webhook-service.com/gophish",
  "secret": "webhook-secret"
}
```

---

## 🔐 Security Hardening Checklist

- [ ] Change default Gophish API key
- [ ] Enable HTTPS with valid certificates (Let's Encrypt via Caddy)
- [ ] Set `ADMIN_PASS_HASH` for Caddy Basic Auth in front of the admin domain
- [ ] Restrict Docker port exposure (only 80/443/9090 public; admin port 3333 stays on 127.0.0.1)
- [ ] If sending mail directly: SPF, DKIM, DMARC (`p=quarantine`/`reject` once confirmed working) and a matching PTR record
- [ ] Use the Chisel reverse tunnel (not exposing internal IPs directly) for internal network scanning
- [ ] Implement rate limiting for API
- [ ] Setup audit logs (Caddy logging)
- [ ] Regular backups (encrypted storage)
- [ ] Monitor for unauthorized access logs (404s against `ADMIN_DOMAIN` from the WAF rule are expected noise — bots probing; repeated hits on the real secret path are the signal to watch)

---

## 🆘 Troubleshooting

### Issue: Can't Access Dashboard

```bash
# Check if services running
docker compose ps

# If not, start:
docker compose up -d

# Check Caddy logs for routing errors
docker compose logs caddy
```

### Issue: Scanner Not Working

```bash
# Verify tools are installed
docker compose exec phish-core which nuclei subfinder httpx

# If missing: binaries are baked in at build time, not installable at runtime.
# With CI/CD (image pulled from GHCR): re-run the GitHub Actions workflow, then on the VPS:
docker compose pull phish-core && docker compose up -d
# Without CI/CD (building locally on the VPS):
docker compose build --no-cache phish-core && docker compose up -d

# Check scanner status
curl http://localhost:3333/api/scanner/status
```

### Issue: Phishing Emails Not Sent

```bash
# Verify Postfix is running
docker compose logs postfix

# Check mail queue
docker compose exec postfix postqueue -p

# Test SMTP connectivity (the runtime image has no telnet; use bash's /dev/tcp instead)
docker compose exec phish-core bash -c 'exec 3<>/dev/tcp/postfix/25 && cat <&3'
```

### Issue: WebSocket Connection Failed

Caddy's `reverse_proxy` handles WebSocket upgrades automatically in v2 — this is almost always
a missing/incorrect `Host` header from a client, or another proxy in front of Caddy stripping
the `Upgrade`/`Connection` headers. Check:
```bash
docker compose logs caddy --tail=50 | grep -i upgrade
```
If you're behind Cloudflare proxied (orange cloud) rather than DNS only, that's also a common
cause — Cloudflare supports WebSocket but adds another hop that can interfere; switch the
record to DNS only (see Step 2) to rule it out.

### Issue: Basic Auth Not Working
- The Caddy Basic Auth username is the fixed `phish-admin` (set in `Caddyfile`), not `admin` —
  that's the separate Gophish login.
- Regenerate password hash:
  ```bash
  docker run --rm caddy caddy hash-password "newpassword"
  ```
- Update `ADMIN_PASS_HASH` in `.env` with the new hash.
- Restart Caddy: `docker compose restart caddy`

### Issue: Reverse Tunnel (tun0) Not Appearing
- Ensure PHISH & DISCOVER container has `cap_add: NET_ADMIN, NET_RAW` and `/dev/net/tun` device mapped
  (already set in `docker-compose.yml`; don't remove them).
- Restart: `docker compose up -d --force-recreate`.
- Check if Chisel server is started in the Dashboard (Scanner tab).
- See [REVERSE_TUNNEL_GUIDE.md](REVERSE_TUNNEL_GUIDE.md) for full steps.

---

## 📚 Additional Resources

- **Gophish Docs**: https://getgophish.com/
- **ProjectDiscovery**: https://projectdiscovery.io/
- **Docker Compose**: https://docs.docker.com/compose/
- **Caddy Documentation**: https://caddyserver.com/docs/
- **Tailwind CSS**: https://tailwindcss.com/

---

**Deployment Date:** ________________  
**Last Updated:** ________________  
**Backup Location:** ________________  
**Support Contact:** ________________

---

**Next Steps:**
1. Complete deployment checklist
2. Test scanner with sample domain
3. Configure SMTP for phishing campaigns
4. Schedule regular backups
5. Monitor logs for errors
