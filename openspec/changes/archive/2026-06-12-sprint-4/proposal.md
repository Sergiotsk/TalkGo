# Sprint 4 Proposal: Polish y Alpha

**Change**: sprint-4
**Status**: proposed
**Date**: 2026-06-11

---

## 1. Intent

TalkGo tiene un backend funcional con signaling, WebRTC SFU, pipeline de traducción bidireccional, y un cliente React Native que puede mantener una conversación con reconexión y background mode. Sin embargo, el sistema es una caja negra: no sabemos cuánto tarda cada etapa del pipeline, los logs son texto plano sin estructura, no hay herramientas para testear en redes reales, y el proyecto carece de documentación de onboarding para nuevos desarrolladores.

Sprint 4 **instrumenta el sistema para hacerlo observable**, moderniza la infraestructura de logging, agrega herramientas de testing de red, documenta el onboarding, y corrige bugs críticos del Sprint 3. Después de este sprint, el equipo puede medir latencia end-to-end, testear en condiciones 4G/WiFi con scripts reproducibles, y cualquier nuevo desarrollador puede levantar el proyecto en <15 minutos.

La alpha con usuarios reales y la infraestructura TURN/NAT se difieren al Sprint 5.

---

## 2. Scope

### In Scope

| Área | Item | CA |
|------|------|----|
| **Instrumentación de latencia** | Agregar timestamps en `runHalf()` y funciones relacionadas del pipeline de traducción (captura de audio, decode, translate, encode, send), calcular p50/p90 por chunk, loguear eventos estructurados de sesión (connect, disconnect, duración, errores) | CA-01, CA-02 |
| **Logging modernization** | Migrar de `slog.NewTextHandler` a `slog.NewJSONHandler` con campos estructurados (`room_id`, `session_id`, `component`, `duration_ms`) en todos los adaptadores y el service layer | — |
| **Real network testing** | Scripts de testing de red (4G simulation via `tc`/`netsh`, WiFi pública, latencia artificial); tooling para ejecutar sesiones de prueba contra el backend con métricas de conectividad | CA-07 |
| **Onboarding documentation** | README expandido con setup paso a paso, arquitectura general, comandos comunes, troubleshooting, checklist de prerequisites (Go 1.23, Android SDK, Xcode, etc.) | — |
| **Bug fixes Sprint 3** | CRIT-01 (copylocks en `ListExpired` — cambiar `[]room.Room` a `[]*room.Room` en port + implementación + callers), WARN-01 (gofmt en 5 archivos), WARN-03 (exitAfterDefer en main.go), WARN-06 (harden test `TestHub_PeerLeft_NotifiedOnDisconnect` de `t.Logf` a `t.Errorf`) | NFR-01 |

### Out of Scope

| Item | Reason |
|------|--------|
| **TURN infrastructure** (Coturn/Twilio for symmetric NAT) | Deferred to Sprint 5 — STUN-only continues for Sprint 4 testing |
| **Real Opus codec** (reemplazar PassthroughCodec) | Deferred to Sprint 5 — PassthroughCodec is sufficient for latency measurement |
| **Alpha coordination with real users** | Deferred to Sprint 5 — this sprint builds the tooling, Sprint 5 runs the alpha |
| **Post-alpha bug fixes** | Deferred to Sprint 5 — unknown until alpha is run |
| **Pion WebRTC v3 → v4 migration** | Breaking change; defer to dedicated sprint |
| **Mobile app changes** (new screens, features) | No mobile feature work — only backend observability and tooling |

---

## 3. Architecture Decisions

### 3.1 Instrumentación de Latencia

#### 3.1.1 Timestamps en el Pipeline

**Where**: `internal/app/roomsvc/pipeline.go` — `startPipeline()`, `runHalf()`, y el nuevo helper `instrumentedHalf()` o wrapper.

**How**: Cada etapa del pipeline (audio capture → decode → translate → encode → send) registra un timestamp `time.Now()` antes y después de la operación. Los timestamps se agregan en un struct `ChunkLatency` que viaja con el chunk a través del pipeline.

Estructura del collector de latencia:

```go
type ChunkLatency struct {
    ChunkID       string        // unique per chunk (UUID or counter)
    StageTimings  []StageTiming // ordered list of stage measurements
    TotalDuration time.Duration // calculated at the end
}

type StageTiming struct {
    Stage    string        // "capture", "decode", "translate", "encode", "send"
    StartAt  time.Time
    EndAt    time.Time
    Duration time.Duration
}
```

**Stages to instrument in `runHalf()`**:

1. **Capture** — timestamp when frame arrives from `trackCh` (in the `OnAudioTrack` callback)
2. **Decode** — before and after `s.codec.Decode(ctx, opusCh)`
3. **Translate** — before and after `s.translator.TranslateStream(ctx, bpCh, ...)`
4. **Encode** — before and after `s.codec.Encode(ctx, translatedCh)`
5. **Send** — before and after `s.peer.SendAudio(ctx, ...)`

**Why not OpenTelemetry?**: OTel is powerful but adds significant complexity (exporter setup, collector, span management) for what is fundamentally a measurements-in-logs problem at our scale. `slog` JSON logging with structured fields can serve as our observability substrate. We can migrate to OTel in a future sprint if needed.

**Decision**: Use a `LatencyTracker` struct with `sync.Pool` for allocation efficiency, attach it to each pipeline half, and emit structured slog JSON at chunk completion. No external dependencies.

#### 3.1.2 Cálculo de p50/p90 por Chunk

Cada chunk completado emite un log JSON con:
```json
{
  "level": "info",
  "msg": "chunk_latency",
  "room_id": "<uuid>",
  "half": "AtoB",
  "chunk_id": "<counter>",
  "stages": {
    "capture_ms": 12,
    "decode_ms": 3,
    "translate_ms": 450,
    "encode_ms": 4,
    "send_ms": 2
  },
  "total_ms": 471,
  "status": "ok"
}
```

El cálculo de p50/p90 se hace offline procesando los logs (jq, grep, awk) o en un dashboard simple. No se implementa un agregador en-memoria en este sprint — mantenerlo simple y externalizado.

#### 3.1.3 Eventos de Sesión Estructurados

Agregar logs con `msg: "session_event"` en:

| Evento | Momento | Campos |
|--------|---------|--------|
| `session_start` | Después de `JoinRoom` exitoso | `room_id`, `session_id`, `user_id`, `lang` |
| `session_end` | En `LeaveRoom` o `OnDisconnect` | `session_id`, `duration_sec`, `reason` (voluntary/disconnect/timeout) |
| `session_error` | En errores del pipeline | `session_id`, `error`, `error_count`, `stage` |
| `pipeline_start` | En `startPipeline` | `room_id`, `sessA`, `sessB`, `langA`, `langB` |
| `pipeline_stop` | Cuando el pipeline termina (ctx cancelado) | `room_id`, `total_chunks_AtoB`, `total_chunks_BtoA` |

### 3.2 Logging Modernization

**Where**: `cmd/server/main.go:24`, más todos los archivos que usan `slog.Error`/`slog.Info`.

**How**: Cambiar `slog.NewTextHandler(os.Stdout, nil)` por:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo, // En desarrollo se puede cambiar a Debug
}))
slog.SetDefault(logger)
```

Además, modernizar TODAS las llamadas a `slog` para incluir campos estructurados en lugar de formatear mensajes:

| Antes | Después |
|-------|---------|
| `slog.Error("creating service", slog.Any("err", err))` | ✅ ya estructurado |
| `slog.Info("TalkGo starting")` | `slog.Info("server_starting", "component", "main")` |
| `slog.Error("server error", slog.Any("err", err))` | `slog.Error("server_error", "component", "http", slog.Any("err", err))` |

**Campos estructurados estándar** que toda llamada debe incluir cuando aplique:
- `component` — `"hub"`, `"service"`, `"http"`, `"pipeline"`, `"main"`
- `room_id` — cuando hay contexto de sala
- `session_id` — cuando hay contexto de sesión
- `duration_ms` — en operaciones medidas

Los mensajes pasan de ser frases en inglés a ser **identificadores cortos en snake_case** (`chunk_latency`, `session_start`, `peer_left`) que son más fáciles de filtrar y procesar.

**Decision**: Se estandariza el formato de mensajes a `snake_case` identificadores, NO frases completas. Porque los IDs son más fáciles de grepear, filtrar en dashboards, y procesar con herramientas de log.

### 3.3 Real Network Testing

**Where**: `scripts/network-test/` (nuevo directorio).

**Contents**:

| File | Purpose |
|------|---------|
| `scripts/network-test/simulate-4g.ps1` | Windows: usar `netsh` para limitar ancho de banda y agregar latencia |
| `scripts/network-test/simulate-4g.sh` | Linux: usar `tc` (netem) para simular 100ms RTT, 5% pérdida, 10Mbps |
| `scripts/network-test/run-test-session.sh` | Script que: (1) levanta el server, (2) conecta dos peers simulados via WebSocket, (3) envía audio sintético, (4) mide latencia, (5) parsea logs para p50/p90, (6) genera reporte |
| `scripts/network-test/README.md` | Cómo usar los scripts, qué redes testear, qué métricas esperar |
| `scripts/network-test/configs/` | Perfiles de red predefinidos: `4g.yml`, `wifi-cafe.yml`, `wifi-home.yml`, `wan-lossy.yml` |

**Simulated peer tool**: Un pequeño programa Go (`cmd/loadgen/`) o script que actúa como un peer WebSocket + WebRTC mínimo para testing automatizado (sin React Native). Envía tonos de audio sintéticos (PCM sinusoidal) y mide cuánto tarda en recibir el audio traducido.

**NOT TURN**: Este sprint solo construye la herramienta de testing. No se deploya TURN ni se testea contra NAT simétrico — eso es Sprint 5.

**Decision**: El loadgen peer será un script en Go (`cmd/loadgen/`) porque (a) reusa los puertos `driven.WebRTCPeer` y `driven.Translator` existentes, (b) no requiere un dispositivo móvil real para tests automatizados, (c) puede correr en CI. Alternativa considerada: script Python con aiortc — descartada porque agrega un lenguaje y dependencias al proyecto.

### 3.4 Onboarding Documentation

**Where**: `README.md` (modificado), más nueva sección en `docs/` si es necesario.

La documentación de onboarding cubre:

1. **Prerequisites**: Go 1.23+, Node.js 20+, React Native CLI, Android SDK (para mobile), Xcode (para iOS), Open AI API key
2. **Setup paso a paso**: clonar → `make setup` → configurar `.env` → `go run ./cmd/server`
3. **Arquitectura**: diagrama de capas hexagonal (domain → ports → adapters → app), explicación de `driving` vs `driven`, flujo de datos de una traducción
4. **Comandos**: `make` targets, `go test -race`, live reload con `air`, debugging
5. **Workflows**: cómo correr el server local, cómo conectar un cliente mobile, cómo testear el pipeline
6. **Testing**: cómo correr tests Go, Jest, cómo agregar un nuevo test, coverage
7. **Troubleshooting**: problemas comunes con Pion, WebRTC en LAN, OpenAI API keys, CGO

### 3.5 Bug Fixes Sprint 3

#### 3.5.1 CRIT-01: copylocks en ListExpired

**Where**:
- `internal/ports/driven/room_repository.go:37` — interfaz retorna `[]room.Room`
- `internal/app/roomsvc/repository.go:101` — implementación hace `append(expired, *rm)`
- `internal/app/roomsvc/repository_test.go:216` — range sobre slice de valores
- `internal/app/roomsvc/service_sprint3_test.go:283` — test retorna `[]room.Room{*r}`

**Fix**: Cambiar la firma de `ListExpired` en el port, la implementación, y todos los callers a `[]*room.Room`. Esto elimina la copia del `sync.Mutex` contenida en `room.Room` y arregla el lint error.

**Ripple effect**: `sweepExpiredRooms` en `service.go` itera sobre `expired` — cambiar a `for _, r := range expired` donde `r` es `*room.Room`. Llamar `s.repo.DeleteRoom(ctx, r.ID)` en lugar de `rm.ID`.

#### 3.5.2 WARN-01: gofmt violations

Correr `gofmt -w` en los 5 archivos:
- `internal/adapters/signaling/hub.go`
- `internal/ports/driven/mocks/mock_room_repository.go`
- `internal/adapters/http/server_test.go`
- `internal/app/roomsvc/service_sprint3_test.go`
- `internal/domain/room/room.go`

#### 3.5.3 WARN-03: exitAfterDefer

**Where**: `cmd/server/main.go:60` — `os.Exit(1)` previene que `defer cancel()` se ejecute.

**Fix**: Reemplazar `os.Exit(1)` con `return 1`. La función `run()` usa `os.Exit(run())` en `main()`, así que `return 1` logra lo mismo pero permite que los defers corran.

#### 3.5.4 WARN-06: TestHub_PeerLeft_NotifiedOnDisconnect

**Where**: test existente usa `t.Logf` en lugar de `t.Errorf` cuando no recibe `peer-left`.

**Fix**: Cambiar `t.Logf` a `t.Errorf`. El `roomClients` SÍ se trackea, así que no recibir `peer-left` es un error real, no un "puede esperarse".

---

## 4. Dependencies

### Go (go.mod) — Sin nuevas dependencias externas

Toda la instrumentación y logging usa solo la stdlib (`log/slog`, `time`, `sync`, `encoding/json`).

### Scripts — Dependencias externas

| Dependencia | Purpose | Instalación |
|-------------|---------|-------------|
| `netsh` (Windows) | Simulación 4G | Built-in en Windows 10/11 |
| `tc` / `netem` (Linux) | Simulación 4G | `apt install iproute2` |
| `jq` | Procesamiento de logs JSON | `choco install jq` / `apt install jq` |

---

## 5. Risks

| # | Risk | Impact | Probability | Mitigation |
|---|------|--------|-------------|------------|
| 1 | **LatencyTracker introduce overhead no trivial** | Mediciones sesgadas por el propio instrumento | Medium | Usar `time.Now()` directo (1µs overhead), NO high-res timers. Benchmark antes/después. `sync.Pool` para evitar allocations. |
| 2 | **Logs JSON muy verbosos en cada chunk** | Log flooding, dificulta lectura humana | Medium | Nivel `Info` para chunk individual; nivel `Warn`/`Error` para outliers. Agregar opción `-log-level` en el server. |
| 3 | **Loadgen peer complejidad inesperada** | Network testing script toma más tiempo que el estimado | Medium | MVP del loadgen: solo conecta WebSocket + mide latencia de round-trip. WebRTC real puede ser fase 2. |
| 4 | **copylocks fix tiene ripple effect en tests** | Tests existentes compilan si cambiamos la firma de `ListExpired` | Medium | Buscar TODOS los callers con `grep -r "ListExpired"` antes de modificar. Hacer el cambio en un solo commit para facilidad de review. |
| 5 | **No se puede testear background iOS en Windows** | Instrumentación de red iOS no verificable | Low | Los scripts de network testing son multiplataforma, pero testing iOS requiere macOS. Documentar limitación. |

---

## 6. Definition of Done

Todos los criterios referencian los Sprint 4 acceptance criteria (CA-01 a CA-08) y NFRs.

### Instrumentación

- [ ] **CA-01**: `LatencyTracker` emite logs JSON con timestamps por chunk en `runHalf()`. Verificado por test: pipeline con chunks sintéticos produce logs con estructura `chunk_latency`.
- [ ] **CA-02**: Logs de latencia incluyen `total_ms` y `stages.*_ms`. Verificado por test: cada chunk emitido tiene campos `total_ms`, `stages.decode_ms`, `stages.translate_ms`, `stages.encode_ms`.
- [ ] Eventos de sesión (`session_start`, `session_end`, `pipeline_start`, `pipeline_stop`) se emiten en los momentos definidos. Verificado por test de integración.
- [ ] Chunks procesados correctamente vs. con error se contabilizan en logs. Verificado por test.

### Logging

- [ ] `slog.NewJSONHandler` reemplaza `NewTextHandler` en `main.go`.
- [ ] Todas las llamadas a `slog` en el códigobase (`main.go`, `hub.go`, `service.go`, `pipeline.go`, `server.go`, `client.go`) usan `component` field estándar y mensajes en snake_case.
- [ ] `grep -r "slog\.Info\|slog\.Error\|slog\.Warn\|slog\.Debug"` en los archivos modificados muestra formato estructurado correcto.
- [ ] Logs de ejemplo documentados en el README.

### Network Testing

- [ ] **CA-07**: Scripts de simulación de red existen para Windows (`.ps1`) y Linux (`.sh`).
- [ ] Loadgen peer en `cmd/loadgen/` puede conectar WebSocket, enviar mensajes, y medir round-trip.
- [ ] `scripts/network-test/README.md` documenta cómo ejecutar una sesión de prueba en 4G simulado.
- [ ] Perfiles de red predefinidos (`4g.yml`, `wifi-cafe.yml`, `wifi-home.yml`) existen en `scripts/network-test/configs/`.

### Onboarding Documentation

- [ ] README.md actualizado con: prerequisites, setup paso a paso, arquitectura, comandos, troubleshooting.
- [ ] README incluye sección de "Primeros pasos" que un nuevo desarrollador puede seguir sin ayuda.
- [ ] Documentación de testing (cómo correr tests, cómo agregar un test nuevo) presente.

### Bug Fixes

- [ ] **CRIT-01**: `ListExpired` retorna `[]*room.Room`. Todos los callers actualizados. `golangci-lint run` pasa sin copylocks.
- [ ] **WARN-01**: `gofmt -w` corrido en los 5 archivos. `go fmt ./...` no produce cambios.
- [ ] **WARN-03**: `os.Exit(1)` reemplazado por `return 1` en `main.go`. `go vet ./...` no reporta `exitAfterDefer`.
- [ ] **WARN-06**: Test `TestHub_PeerLeft_NotifiedOnDisconnect` usa `t.Errorf`. Test pasa tanto cuando `peer-left` se recibe como cuando no (nuevo test opcional).
- [ ] **NFR-01**: `go vet ./...` y `golangci-lint run ./...` pasan con zero issues.
- [ ] **NFR-02**: Coverage ≥ 80% en archivos modificados. Coverage existente no disminuye.

---

## 7. Workstreams & Task Ordering

El sprint se organiza en 3 workstreams que pueden correr en paralelo:

### Workstream A: Instrumentación + Logging (Backend Go puro)

Dependencias: sdd-spec → sdd-design → sdd-tasks → sdd-apply → sdd-verify

Este es el workstream principal y el único que toca código de producción.

Orden recomendado:
1. Bug fixes Sprint 3 (CRIT-01, WARN-01, WARN-03, WARN-06) — primero porque son fixes limpios y desbloquean lint
2. Logging modernization (JSON Handler, estandarizar campos) — porque la instrumentación depende del nuevo logger
3. Instrumentación de latencia (LatencyTracker, timestamps, eventos de sesión) — sobre el logging modernizado
4. Tests de latencia (verificar que los logs emitidos contienen los campos esperados)

### Workstream B: Network Testing (Scripts + loadgen)

Dependencias: sdd-design → sdd-tasks → sdd-apply → sdd-verify

Orden recomendado:
1. Loadgen peer en `cmd/loadgen/` (mínimo: WebSocket connect + audio sintético)
2. Scripts de simulación de red (Windows + Linux, perfiles 4G/WiFi)
3. Reporte de testing (parsing logs JSON → p50/p90)

### Workstream C: Onboarding Documentation

Dependencias: debe esperar a que los cambios de Workstream A estén estables (para documentar el setup actual).

Orden recomendado:
1. README expandido (al final del sprint, cuando todo lo demás esté estable)

---

## Appendix: Capabilities Changes

Esta sección es el contrato entre la propuesta y el `sdd-spec`.

### New Capabilities

| Capability | Description |
|------------|-------------|
| `latency-instrumentation` | Instrumentación del pipeline de traducción con timestamps, cálculo de latencia por chunk, y eventos estructurados de sesión |
| `network-testing` | Scripts de simulación de red (4G, WiFi), loadgen peer para testing automatizado, y perfiles de red predefinidos |

### Modified Capabilities

| Capability | What Changes |
|------------|-------------|
| `translation` | El pipeline ahora registra timestamps en cada etapa y emite logs JSON de latencia por chunk. Se agregan eventos de sesión (`session_start`, `session_end`, `pipeline_start`, `pipeline_stop`). Requisitos existentes no cambian — solo se agrega observabilidad. |

### No Spec Changes Required

- **Logging modernization**: Refactor puro de infraestructura — no cambia comportamiento observable por el dominio o los clientes.
- **Onboarding documentation**: Documentación pura — no requiere spec.
- **Bug fixes**: Corrigen comportamiento existente para cumplir con specs ya definidas.
