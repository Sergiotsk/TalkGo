# TalkGo Network Testing Toolkit

Herramientas para simular condiciones de red realistas y ejecutar tests de rendimiento
end-to-end en TalkGo.

---

## Prerequisites

| Tool | Linux | Windows |
|------|-------|---------|
| Go 1.23+ | `go version` | `go version` |
| `iproute2` (tc) | `tc -V` | — (usar WSL2) |
| `jq` | `jq --version` | `choco install jq` |
| `netsh` | — | incluido en Windows |
| `curl` | `curl --version` | `curl --version` |
| Administrator | `sudo` | Run as Administrator |
| PowerShell 7+ | — | `pwsh --version` |

Instalación rápida (Debian/Ubuntu):

```bash
sudo apt-get install -y iproute2 jq curl
```

---

## Network Profiles

Los perfiles de red están en `configs/` como YAML:

| Profile | File | RTT | Loss | Bandwidth | Jitter | Escenario |
|---------|------|-----|------|-----------|--------|-----------|
| `4g` | `configs/4g.yml` | 100ms | 5% | 10 Mbps | 10ms | Red móvil 4G típica |
| `wifi-cafe` | `configs/wifi-cafe.yml` | 150ms | 8% | 5 Mbps | 20ms | WiFi público congestionado |
| `wifi-home` | `configs/wifi-home.yml` | 20ms | 1% | 50 Mbps | 2ms | WiFi hogar ideal |
| `wan-lossy` | `configs/wan-lossy.yml` | 300ms | 15% | 2 Mbps | 30ms | WAN con pérdida severa |

---

## Simulation Scripts

### Linux: `simulate-4g.sh`

Aplica perfiles de red usando `tc` (qdisc + netem).

```bash
# Aplicar perfil WiFi hogar
./simulate-4g.sh -Profile wifi-home

# Perfil específico con parámetros explícitos
./simulate-4g.sh -Interface eth0 -LatencyMs 100 -LossPct 5

# Listar perfiles disponibles
./simulate-4g.sh -ShowProfiles

# Resetear todas las reglas tc
./simulate-4g.sh -Reset

# Ver configuración actual
./simulate-4g.sh -Status
```

Requiere `sudo` para ejecutar comandos `tc`.

### Windows: `simulate-4g.ps1`

Aplica restricciones de ancho de banda vía `netsh` y `advfirewall`.

```powershell
# Requiere: Run as Administrator

# Aplicar perfil 4G
.\simulate-4g.ps1 -Profile 4g

# Restaurar configuración normal
.\simulate-4g.ps1 -Reset

# Listar perfiles
.\simulate-4g.ps1 -ShowProfiles
```

> **Limitación importante:** `netsh` solo puede limitar ancho de banda TCP.
> No soporta simulación de latencia, pérdida de paquetes o jitter.
> Para simulación completa (RTT, loss, jitter): usar [WSL2](https://learn.microsoft.com/en-us/windows/wsl/) o Linux nativo.

---

## Run Test Session

### Linux: `run-test-session.sh`

Script automatizado que:

1. Aplica perfil de red (opcional)
2. Compila servidor + loadgen
3. Inicia servidor en background
4. Espera hasta que el health check responda 200
5. Ejecuta loadgen por la duración configurada
6. Parsea logs del servidor buscando `chunk_latency`
7. Genera reporte JSON consolidado
8. Limpia (mata servidor, resetea red)

```bash
# Test rápido 30s con perfil WiFi hogar
./run-test-session.sh -Profile wifi-home -Duration 30s

# Test 10s sin simulación (baseline)
./run-test-session.sh -Duration 10s -SkipSimulation

# Guardar reporte en ruta específica
./run-test-session.sh -Profile 4g -Duration 60s -Output ./results/4g-test.json
```

Flags:

| Flag | Default | Descripción |
|------|---------|-------------|
| `-Profile` | `wifi-home` | Perfil de red a simular |
| `-Duration` | `60s` | Duración del test (ej: `30s`, `120s`) |
| `-Output` | `./report-<timestamp>.json` | Ruta del reporte JSON |
| `-SkipSimulation` | `false` | Omitir simulación de red (baseline) |
| `-h` / `--help` | — | Mostrar ayuda |

### Windows: `run-test-session.ps1`

Misma funcionalidad, adaptada a PowerShell:

```powershell
.\run-test-session.ps1 -Profile wifi-home -Duration 30s
.\run-test-session.ps1 -Duration 10s -SkipSimulation
.\run-test-session.ps1 -ShowProfiles
```

---

## Reporte JSON

El reporte consolidado tiene esta estructura:

```json
{
  "timestamp": "2026-06-12T14:30:00Z",
  "profile": "wifi-home",
  "duration_sec": 30,
  "server_logs": {
    "total_chunks": 1500,
    "chunks_ok": 1493,
    "chunks_error": 7,
    "error_rate_pct": 0.47,
    "latency_p50_ms": 18,
    "latency_p90_ms": 32,
    "min_chunk_ms": 5,
    "max_chunk_ms": 87,
    "total_chunks_AtoB": 750,
    "total_chunks_BtoA": 750
  },
  "loadgen": {
    "avg_rtt_ms": 22.3,
    "p50_rtt_ms": 20,
    "p90_rtt_ms": 35,
    "packet_loss_pct": 0.0
  },
  "status": "ok",
  "notes": []
}
```

### Status Classification

| Status | Criterio | Significado |
|--------|----------|-------------|
| `ok` | error_rate ≤ 5% AND p90 ≤ 1500ms | Rendimiento aceptable |
| `degraded` | error_rate 5-15% OR p90 1500-2500ms | Rendimiento reducido |
| `failed` | error_rate > 15% OR p90 > 2500ms OR loadgen error | No cumple requisitos |

### Campos clave

- **`server_logs.chunks_ok/error`**: Paquetes de audio procesados correctamente/fallidos
- **`server_logs.latency_p50/p90`**: Latencia de procesamiento de chunks (milisegundos)
- **`server_logs.error_rate_pct`**: Porcentaje de errores sobre total de chunks
- **`loadgen.avg_rtt_ms`**: RTT promedio medido desde WebSocket pings
- **`status`**: Clasificación automática basada en thresholds

---

## Advanced Usage

### Parsing logs con jq

```bash
# Ver solo chunk_latency durante un test
go run ./cmd/server | jq 'select(.msg == "chunk_latency")'

# Extraer latencias y calcular p50/p90 manualmente
go run ./cmd/server | jq 'select(.msg == "chunk_latency") | .total_ms' | sort -n | awk '{a[NR]=$1} END{print "p50:", a[int(NR*0.5)], "p90:", a[int(NR*0.9)]}'

# Ver eventos de sesión
go run ./cmd/server | jq 'select(.msg == "session_event")'
```

### Multiple profiles consecutivos

```bash
#!/bin/bash
for profile in wifi-home wifi-cafe 4g wan-lossy; do
    ./run-test-session.sh -Profile "$profile" -Duration 30s -Output "report-${profile}.json"
    sleep 5
done
```

### Baseline + profile comparison

```bash
# Baseline (sin simulación)
./run-test-session.sh -Duration 30s -SkipSimulation -Output baseline.json

# Con perfil
./run-test-session.sh -Profile 4g -Duration 30s -Output 4g.json

# Comparar
jq '{baseline_p50: .server_logs.latency_p50_ms}' baseline.json
jq '{profile_p50: .server_logs.latency_p50_ms}' 4g.json
```

---

## Troubleshooting

### Linux: `tc` permission denied

```bash
# El script intenta sudo automáticamente.
# Si no funciona, ejecutar manualmente:
sudo ./simulate-4g.sh -Profile wifi-home
```

### Linux: `qdisc not found`

```bash
# Verificar que iproute2 está instalado
tc -V

# Ver qdiscs existentes
tc qdisc show dev eth0

# Reset manual
sudo tc qdisc del dev eth0 root 2>/dev/null
```

### Linux: `jq not found`

```bash
# El script funciona sin jq, pero no parsea logs del servidor.
# Instalar:
sudo apt-get install -y jq   # Debian/Ubuntu
sudo yum install -y jq       # RHEL/CentOS
```

### Windows: `netsh admin required`

```powershell
# El script detecta y advierte si no hay admin.
# Ejecutar PowerShell como Administrador:
# 1. Click derecho en PowerShell → "Run as Administrator"
# 2. O: Start-Process powershell -Verb RunAs
```

### Windows: Latency/loss simulation no disponible

netsh en Windows nativo NO soporta simulación de latencia o pérdida de paquetes.
Opciones:

1. **WSL2** — Instalar [WSL2](https://learn.microsoft.com/en-us/windows/wsl/) con distribución Linux y usar `simulate-4g.sh`
2. **Clausewitz** — Herramienta de terceros para simulación de red en Windows
3. **Hardware** — Switch de red con capacidades de traffic shaping

### Server port conflict

```bash
# Si el puerto 8080 ya está en uso, matar el proceso:
lsof -ti:8080 | xargs kill
# Windows:
netstat -ano | findstr :8080
taskkill /PID <PID> /F
```

### Build fails

```bash
# Verificar versión de Go
go version  # debe ser 1.23 o superior

# Verificar módulo
go mod tidy
go build ./cmd/server && go build ./cmd/loadgen
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    End-to-End Network Test                    │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │  Network      │    │  TalkGo      │    │  Loadgen     │  │
│  │  Simulation   │───▶│  Server      │◀───│  Peer        │  │
│  │  (tc/netsh)   │    │  (cmd/server)│    │  (cmd/loadgen)│  │
│  └──────────────┘    └──────┬───────┘    └──────────────┘  │
│                              │                               │
│                     ┌────────▼────────┐                     │
│                     │  Server Logs    │                     │
│                     │  (JSON Lines)   │                     │
│                     └────────┬────────┘                     │
│                              │                               │
│                     ┌────────▼────────┐                     │
│                     │  Consolidated   │                     │
│                     │  Report (JSON)  │                     │
│                     └─────────────────┘                     │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## File Reference

| File | Propósito |
|------|-----------|
| `configs/4g.yml` | Perfil de red 4G |
| `configs/wifi-cafe.yml` | Perfil WiFi café congestionado |
| `configs/wifi-home.yml` | Perfil WiFi hogar ideal |
| `configs/wan-lossy.yml` | Perfil WAN con pérdida severa |
| `simulate-4g.sh` | Simulación de red Linux (tc/netem) |
| `simulate-4g.ps1` | Simulación de red Windows (netsh) |
| `run-test-session.sh` | Script de test automatizado Linux |
| `run-test-session.ps1` | Script de test automatizado Windows |
