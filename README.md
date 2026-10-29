# Phish & Discover (Gophish Security Operations Hub)

**Phish & Discover** is an advanced fork of **Gophish**, transformed into a unified Security Operations Hub. It acts as a powerful, asynchronous Go-based CLI wrapper and orchestrator for **ProjectDiscovery** tools, allowing security teams to manage phishing simulations alongside automated network scanning.

---

## 🖼️ Screenshots

### Unified Security Ops Dashboard
![Phish & Discover Dashboard](dashboard.jpg)

### Enterprise Docker Stack & Reverse Tunneling
![Phish & Discover Docker Architecture](docker.jpg)

---

## 🚀 Key Features

*   **Unified Dashboard**: Manage phishing campaigns and security scans from a single modern UI.
*   **ProjectDiscovery CLI Wrapper**: Fully integrates and orchestrates ProjectDiscovery tools (**Nuclei**, **Subfinder**, **HTTPx**, **Naabu**, **DNSx**, **Katana**, **TLSx**, **ASNMap**, **Uncover**) as an asynchronous scan engine.
*   **Reverse L3 Tunneling**: Perform internal network scans through a secure Chisel-based reverse tunnel.
*   **Real-time Insights Discovery**: WebSocket-based live scan logs and campaign performance tracking.
*   **Enterprise-Ready Deployment**: Orchestrated via Docker Compose with Caddy (HTTPS) and Postfix integration.

---

## 📚 Documentation

For detailed setup and usage instructions, please refer to the following guides:

*   📖 **[Phish & Discover Overview & API Reference](doc/README_PHISH_AND_DISCOVER.md)** - Main project documentation.
*   🏗️ **[Architecture](doc/ARCHITECTURE.md)** - Scanner engine design and where things live.
*   🚀 **[Deployment & Operations Guide](doc/DEPLOYMENT_GUIDE.md)** - Step-by-step VPS/Server deployment.
*   🌐 **[Reverse L3 Tunnel Guide](doc/REVERSE_TUNNEL_GUIDE.md)** - **[NEW]** Setup and usage for internal scanning.
*   📝 **[Changelog](doc/CHANGELOG.md)** - Notable changes and known open items.

## 🌐 Platform Architecture (Phishing & Scanning Workflows)

Phish & Discover operates from a **VPS with a Static IP bound to a domain via DNS records** (essential for phishing/oltalama delivery). It supports three main workflows:

1. **External Scanning (Direct from VPS)**: Scans public internet assets directly from the VPS.
2. **Internal Scanning (Compromised Endpoint Simulation)**: Scans are routed from the VPS through a secure Chisel reverse tunnel (`tun0`) to the Phish & Discover Agent running on a compromised endpoint inside the corporate network.
3. **Phishing Simulation**: Sends oltalama emails from the VPS to corporate users via Postfix SMTP, driving engagement tracking back to the dashboard.

```mermaid
graph TD
    %% Styling
    classDef vps fill:#eef2ff,stroke:#4f46e5,stroke-width:2px,color:#1e1b4b;
    classDef ext fill:#fff7ed,stroke:#ea580c,stroke-width:2px,color:#7c2d12;
    classDef internal fill:#f0fdf4,stroke:#16a34a,stroke-width:2px,color:#14532d;

    %% Nodes & Subgraphs
    subgraph VPS ["Phish & Discover VPS (Cloud Deployment)"]
        Core["Phish & Discover Hub Core<br>(Scanner & Phishing Engine)"]:::vps
        SMTP["Postfix SMTP Container"]:::vps
    end

    subgraph ExtNet ["Public Internet (External Zone)"]
        ExtAssets["External Assets<br>(Public IPs, Domains, Web Apps)"]:::ext
    end

    subgraph Corporate ["Corporate Network (Internal Zone)"]
        Agent["Phish & Discover Agent<br>(Compromised Endpoint Sim.)"]:::internal
        IntAssets["Internal Corporate Assets<br>(Active Directory, Local Servers)"]:::internal
        Users["Corporate Users<br>(Phishing Targets)"]:::internal
    end

    %% Workflows
    
    %% 1. External Scanning
    Core -->|1. Direct External Scan| ExtAssets
    
    %% 2. Internal Scanning (Compromised Endpoint Simulation)
    Core <-->|2a. Reverse Chisel Tunnel| Agent
    Agent -->|2b. Local Subnet Scan| IntAssets

    %% 3. Phishing Simulation
    Core -->|3a. Trigger Email Campaign| SMTP
    SMTP -->|3b. Phishing Emails via SMTP| Users
    Users -.->|3c. Phishing Interactions via HTTPS| Core

    %% Group styling
    style VPS fill:#f8fafc,stroke:#cbd5e1,stroke-width:2px
    style ExtNet fill:#fffbeb,stroke:#fef3c7,stroke-width:2px
    style Corporate fill:#f0fdf4,stroke:#bbf7d0,stroke-width:2px
```

---

## 🏗️ Quick Start

```bash
git clone https://github.com/ytalharabaci/phish-and-discover.git
cd phish-and-discover
cp .env.example .env
# Edit .env and start
docker compose up -d
```

Access your dashboard at `https://yourdomain.com/` (as configured in `.env`).

---

## 🍴 Fork Notice

Phish & Discover is a fork of [Gophish](https://github.com/gophish/gophish) by Jordan Wright, extended
with a ProjectDiscovery-based scanning engine and reverse-tunnel infrastructure. Gophish's
original MIT copyright notice is preserved in [LICENSE](./LICENSE) alongside Phish & Discover's own.

## ⚖️ License

This project extends **Gophish** (MIT License) and integrates **ProjectDiscovery** tools.
See [LICENSE](./LICENSE) for full details.

---

**Built with ❤️ for offensive security teams.**
