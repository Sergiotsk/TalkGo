# VPS Setup Guide

Deploy TalkGo to a single Hetzner CX11 (or equivalent) using Docker Compose.
Estimated time: 20–30 minutes.

---

## Prerequisites

Before starting, make sure the VPS has:

- **Docker 24+** and **Docker Compose v2** installed
  ```bash
  docker --version        # Docker version 24.x or later
  docker compose version  # Docker Compose version v2.x or later
  ```
- **Open ports** — configure via Hetzner Firewall or `ufw`:

  | Port | Protocol | Purpose |
  |------|----------|---------|
  | 80   | TCP      | HTTP (Caddy ACME challenge + redirect) |
  | 443  | TCP      | HTTPS (Caddy TLS termination) |
  | 3478 | UDP + TCP | TURN/STUN (Coturn) |
  | 5349 | TCP      | TURN over TLS (Coturn) |
  | 49152–49200 | UDP | WebRTC media relay range |

  Example with `ufw`:
  ```bash
  ufw allow 80/tcp
  ufw allow 443/tcp
  ufw allow 3478/udp
  ufw allow 3478/tcp
  ufw allow 5349/tcp
  ufw allow 49152:49200/udp
  ufw enable
  ```

---

## Step 1: Get your domain

TalkGo uses [sslip.io](https://sslip.io) — a free wildcard DNS service that maps hostnames to IPs with no registration needed.

Construct your domain from the VPS public IP:

1. Get the VPS IP from the Hetzner console (e.g., `45.123.45.67`)
2. Replace dots with dashes: `45-123-45-67`
3. Append `.sslip.io`: `45-123-45-67.sslip.io`

That's your domain. No registration, no DNS propagation wait — it works immediately.

**Verify it resolves:**
```bash
ping 45-123-45-67.sslip.io
# Should resolve to 45.123.45.67
```

---

## Step 2: Clone and configure

```bash
git clone https://github.com/Sergiotsk/TalkGo.git
cd TalkGo
cp .env.example .env
```

Edit `.env` and fill in the required values:

```bash
nano .env
```

Required fields:

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | Your OpenAI API key (required — server won't start without it) |
| `TURN_PASSWORD` | Password for Coturn — choose a strong random string |
| `DOMAIN` | Your sslip.io domain, e.g. `45-123-45-67.sslip.io` |
| `ACME_EMAIL` | Your email for Let's Encrypt expiry notifications |

Example filled `.env`:
```env
OPENAI_API_KEY=sk-proj-...
TURN_PASSWORD=s3cr3t-turn-p4ss
DOMAIN=45-123-45-67.sslip.io
ACME_EMAIL=you@example.com
```

---

## Step 3: Configure Coturn

Edit the Coturn config to set the same TURN password:

```bash
nano deploy/coturn/turnserver.conf
```

Find the line:
```
user=talkgo:CHANGEME_TURN_PASSWORD
```

Replace `CHANGEME_TURN_PASSWORD` with the same value you used for `TURN_PASSWORD` in `.env`:
```
user=talkgo:s3cr3t-turn-p4ss
```

> The password must match exactly in both places — Coturn uses this for credential validation.

---

## Step 4: Start services

```bash
docker compose up -d
```

Verify all three services are running:

```bash
docker compose ps
```

Expected output:
```
NAME              IMAGE                  STATUS
talkgo-talkgo-1   talkgo-talkgo          Up X seconds
talkgo-coturn-1   coturn/coturn:latest   Up X seconds
talkgo-caddy-1    caddy:2-alpine         Up X seconds
```

All three should show `Up`. If any shows `Exit`, check logs (see Troubleshooting).

---

## Step 5: Smoke test

Wait ~60 seconds for Caddy to provision the TLS certificate, then:

```bash
curl https://45-123-45-67.sslip.io/health
```

Expected response:
```json
{
  "status": "ok",
  "turn_configured": true,
  "api_key_present": true,
  "codec_mode": "opus"
}
```

- `status: "ok"` — server is running
- `turn_configured: true` — TURN env vars are set
- `api_key_present: true` — OpenAI key is configured
- `codec_mode: "opus"` — real Opus codec is active

---

## Step 6: Verify TURN

Check that Coturn is reachable on UDP port 3478:

```bash
# Quick connectivity check (press Ctrl+C after 2-3 seconds)
nc -u 45.123.45.67 3478
```

If `turnutils_uclient` is available on your local machine (from `coturn` package):

```bash
turnutils_uclient -u talkgo -w s3cr3t-turn-p4ss -p 3478 45.123.45.67
```

A successful run shows `TURN session established` in the output.

---

## Troubleshooting

**Certificate not provisioned (curl returns SSL error)**

Caddy needs to reach Let's Encrypt. Wait 60–90 seconds after `docker compose up -d`, then retry. Check Caddy logs:
```bash
docker compose logs caddy
```
Look for `certificate obtained successfully`. If you see `challenge failed`, verify port 80 is open in your firewall.

**TURN not reachable**

- Check that UDP 3478 is open: `ufw status` should show `3478/udp ALLOW`
- Verify Coturn started correctly: `docker compose logs coturn`
- Confirm the password in `turnserver.conf` matches `TURN_PASSWORD` in `.env`

**Service won't start (Exit status)**

```bash
docker compose logs talkgo
```

Common causes:
- `OPENAI_API_KEY` is missing or empty → set it in `.env`
- Port already in use → stop any other service using port 80/443/3478

**View all logs at once**

```bash
docker compose logs -f
```

**Restart a single service**

```bash
docker compose restart talkgo
```
