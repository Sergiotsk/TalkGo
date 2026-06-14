# Sprint 4 Spec: Polish y Alpha — Instrumentación, Logging, Network Testing

**Change**: sprint-4
**Status**: spec
**Date**: 2026-06-11

---

## Overview

Sprint 4 instrumenta el pipeline de traducción para hacerlo observable, moderniza la infraestructura de logging, agrega herramientas de testing de red, y cierra bugs críticos del Sprint 3. Después de este sprint, el equipo puede medir latencia end-to-end por chunk (p50/p90), testear en condiciones 4G/WiFi con scripts reproducibles, y el proyecto cumple con lint estricto.

### What Changes vs Main Specs

| Spec | Change |
|------|--------|
| `specs/translation/spec.md` (Sprint 2) | El pipeline ahora registra timestamps en cada etapa y emite logs JSON de latencia por chunk. Se agregan eventos de sesión (`session_start`, `session_end`, `pipeline_start`, `pipeline_stop`). Requisitos existentes (REQ-01 a REQ-14) no cambian — solo se agrega observabilidad. |
| New: `latency-instrumentation` | `ChunkLatency` struct, `LatencyTracker` con `sync.Pool`, stage timing en `runHalf()`, emisión de logs JSON estructurados. |
| New: `network-testing` | Loadgen peer en `cmd/loadgen/`, scripts de simulación de red para Windows (`.ps1`) y Linux (`.sh`), perfiles de red predefinidos. |

### Workstreams

| Workstream | Área | Toca código producción | Depende de |
|------------|------|:----------------------:|------------|
| A | Instrumentación + Logging | Sí | — |
| B | Network Testing | Sí (cmd/loadgen/) | — |
| C | Onboarding Documentation | No | Workstream A estable |

---

## Workstream A: Instrumentación y Logging

### REQ-LOG-01: JSON structured logging handler

El `main.go` MUST configurar `slog.NewJSONHandler(os.Stdout, nil)` como default handler en lugar de `slog.NewTextHandler`. El handler level MUST ser `slog.LevelInfo` por defecto, configurable vía flag `-log-level`.

**Escenarios**:

**SC-LOG-01-01: JSON handler configurado al iniciar**
```
Given: cmd/server/main.go
When:  el servidor inicia
Then:  slog.SetDefault() recibe un *slog.JSONHandler escribiendo a os.Stdout
And:   el level por defecto es slog.LevelInfo
```

**SC-LOG-01-02: Log level configurable vía flag**
```
Given: servidor iniciado con -log-level=debug
When:  se emite un mensaje slog.Debug
Then:  el mensaje aparece en stdout
```

---

### REQ-LOG-02: Mensajes en formato snake_case

TODAS las llamadas a `slog.Info`, `slog.Error`, `slog.Warn`, `slog.Debug` en los archivos modificados del workstream A (`main.go`, `hub.go`, `service.go`, `pipeline.go`, `server.go`, `client.go`) MUST usar identificadores cortos en `snake_case` como primer argumento del mensaje, NO frases completas en lenguaje natural.

| Antes | Después |
|-------|---------|
| `slog.Info("TalkGo starting")` | `slog.Info("server_starting", "component", "main")` |
| `slog.Error("server error", ...)` | `slog.Error("server_error", "component", "http", ...)` |
| `slog.Info("peer left room")` | `slog.Info("peer_left", "component", "hub", "room_id", rID)` |

**Escenarios**:

**SC-LOG-02-01: Mensajes son identificadores snake_case**
```
Given: cualquier archivo modificado (main.go, hub.go, service.go, pipeline.go, server.go, client.go)
When:  se inspecciona el string constante en la primera posición de slog.Info/Error/Warn/Debug
Then:  el string es snake_case (ej: "chunk_latency", "session_start", "server_starting", "peer_left")
And:   NO contiene espacios ni caracteres en mayúscula sostenida
```

**SC-LOG-02-02: Sin frases en lenguaje natural**
```
Given: grep de los archivos modificados
When:  se buscan strings con espacios en primera posición de slog llamadas
Then:  zero resultados (no hay "starting server", "error occurred", etc.)
```

---

### REQ-LOG-03: Campos estructurados estándar

TODA llamada a `slog` en los archivos modificados MUST incluir el field `"component"` con uno de los valores estándar. Cuando aplique contexto, MUST incluir también `room_id`, `session_id`, `duration_ms`.

**Valores estándar de `component`**:
- `"main"` — para mensajes de inicio/cierre del server
- `"http"` — para mensajes del adaptador HTTP
- `"hub"` — para mensajes del signaling hub (WebSocket)
- `"service"` — para mensajes del service layer
- `"pipeline"` — para mensajes del pipeline de traducción
- `"loadgen"` — para el peer de testing (Workstream B)

**Escenarios**:

**SC-LOG-03-01: Component field presente en toda llamada**
```
Given: lista de archivos modificados en Workstream A
When:  se verifica cada llamada a slog con grep
Then:  toda llamada incluye el par "component", "<valor>"
And:   el valor está en la lista estándar
```

**SC-LOG-03-02: Contexto de sala incluye room_id**
```
Given: una operación que ocurre dentro del contexto de una sala (JoinRoom, LeaveRoom, pipeline)
When:  se emite un log
Then:  el log incluye "room_id", "<uuid>"
```

**SC-LOG-03-03: Operaciones medidas incluyen duration_ms**
```
Given: una operación que mide tiempo (creación de sala, latency chunk, sweep)
When:  se emite un log con resultado de esa operación
Then:  el log incluye "duration_ms", <int>
```

---

### REQ-LAT-01: ChunkLatency struct

El paquete `internal/app/roomsvc` (o un nuevo paquete `internal/app/latency/`) MUST contener el struct `ChunkLatency` con la siguiente estructura:

```go
type ChunkLatency struct {
    ChunkID       string
    StageTimings  []StageTiming
    TotalDuration time.Duration
}

type StageTiming struct {
    Stage    string    // "capture", "decode", "translate", "encode", "send"
    StartAt  time.Time
    EndAt    time.Time
    Duration time.Duration
}
```

`ChunkLatency` NO MUST ser un puntero en el pool (para evitar escapes al heap). Los stage names MUST ser constantes del paquete.

---

### REQ-LAT-02: LatencyTracker con sync.Pool

El `LatencyTracker` MUST:
- Usar `sync.Pool` para reutilizar instancias de `ChunkLatency` y slices de `StageTiming`
- Exponer `StartStage(stage string)` y `EndStage(stage string)` que registran timestamps
- Calcular `TotalDuration` automáticamente al llamar `Emit()` (que también retorna el struct al pool)
- Ser seguro para uso concurrente (cada pipeline half tiene su propio tracker)

```go
type LatencyTracker struct {
    pool    *sync.Pool
    current *ChunkLatency
    mu      sync.Mutex
}

func NewLatencyTracker() *LatencyTracker
func (lt *LatencyTracker) StartStage(stage string)
func (lt *LatencyTracker) EndStage(stage string)
func (lt *LatencyTracker) Emit(ctx context.Context, half string, roomID string, status string)  // emite log y retorna al pool
func (lt *LatencyTracker) Reset()  // prepara para nuevo chunk
```

**Escenarios**:

**SC-LAT-02-01: sync.Pool reutiliza instancias**
```
Given: LatencyTracker creado con sync.Pool
When:  100 chunks son procesados secuencialmente, cada uno con StartStage/EndStage/Emit
Then:  el número de allocaciones de ChunkLatency es ≤ 10
And:   runtime.ReadMemStats en benchmark muestra zero escapes al heap (no heap allocs por tracker)
```

**SC-LAT-02-02: Tracker captura Start/End correctamente**
```
Given: LatencyTracker.Reset() llamado
When:  StartStage("decode") → EndStage("decode") → StartStage("translate")
Then:  StageTimings tiene 2 entries
And:   StageTimings[0].Stage == "decode"
And:   StageTimings[0].Duration ≈ StageTimings[0].EndAt.Sub(StageTimings[0].StartAt)
And:   StageTimings[1].Stage == "translate"
And:   StageTimings[1].Duration es cero (EndStage aún no llamado)
```

**SC-LAT-02-03: Emit retorna struct al pool correctamente**
```
Given: LatencyTracker con chunk completado
When:  Emit() es llamado
Then:  el log JSON con chunk_latency es emitido
And:   tras Emit(), Reset() prepara el tracker para el próximo chunk
And:   nextChunk = pool.Get() devuelve la misma instancia (comprobable con puntero)
```

---

### REQ-LAT-03: Stage timing en runHalf()

La función `runHalf()` en `pipeline.go` MUST ser instrumentada para registrar timestamps en las 5 etapas del pipeline:

| Etapa | Momento Start | Momento End |
|-------|---------------|-------------|
| `capture` | Frame recibido de `trackCh` (callback `OnAudioTrack`) | Cuando el dato es puesto en el canal de entrada del decode |
| `decode` | Antes de `s.codec.Decode(ctx, opusCh)` | Después de que `Decode` retorna (canal de salida listo) |
| `translate` | Antes de `s.translator.TranslateStream(ctx, bpCh, ...)` | Después de que `TranslateStream` retorna (canal de salida listo) |
| `encode` | Antes de `s.codec.Encode(ctx, translatedCh)` | Después de que `Encode` retorna (canal de salida listo) |
| `send` | Antes de `s.peer.SendAudio(ctx, ...)` | Después de que `SendAudio` retorna (o error) |

Cada etapa MUST llamar a `tracker.StartStage("name")` y `tracker.EndStage("name")`. Si una etapa falla, MUST registrar `EndStage` igualmente (el tiempo hasta el error queda registrado). El `status` en `Emit()` se setea a `"error"` si alguna etapa falló, `"ok"` en caso contrario.

**Escenarios**:

**SC-LAT-03-01: Pipeline completo emite chunk_latency con status ok**
```
Given: pipeline activo para A→B con mocks rápidos (<1ms por etapa)
When:  un frame Opus es recibido en trackCh
Then:  el pipeline procesa capture→decode→translate→encode→send
And:   tracker.Emit() es llamado
And:   el log tiene:
  - "msg": "chunk_latency"
  - "component": "pipeline"
  - "room_id": "<uuid>"
  - "half": "AtoB"
  - "chunk_id": <counter starting at 1>
  - "stages": { "capture_ms": <int>, "decode_ms": <int>, "translate_ms": <int>, "encode_ms": <int>, "send_ms": <int> }
  - "total_ms": <int>
  - "status": "ok"
```

**SC-LAT-03-02: Etapa falla — chunk_latency con status error**
```
Given: pipeline activo con mock de AudioCodec.Decode que falla
When:  un frame Opus es recibido
Then:  decode registra EndStage con error
And:   el log chunk_latency tiene:
  - "status": "error"
  - "stages.decode_ms": <tiempo hasta el error>
  - stages.subsiguientes pueden estar ausentes o cero
```

**SC-LAT-03-03: ChunkID incrementa secuencialmente por half**
```
Given: pipeline activo procesando múltiples chunks
When:  3 chunks son procesados en el half AtoB
Then:  chunk_id = 1, 2, 3 en orden
And:   el half BtoA tiene su propio contador independiente
```

---

### REQ-LAT-04: Eventos de sesión estructurados

El service layer MUST emitir logs con `"msg": "session_event"` y un field `"event"` adicional en los siguientes momentos:

| Evento | Momento | Campos Requeridos |
|--------|---------|-------------------|
| `session_start` | Después de `JoinRoom` exitoso (ambos peers conectados) | `room_id`, `session_id`, `user_id`, `lang`, `component: "service"` |
| `session_end` | En `LeaveRoom` o `OnDisconnect` | `session_id`, `duration_sec`, `reason` ("voluntary", "disconnect", "timeout"), `component: "service"` |
| `session_error` | En errores del pipeline | `session_id`, `error`, `error_count`, `stage`, `component: "pipeline"` |
| `pipeline_start` | En `startPipeline` | `room_id`, `sessA`, `sessB`, `langA`, `langB`, `component: "pipeline"` |
| `pipeline_stop` | Cuando el pipeline termina (ctx cancelado) | `room_id`, `total_chunks_AtoB`, `total_chunks_BtoA`, `component: "pipeline"` |

**Escenarios**:

**SC-LAT-04-01: session_start emitido después de JoinRoom exitoso**
```
Given: dos peers A y B se unen a la misma sala, ambos completan ICE
When:  startPipeline es invocado
Then:  se emite log JSON con:
  - "msg": "session_event"
  - "event": "session_start"
  - "room_id": "<uuid>"
  - "session_id": "<session_A_ID>"
  - "user_id": "<user_A>"
  - "lang": "es"
  - "component": "service"
And:  también se emite para B con Lang="en"
```

**SC-LAT-04-02: session_end emitido en LeaveRoom voluntario**
```
Given: sesión activa entre A y B
When:  Service.LeaveRoom(sessionID_A) es llamado
Then:  se emite log JSON con:
  - "msg": "session_event"
  - "event": "session_end"
  - "session_id": "<session_A_ID>"
  - "duration_sec": <int> (segundos desde session_start)
  - "reason": "voluntary"
  - "component": "service"
```

**SC-LAT-04-03: session_end emitido en desconexión abrupta**
```
Given: sesión activa entre A y B
When:  Hub detecta que la conexión WebSocket de A cayó (sin mensaje leave)
And:   Hub.OnDisconnect(ctx, sessionID_A) es llamado en el Service
Then:  se emite log session_event con:
  - "event": "session_end"
  - "session_id": "<session_A_ID>"
  - "reason": "disconnect"
```

**SC-LAT-04-04: session_error emitido en fallo del pipeline**
```
Given: pipeline activo con un Translator que falla intermitentemente
When:  TranslateStage retorna error por segunda vez
Then:  se emite log session_event con:
  - "event": "session_error"
  - "session_id": "<sess_A>"
  - "error": "translation failed: <msg>"
  - "error_count": 2
  - "stage": "translate"
  - "component": "pipeline"
```

**SC-LAT-04-05: pipeline_start emitido al iniciar**
```
Given: sala completa con A (Lang: es) y B (Lang: en) unidos
When:  startPipeline es invocado con sessA y sessB
Then:  se emite log JSON con:
  - "msg": "session_event"
  - "event": "pipeline_start"
  - "room_id": "<uuid>"
  - "sessA": "<session_A_ID>"
  - "sessB": "<session_B_ID>"
  - "langA": "es"
  - "langB": "en"
  - "component": "pipeline"
```

**SC-LAT-04-06: pipeline_stop emitido al cancelar pipeline**
```
Given: pipeline activo para A→B y B→A, se procesaron 15 y 22 chunks respectivamente
When:  el contexto del pipeline es cancelado (por LeaveRoom o DeleteRoom)
Then:  se emite log JSON con:
  - "msg": "session_event"
  - "event": "pipeline_stop"
  - "room_id": "<uuid>"
  - "total_chunks_AtoB": 15
  - "total_chunks_BtoA": 22
  - "component": "pipeline"
```

---

### REQ-LAT-05: Contador de chunks procesados vs errores

El pipeline MUST mantener contadores atómicos de:
- `totalChunks` — chunks procesados completamente (status: ok)
- `errorChunks` — chunks que fallaron en alguna etapa (status: error)

Estos contadores MUST ser almacenados en el struct `pipelineHalf` (o equivalente) y expuestos en el log `pipeline_stop` como `total_chunks_AtoB` / `total_chunks_BtoA`. También MUST ser accesibles para que `pipeline_stop` refleje valores precisos incluso si el pipeline es cancelado abruptamente.

**Escenarios**:

**SC-LAT-05-01: Contadores se incrementan correctamente**
```
Given: pipeline activo
When:  5 chunks ok y 2 chunks error son procesados
Then:  totalChunks == 5
And:   errorChunks == 2
And:   pipeline_stop.total_chunks_AtoB == 5
```

---

## Workstream B: Network Testing

### REQ-NET-01: Loadgen peer en cmd/loadgen/

El directorio `cmd/loadgen/` MUST contener un programa Go que actúa como peer simulado para testing de red. El loadgen peer:

1. MUST conectar al servidor TalkGo vía WebSocket (`/ws/{roomID}`)
2. MUST seguir el protocolo de signaling (join → offer/answer → ICE)
3. MUST enviar audio sintético (tono sinusoidal PCM a 24 kHz mono, duración 20ms por frame)
4. MUST medir round-trip time de mensajes signaling
5. MUST soporte los siguientes flags CLI:

```
-server string      URL del servidor (default "localhost:8080")
-room string        Room ID (default: auto-generado)
-lang string        Idioma del peer (default "es")
-duration duration  Duración de la sesión (default 30s)
-profile string     Perfil de red para documentar en reporte (default "wifi-home")
-output string      Archivo de reporte (default: stdout JSON)
```

6. MUST emitir un reporte JSON al finalizar con esta estructura:

```json
{
  "profile": "4g",
  "duration_sec": 30,
  "total_messages": 150,
  "avg_rtt_ms": 120.5,
  "min_rtt_ms": 85,
  "max_rtt_ms": 450,
  "p50_rtt_ms": 115,
  "p90_rtt_ms": 200,
  "packet_loss_pct": 2.5,
  "errors": []
}
```

**Escenarios**:

**SC-NET-01-01: Loadgen conecta y completa handshake**
```
Given: servidor TalkGo corriendo en localhost:8080
When:  go run ./cmd/loadgen -server localhost:8080 -duration 5s es ejecutado
Then:  el loadgen conecta WebSocket a ws://localhost:8080/ws/<roomID>
And:   envía mensaje join
And:   recibe joined con sessionID no vacío
And:   envía offer SDP
And:   recibe answer SDP
And:   completa el handshake en ≤5s
```

**SC-NET-01-02: Loadgen emite reporte al finalizar**
```
Given: sesión de loadgen completada (5s)
When:  el programa termina
Then:  stdout contiene un JSON con todos los campos de reporte
And:   los valores numéricos son plausibles (0 < avg_rtt_ms < 10000, 0 ≤ packet_loss_pct ≤ 100)
```

**SC-NET-01-03: Loadgen reporta error si el servidor no está disponible**
```
Given: ningún servidor en localhost:8080
When:  go run ./cmd/loadgen es ejecutado
Then:  el programa falla con error claro: "connection refused"
And:   exit code != 0
```

---

### REQ-NET-02: Scripts de simulación de red

Los scripts en `scripts/network-test/` MUST aplicar y remover restricciones de red para simular condiciones realistas.

**Windows (`simulate-4g.ps1`)** MUST usar `netsh` para:
- Limitar ancho de banda (inbound/outbound)
- NO usar `tc` (no disponible en Windows)
- Soportar perfiles via parámetro `-Profile` o config directa via `-Bandwidth`, `-LatencyMs`, `-LossPct`
- Soportar `-Reset` para eliminar todas las restricciones
- Requerir permisos de administrador y verificar al inicio

**Linux (`simulate-4g.sh`)** MUST usar `tc` y `netem` para:
- Aplicar delay, pérdida de paquetes, y rate limiting en interfaz `eth0` (configurable vía `-Interface`)
- Soportar perfiles via parámetro `-Profile` o flags directos
- Soportar `-Reset` para eliminar reglas `tc`
- Requerir `sudo` y verificar disponibilidad de `iproute2`

Sintaxis común:
```bash
# Aplicar perfil 4G
./simulate-4g.sh -Profile 4g

# Aplicar configuración manual
./simulate-4g.sh -Interface wlan0 -LatencyMs 150 -LossPct 3 -RateMbps 5

# Reset
./simulate-4g.sh -Reset
```

**Escenarios**:

**SC-NET-02-01: Simulate-4g.ps1 aplica restricciones en Windows**
```
Given: Windows 10/11 con PowerShell como administrador
When:  .\scripts\network-test\simulate-4g.ps1 -Profile 4g es ejecutado
Then:  netsh int tcp set global autotuninglevel=disabled
And:   netsh advfirewall ... (o equivalente) limita throughput
And:   script retorna exit code 0
```

**SC-NET-02-02: Simulate-4g.sh aplica y remueve restricciones en Linux**
```
Given: Linux con iproute2 instalado y sudo
When:  ./scripts/network-test/simulate-4g.sh -Profile 4g es ejecutado
Then:  tc qdisc add dev eth0 root netem delay 50ms loss 5%
And:   tc qdisc add dev eth0 root tbf rate 10mbit (o equivalente)
And:   el script retorna exit code 0
When:  ./scripts/network-test/simulate-4g.sh -Reset es ejecutado
Then:  tc qdisc del dev eth0 root
And:   retorna exit code 0
```

---

### REQ-NET-03: Perfiles de red predefinidos

El directorio `scripts/network-test/configs/` MUST contener archivos YAML con perfiles de red. Cada archivo MUST tener el siguiente formato:

```yaml
name: "4g"
description: "4G móvil estándar — 100ms RTT, 5% pérdida, 10Mbps"
bandwidth_mbps: 10
rtt_ms: 100
loss_pct: 5
jitter_ms: 10
interface: "eth0"  # default
```

Perfiles requeridos:

| Archivo | Bandwidth | RTT | Loss | Jitter | Caso de uso |
|---------|-----------|-----|------|--------|-------------|
| `4g.yml` | 10 Mbps | 100 ms | 5% | 10 ms | Red móvil estándar |
| `wifi-cafe.yml` | 5 Mbps | 150 ms | 8% | 20 ms | WiFi pública congestionada |
| `wifi-home.yml` | 50 Mbps | 20 ms | 1% | 2 ms | WiFi residencial típica |
| `wan-lossy.yml` | 2 Mbps | 300 ms | 15% | 30 ms | WAN con pérdida severa |

**Escenarios**:

**SC-NET-03-01: Todos los perfiles existen y son YAML válidos**
```
Given: scripts/network-test/configs/
When:  se listan los archivos *.yml
Then:  4g.yml, wifi-cafe.yml, wifi-home.yml, wan-lossy.yml existen
And:   cada archivo es YAML válido (verificable con python -c "import yaml; yaml.safe_load(open(f))")
And:   cada archivo contiene todos los campos: name, description, bandwidth_mbps, rtt_ms, loss_pct, jitter_ms
```

---

### REQ-NET-04: Script run-test-session

`scripts/network-test/run-test-session.sh` MUST automatizar una sesión de prueba completa:

1. Aplicar perfil de red (vía simulate-4g)
2. Iniciar servidor TalkGo (vía `go run ./cmd/server`)
3. Ejecutar loadgen peer con duración configurada
4. Parsear los logs del servidor para extraer métricas de latencia
5. Generar reporte consolidado en stdout y archivo
6. Limpiar (matar server, resetear red)

Soporta flags:
```
-Profile string      Perfil de red (default "wifi-home")
-Duration duration   Duración de la sesión (default 60s)
-Output string       Archivo de reporte (default "./report-<timestamp>.json")
-SkipSimulation      No aplicar simulación de red (para baseline)
```

El reporte generado MUST tener este formato:

```json
{
  "timestamp": "2026-06-11T12:00:00Z",
  "profile": "4g",
  "duration_sec": 60,
  "server_logs": {
    "total_chunks": 450,
    "chunks_ok": 430,
    "chunks_error": 20,
    "error_rate_pct": 4.4,
    "latency_p50_ms": 520,
    "latency_p90_ms": 980,
    "min_chunk_ms": 310,
    "max_chunk_ms": 1450,
    "total_chunks_AtoB": 230,
    "total_chunks_BtoA": 220
  },
  "loadgen": {
    "avg_rtt_ms": 115.2,
    "p50_rtt_ms": 110,
    "p90_rtt_ms": 200,
    "packet_loss_pct": 4.8
  },
  "status": "ok",
  "notes": []
}
```

Donde `status` es:
- `"ok"` — error_rate ≤ 5% y latency_p90 ≤ 1500ms
- `"degraded"` — error_rate 5-15% o latency_p90 1500-2500ms
- `"failed"` — error_rate > 15% o latency_p90 > 2500ms o el loadgen no pudo conectar

**Escenarios**:

**SC-NET-04-01: run-test-session produce reporte completo**
```
Given: Linux con iproute2, Go 1.23+, y servidor TalkGo compilable
When:  ./scripts/network-test/run-test-session.sh -Profile wifi-home -Duration 10s -SkipSimulation
Then:  servidor TalkGo inicia en background (PID capturado)
And:   loadgen ejecuta sesión de 10s
And:   logs del servidor son parseados con jq
And:   reporte JSON es escrito a stdout
And:   reporte contiene todos los campos de server_logs y loadgen
And:   servidor es terminado al finalizar
```

**SC-NET-04-02: run-test-session maneja error de conexión**
```
Given: ningún servidor disponible en el puerto default
When:  run-test-session.sh intenta ejecutar
Then:  reporte tiene status: "failed"
And:   notes contiene descripción del error
And:   exit code no es cero
```

---

## Bug Fixes Sprint 3

Los siguientes bugs se corrigen como parte del Sprint 4. Ver el proposal para detalle completo de cada fix.

### REQ-FIX-01: Copylocks en ListExpired (CRIT-01)

`driven.RoomRepository.ListExpired` MUST cambiar su firma de `[]room.Room` a `[]*room.Room`. Todos los callers (implementación en repository, tests, y `sweepExpiredRooms` en service) MUST ser actualizados.

**Escenarios**:

**SC-FIX-01-01: ListExpired firma actualizada**
```
Given: internal/ports/driven/room_repository.go
When:  ListExpired es declarado
Then:  retorna []*room.Room (no []room.Room)
```

**SC-FIX-01-02: golangci-lint pasa sin copylocks**
```
Given: todo el código actualizado
When:  golangci-lint run ./... es ejecutado
Then:  zero issues de tipo copylocks
```

---

### REQ-FIX-02: gofmt violations (WARN-01)

`gofmt -w` MUST ser corrido en los 5 archivos:
- `internal/adapters/signaling/hub.go`
- `internal/ports/driven/mocks/mock_room_repository.go`
- `internal/adapters/http/server_test.go`
- `internal/app/roomsvc/service_sprint3_test.go`
- `internal/domain/room/room.go`

**Escenarios**:

**SC-FIX-02-01: gofmt produce zero cambios**
```
Given: repositorio con cambios aplicados
When:  gofmt -d -l es ejecutado en los 5 archivos
Then:  output vacío (todos están formateados correctamente)
```

---

### REQ-FIX-03: exitAfterDefer (WARN-03)

`cmd/server/main.go:60` MUST reemplazar `os.Exit(1)` por `return 1` para permitir que `defer cancel()` se ejecute.

**Escenarios**:

**SC-FIX-03-01: exitAfterDefer eliminado**
```
Given: cmd/server/main.go
When:  go vet ./cmd/server es ejecutado
Then:  zero resultados para exitAfterDefer
```

---

### REQ-FIX-04: TestHub_PeerLeft_NotifiedOnDisconnect (WARN-06)

El test `TestHub_PeerLeft_NotifiedOnDisconnect` MUST usar `t.Errorf` en lugar de `t.Logf` cuando no recibe el mensaje `peer-left`.

**Escenarios**:

**SC-FIX-04-01: Test usa t.Errorf**
```
Given: internal/adapters/signaling/hub_test.go
When:  TestHub_PeerLeft_NotifiedOnDisconnect es ejecutado
Then:  si peer-left no se recibe, el test falla con t.Errorf
And:   si peer-left se recibe, el test pasa (t.Logf para información adicional OK)
```

---

## Delta Markers

### New Capabilities (not in any previous spec)

| Capability | Requirements | Scenarios |
|------------|-------------|-----------|
| `latency-instrumentation` | REQ-LAT-01, REQ-LAT-02, REQ-LAT-03, REQ-LAT-04, REQ-LAT-05 | SC-LAT-02-01 → SC-LAT-05-01 |
| `network-testing` | REQ-NET-01, REQ-NET-02, REQ-NET-03, REQ-NET-04 | SC-NET-01-01 → SC-NET-04-02 |

### Modified Capabilities

| Capability | Previous Spec | What Changes |
|------------|--------------|--------------|
| `translation` | `specs/translation/spec.md` (Sprint 2) — REQ-01 a REQ-14, SC-01 a SC-10 | El pipeline `runHalf()` ahora instrumenta cada etapa con timestamps (REQ-LAT-03) y emite logs `chunk_latency` por chunk completado. Se emiten eventos de sesión estructurados (REQ-LAT-04). Contadores de chunk ok/error agregados (REQ-LAT-05). Requisitos y escenarios existentes no se modifican — solo se agrega observabilidad. |

### Infrastructure Changes (no spec change required per proposal)

| Item | Description |
|------|-------------|
| **Logging modernization** | `slog.NewJSONHandler` reemplaza `NewTextHandler` en `main.go`. Mensajes estandarizados a snake_case. Field `component` obligatorio. Cubierto por REQ-LOG-01, REQ-LOG-02, REQ-LOG-03. |
| **Onboarding documentation** | README.md expandido con prerequisites, setup, arquitectura, comandos, troubleshooting. Sin cambios de spec — documentación pura. |
| **Bug fixes Sprint 3** | CRIT-01 (REQ-FIX-01), WARN-01 (REQ-FIX-02), WARN-03 (REQ-FIX-03), WARN-06 (REQ-FIX-04). Arreglan comportamiento para cumplir specs existentes. |

---

## Acceptance Criteria Matrix

| CA | Descripción | REQ-LOG | REQ-LAT | REQ-NET | REQ-FIX |
|----|------------|:-------:|:-------:|:-------:|:-------:|
| CA-01 | Latencia e2e p50 < 1.0s medida con timestamps | | ✓ (LAT-01, LAT-02, LAT-03) | | |
| CA-02 | Latencia e2e p90 < 1.5s medida con timestamps | | ✓ (LAT-01, LAT-02, LAT-03) | | |
| CA-05 | Errores de pipeline < 2% del total de chunks | | ✓ (LAT-04, LAT-05) | | |
| CA-07 | Sistema funciona correctamente en redes 4G y WiFi | | | ✓ (NET-01, NET-02, NET-03, NET-04) | |
| NFR-01 | `go vet` y `golangci-lint` pasan con zero issues | | | | ✓ (FIX-01, FIX-02, FIX-03, FIX-04) |
| NFR-02 | Coverage ≥ 80% en archivos modificados | ✓ | ✓ | ✓ | ✓ |

### Sprint 4 DoD — Mapeo a Requirements

| DoD Item (del proposal) | Cubierto por |
|------------------------|-------------|
| `LatencyTracker` emite logs JSON con timestamps | REQ-LAT-02, SC-LAT-02-03, REQ-LAT-03 |
| Logs incluyen `total_ms` y `stages.*_ms` | REQ-LAT-03, SC-LAT-03-01 |
| Eventos de sesión emitidos | REQ-LAT-04, SC-LAT-04-01 → SC-LAT-04-06 |
| Chunks ok vs error contabilizados | REQ-LAT-05, SC-LAT-05-01 |
| `slog.NewJSONHandler` reemplaza text handler | REQ-LOG-01, SC-LOG-01-01 |
| Mensajes snake_case con field component | REQ-LOG-02, SC-LOG-02-01; REQ-LOG-03, SC-LOG-03-01 |
| Scripts de simulación (Windows .ps1, Linux .sh) | REQ-NET-02, SC-NET-02-01, SC-NET-02-02 |
| Loadgen peer en cmd/loadgen/ | REQ-NET-01, SC-NET-01-01, SC-NET-01-02 |
| Perfiles de red predefinidos | REQ-NET-03, SC-NET-03-01 |
| `run-test-session` produce reporte | REQ-NET-04, SC-NET-04-01, SC-NET-04-02 |
| CRIT-01: ListExpired → `[]*room.Room` | REQ-FIX-01, SC-FIX-01-01, SC-FIX-01-02 |
| WARN-01: gofmt en 5 archivos | REQ-FIX-02, SC-FIX-02-01 |
| WARN-03: exitAfterDefer corregido | REQ-FIX-03, SC-FIX-03-01 |
| WARN-06: t.Logf → t.Errorf | REQ-FIX-04, SC-FIX-04-01 |

---

## Constraints & NFRs

| ID | Constraint |
|----|------------|
| NFR-01 | `go vet ./...` y `golangci-lint run ./...` MUST pasar con zero issues en todos los archivos modificados |
| NFR-02 | Coverage ≥ 80% en archivos modificados, medido con `go test -cover`. Coverage existente no disminuye. |
| NFR-03 | Sin nuevas dependencias externas de Go para instrumentación y logging — solo stdlib (`log/slog`, `time`, `sync`, `encoding/json`) |
| NFR-04 | LatencyTracker NO MUST usar high-res timers — `time.Now()` con resolución de microsegundos es suficiente (overhead < 1µs) |
| NFR-05 | Los scripts de network testing NO MUST requerir npm, Python, o cualquier runtime fuera de Go y herramientas del sistema operativo |
| NFR-06 | Arquitectura Hexagonal: `internal/domain/` MUST NOT importar `internal/adapters/` — verificado por `golangci-lint` (depguard) |
| NFR-07 | Loadgen peer (`cmd/loadgen/`) MUST reutilizar los puertos `driven.WebRTCPeer` y `driven.Translator` existentes — no duplicar lógica |
| NFR-08 | `cmd/loadgen/` NO MUST depender de paquetes internos de `internal/app/` o `internal/adapters/` — es un programa externo que solo usa los ports driven |

---

## Error Catalog

| Sentinel / Constante | Situación |
|----------------------|-----------|
| `ErrMissingLang` | `SignalingMessage.Lang` vacío en `JoinRoom` (ya existente, sin cambios) |
| `ErrRoomFull` | Se intenta unir un tercer participante (ya existente, sin cambios) |
| `ErrNilDependency` | `NewService` recibe `nil` (ya existente, sin cambios) |
| *(nuevo)* `ErrShortCodeExhausted` | Colisión de short code tras 5 reintentos (existente Sprint 3, sin cambios) |

No se agregan nuevos centinelas de error en Sprint 4 — los errores de instrumentación se registran en logs pero no se exponen al dominio ni al cliente.

---

## Test Infrastructure

- Los tests de instrumentación (`pipeline_test.go` o `latency_test.go`) MUST verificar la salida de logs capturando `slog` con un `bytes.Buffer` y `slog.NewJSONHandler`.
- Los tests de `LatencyTracker` MUST verificar allocaciones con `testing.AllocsPerRun`.
- Los tests de eventos de sesión MUST usar un `slog.Handler` que capture los eventos emitidos y los exponga para assertion.
- Los tests del loadgen peer son scripts de integración en `scripts/network-test/` (no tests Go unitarios).
- Benchmark de overhead del tracker: `BenchmarkLatencyTracker` MUST ejecutar ≥1000 iteraciones midiendo allocaciones y tiempo por chunk.
