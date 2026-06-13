# TalkGo — Traducción Simultánea por Dispositivo

TalkGo es una plataforma de traducción simultánea por voz en tiempo real pensada para conversaciones cara a cara multidisciplinarias e internacionales utilizando dispositivos móviles individuales.

## Prerequisites

- **Go 1.23+** — [Descargar](https://go.dev/dl/)
- **Node.js 20+** — para el cliente móvil React Native
- **React Native CLI** + **Android SDK** / **Xcode** — para desarrollo mobile
- **OpenAI API key** — para el translator (si se conecta a OpenAI Realtime)
- **WSL2** (Windows) — para simulación de red completa (packet loss, jitter)
- **jq** (opcional) — para filtrar logs JSON: `winget install jq` o `apt install jq`

## Quick Start

```bash
# 1. Clonar
git clone https://github.com/Sergiotsk/TalkGo.git
cd TalkGo

# 2. Setup inicial (linters, herramientas)
make setup

# 3. Configurar .env (opcional, valores por defecto funcionan)
cp .env.example .env
# Editar OPENAI_API_KEY=sk-... si usas OpenAI Realtime

# 4. Build
make build

# 5. Correr servidor
go run ./cmd/server -addr :8080 -log-level debug

# 6. En otra terminal, probar con curl
curl -X POST http://localhost:8080/api/rooms -H "Content-Type: application/json" -d '{"source_lang":"es","target_lang":"en"}'
```

## Arquitectura

El backend del proyecto está estructurado con **Arquitectura Hexagonal (Ports & Adapters)** estricta en Go.

### Mapa de Dependencias

```
Driving Ports         Service Layer          Driven Ports
  (HTTP/WS)           (roomsvc)              (WebRTC/Translator/Codec/Repo)
       |                  |                        |
  cmd/server → internal/app/roomsvc → internal/ports/driven/{interfaces}
       ↓                  ↓                        ↓
  internal/adapters   internal/domain          mocks + implementations
  (hub, http, ws)     (room, session)         (pion, mock, etc.)
```

### Capas

- **Domain (`internal/domain/`)**: Tipos puros y lógica de negocio. Cero dependencias externas. `Room`, `Session`.
- **Ports (`internal/ports/`)**: Interfaces (driving y driven). Contratos que definen los límites arquitectónicos.
- **App (`internal/app/roomsvc/`)**: Orquestación. Pipeline de traducción, manejo de sesiones, logging instrumentado.
- **Adapters (`internal/adapters/`)**: Implementaciones concretas (WebSocket hub, servidor HTTP, mocks).

### Enforcing

El proyecto usa `depguard` en `golangci-lint` para **bloquear importaciones que violen la arquitectura**. Por ejemplo, `internal/domain/` no puede importar `pion/webrtc`.

```
AI Agent Harness:
1. Prevención → `.cursor/rules/talkgo.mdc`
2. Detección  → `.golangci.yml` (depguard)
3. Bloqueo    → CI/CD (`make check` antes de merge)
```

## Make Targets

| Target | Description |
|--------|-------------|
| `make setup` | Instala linters y herramientas |
| `make build` | Compila todo el backend |
| `make test` | Tests con race detector |
| `make lint` | golangci-lint estricto |
| `make check` | Format + lint + test (CI) |
| `make run` | go run ./cmd/server |

## Running Tests

```bash
# Todos los tests con race detector
go test -race ./...

# Con cobertura
go test -cover ./...

# Tests específicos
go test -v -run TestLatency ./internal/app/roomsvc/...

# Benchmarks
go test -bench=. -benchmem ./internal/app/roomsvc/...

# Saturation test (N=10 salas concurrentes)
go test -v -run TestSaturation ./internal/app/roomsvc/...
```

### Tests notables

- **65+ tests** en `internal/app/roomsvc/` — pipeline, service, latency, logging
- **14 tests** en `cmd/loadgen/` — report math, audio validation, benchmarks
- **0 races** en toda la suite (`-race` limpio)

## Logging

TalkGo usa `slog.NewJSONHandler` para logs estructurados en JSON. Todos los mensajes usan **snake_case** y tienen un atributo `component` (service, pipeline, hub).

```bash
# Ver todos los chunk_latency con jq
go run ./cmd/server | jq 'select(.msg == "chunk_latency") | {total_ms, stages}'

# Pipeline events
go run ./cmd/server | jq 'select(.msg == "session_event") | {event, session_id}'

# Live filter por nivel
go run ./cmd/server | jq 'select(.level == "error")'
```

### Eventos principales

| Evento | Componente | Campos clave |
|--------|-----------|--------------|
| `session_event` (session_start) | service | session_id, room_id, user_id, lang |
| `session_event` (pipeline_start) | pipeline | room_id, sessA, sessB, langA, langB |
| `session_event` (pipeline_stop) | pipeline | total_chunks_AtoB, total_chunks_BtoA |
| `session_event` (session_end) | service | session_id, duration_sec, event_type |
| `chunk_latency` | pipeline | chunk_id, half, total_ms, stages{...} |
| `session_event` (session_error) | pipeline | session_id, error, error_count, stage |

## Network Testing

### Load Generator

```bash
# Conectar a servidor local, 10 sesiones virtuales, 30s
go run ./cmd/loadgen -server ws://localhost:8080 -room abc123 -duration 30s -sessions 10
```

Flags: `-server`, `-room` (o auto-creación), `-duration`, `-sessions`, `-rate`, `-output`

### Simulación de Red

```bash
# Linux / WSL2 — simular red 4G
sudo ./scripts/network-test/simulate-4g.sh apply 4g

# Windows — limitar ancho de banda
.\scripts\network-test\simulate-4g.ps1

# Ver perfiles disponibles
ls scripts/network-test/configs/
```

### Run-Test-Session (end-to-end)

```bash
# Linux
./scripts/run-test-session.sh --profile 4g --duration 60s

# Windows
.\scripts\run-test-session.ps1 -Profile 4g -Duration 60
```

## Cómo Contribuir (Gente y Agentes)

Tanto si eres humano como un agente de IA, debes seguir estas reglas:

1. **Strict TDD**: Escribir los tests antes de la implementación.
2. **Hexagonal isolation**: Nunca cruzar fronteras de paquetes. Define un puerto (Port) primero.
3. **Zero warnings**: No se permiten warnings de compilación o linter acumulados.
4. **Conventional commits**: Usar `feat:`, `fix:`, `docs:`, `refactor:` etc. Sin "Co-Authored-By".

### Documentación para desarrolladores

Ver `docs/devel/onboarding.md` para una guía completa de onboarding, y `docs/devel/architecture.md` para el detalle de arquitectura.

## Troubleshooting

| Problema | Causa | Solución |
|----------|-------|----------|
| `CGO_ENABLED=0` required | mingw o GCC no instalado en Windows | `set CGO_ENABLED=0` o instalar GCC |
| Port already in use | Otro proceso en el puerto | Cambiar con `-addr :8081` |
| OpenAI API key missing | Translator necesita API key | Configurar `OPENAI_API_KEY` en `.env` |
| WebSocket connection fails | Server no corriendo o puerto incorrecto | Verificar `go run ./cmd/server` |
| Data races in tests | Log buffer compartido sin mutex | Usar `lockedBuffer` (ver tests existentes) |
| Packet loss simulation no-op | Windows sin WSL2 | Usar WSL2 para netem, o netsh para bandwidth |
| `make lint` fails | depguard bloquea import | Mover el código al paquete correcto siguiendo hex. arch. |

## Estado del Proyecto

| Sprint | Descripción | Estado |
|--------|-------------|--------|
| Sprint 0 | Scaffolding, arquitectura hexagonal, AI harness | ✅ Completo |
| Sprint 1 | WebRTC Signaling & Room Management | ✅ Completo |
| Sprint 2 | Pipeline de Traducción (OpenAI Realtime) | ✅ Completo |
| Sprint 3 | UX y Edge Cases | ✅ Completo |
| Sprint 4 | Polish y Alpha (Logging, Latency, Network Testing, Docs) | ✅ Completo |

### Deuda técnica activa
- **DT-01**: `PassthroughCodec` → codec Opus real (libopus) — bloqueante para producción
- **DT-02**: Reinicio por chunk en `runHalf` cuando el translator falla (CA-04 parcial)
- **DT-03**: Loadgen es WebSocket-only — falta WebRTC loadgen (Sprint 5)
- **DT-04**: TURN infrastructure no implementada — symmetric NAT blocked

## Estructura del Proyecto (Monorepo)

```
TalkGo/
├── cmd/
│   ├── server/            # Entry point del servidor backend
│   └── loadgen/           # WebSocket load generator
├── internal/
│   ├── domain/            # Core puro (room, session)
│   ├── ports/             # Interfaces driving/driven
│   ├── app/roomsvc/       # Service layer (pipeline, latency)
│   └── adapters/          # Implementaciones (hub, http, ws)
├── scripts/
│   ├── network-test/      # Simulación de red + run-test-session
│   └── ...
├── mobile/                # Cliente React Native
├── docs/
│   ├── devel/             # Onboarding + architecture docs
│   └── sprints/           # Sprint plans
├── openspec/              # SDD artifacts
└── Makefile
