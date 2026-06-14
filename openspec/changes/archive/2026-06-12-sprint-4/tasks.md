# Sprint 4 Task Breakdown: Polish y Alpha — Instrumentación, Logging, Network Testing

**Change**: sprint-4
**Status**: tasks
**Date**: 2026-06-11

---

## Legend

- TDD pairs: `TEST` task precedes its `IMPL` task — never implement before the test exists
- Atomic: each task is ~30–90 minutes of focused work
- `(requires TASK-XXX)` = hard dependency, do not start before the prerequisite is done
- Workstreams A/B/C can run in parallel after Phases 1–2 complete
- Workstream C (README) must wait until Workstream A code is stable

---

## Fase 1: Bug Fixes Sprint 3 — CRIT-01, WARN-01, WARN-03, WARN-06

Archivos: `internal/ports/driven/room_repository.go`, `internal/app/roomsvc/service.go`, `internal/app/roomsvc/repository.go`, `internal/app/roomsvc/repository_test.go`, `internal/app/roomsvc/service_sprint3_test.go`, `internal/app/roomsvc/pipeline_test.go`, `internal/adapters/signaling/hub.go`, `internal/adapters/signaling/hub_sprint3_test.go`, `internal/adapters/http/server.go`, `internal/adapters/http/server_test.go`, `internal/ports/driven/mocks/mock_room_repository.go`, `internal/domain/room/room.go`, `cmd/server/main.go`

- [x] TASK-001: TEST - CRIT-01: `TestListExpired_ReturnsPointers` — verificar que `golangci-lint` no reporta copylocks en ninguna llamada a `ListExpired`. Audit de TODOS los callers con `grep -r "ListExpired" .` y confirmar que retornan `[]*room.Room`. Escribir test que compila contra la firma de pointer (type assertion). (requiere nada — audit codebase)
- [x] TASK-002: IMPL - CRIT-01: Corregir cualquier caller que todavía pase `room.Room` por valor donde `ListExpired` retorna `[]*room.Room`. Verificar en `internal/app/roomsvc/repository.go:101`, `internal/app/roomsvc/service.go:409`, y tests que no haya `range expired` sobre valores. Ejecutar `golangci-lint run ./...` y confirmar zero copylocks. (requiere TASK-001)
- [x] TASK-003: TEST - WARN-01: `TestGofmt_NoChanges` — ejecutar `gofmt -d -l` en los 5 archivos indicados, verificar que el output está vacío (test de formato que falla si hay violaciones). (requiere nada)
- [x] TASK-004: IMPL - WARN-01: Ejecutar `gofmt -w` en los 5 archivos: `internal/adapters/signaling/hub.go`, `internal/ports/driven/mocks/mock_room_repository.go`, `internal/adapters/http/server_test.go`, `internal/app/roomsvc/service_sprint3_test.go`, `internal/domain/room/room.go`. Verificar con `go fmt ./...` que no produce diff. (requiere TASK-003)
- [x] TASK-005: TEST - WARN-03: `TestExitAfterDefer_Eliminated` — verificar con `go vet ./cmd/server` que no reporta `exitAfterDefer`. Test que compila `run()` function y confirma que retorna `int` en lugar de llamar `os.Exit()`. (requiere nada)
- [x] TASK-006: IMPL - WARN-03: En `cmd/server/main.go:64` y `:82`, reemplazar `os.Exit(1)` con `return 1`. `main()` llama `os.Exit(run())`, así que `return 1` logra el mismo código de salida pero permite que `defer cancel()` se ejecute. (requiere TASK-005)
- [x] TASK-007: TEST - WARN-06: `TestHub_PeerLeft_NotifiedOnDisconnect_UsesErrorf` — inspeccionar `internal/adapters/signaling/hub_sprint3_test.go` y verificar que la línea que maneja el timeout en el test existente usa `t.Errorf` (no `t.Logf`). Si ya usa `t.Errorf`, el test pasa; si no, lo corrige. (requiere nada)
- [x] TASK-008: IMPL - WARN-06: Si después de TASK-007 se confirma que el test ya usa `t.Errorf`, este task es NO-OP (verificación documentada). Si no, cambiar `t.Logf` a `t.Errorf` en `TestHub_PeerLeft_NotifiedOnDisconnect` cuando no recibe `peer-left`. (requiere TASK-007)

---

## Fase 2: Logging Modernization — JSON handler + snake_case + component field

Archivos: `cmd/server/main.go`, `internal/adapters/signaling/hub.go`, `internal/adapters/signaling/client.go`, `internal/adapters/http/server.go`, `internal/app/roomsvc/service.go`

- [x] TASK-009: TEST - LOG: `TestJSONHandler_ConfiguredAtStartup` — test que ejecuta `run()` con mock de stdout capturado en `bytes.Buffer`, verifica que el primer log emitido es JSON válido. Test que `-log-level=debug` permite mensajes `slog.Debug`. (requiere TASK-006)
- [x] TASK-010: IMPL - LOG: `cmd/server/main.go` — migrar de `slog.NewTextHandler` a `slog.NewJSONHandler` con `HandlerOptions{Level: level}`. Agregar flag `-log-level` con valores `debug/info/warn/error`. Mensajes modernizados:
  - `"server_starting"` con `component: "main"`, `"addr"`
  - `"shutdown_starting"` con `component: "main"`
  - `"service_creation_failed"` con `component: "main"`, `err`
  - `"server_error"` con `component: "http"`, `err`
  (requiere TASK-009)
- [x] TASK-011: TEST - LOG: `TestHubLogs_UseSnakeCaseAndComponent` — test que captura logs del hub en un buffer, verifica que todas las llamadas a `slog.Error`/`slog.Info` usan identificador snake_case e incluyen `component: "hub"`. (requiere TASK-004 — gofmt ya aplicado)
- [x] TASK-012: IMPL - LOG: `internal/adapters/signaling/hub.go` + `client.go` — modernizar TODAS las llamadas a slog al formato estándar:
  - `on_disconnect_error` con `component: "hub"`, `session_id`, `err`
  - `ws_upgrade_failed` con `component: "hub"`, `err`
  - `signal_response_marshal_error` con `component: "hub"`, `err`
  - `notify_session_marshal_error` con `component: "hub"`, `err`
  - `ws_write_error` con `component: "hub"`, `err`
  - `ws_read_error` con `component: "hub"`, `err`
  (requiere TASK-011)
- [x] TASK-013: TEST - LOG: `TestServiceLogs_UseSnakeCaseAndComponent` — test que captura logs del service en buffer, verifica snake_case + `component: "service"`. (requiere TASK-002 — CRIT-01 fix aplicado)
- [x] TASK-014: IMPL - LOG: `internal/app/roomsvc/service.go` — modernizar TODAS las llamadas a slog:
  - `close_session_error` con `component: "service"`, `session_id`, `err`
  - `grace_timer_delete_error` con `component: "service"`, `room_id`, `err`
  - `sweep_list_error` con `component: "service"`, `err`
  - `sweep_delete_error` con `component: "service"`, `room_id`, `err`
  (requiere TASK-013)
- [x] TASK-015: TEST - LOG: `TestHTTPLogs_UseSnakeCaseAndComponent` — test que captura logs HTTP, verifica snake_case + `component: "http"`. (requiere TASK-004)
- [x] TASK-016: IMPL - LOG: `internal/adapters/http/server.go` — modernizar TODAS las llamadas a slog:
  - `http_listening` con `component: "http"`, `addr`
  - `http_shutdown` con `component: "http"`
  - `http_stopped` con `component: "http"`
  - `create_room_error` con `component: "http"`, `err`
  - `delete_room_error` con `component: "http"`, `err`
  - `find_by_code_error` con `component: "http"`, `err`
  - `ws_handler_error` con `component: "http"`, `err`
  (requiere TASK-015)

---

## Fase 3: LatencyTracker struct + sync.Pool (`latency.go`)

Archivos: `internal/app/roomsvc/latency.go` (NUEVO), `internal/app/roomsvc/latency_test.go` (NUEVO), `internal/domain/session/session.go`

- [x] TASK-017: TEST - LAT: `TestChunkLatency_ValueSemantics` — test que verifica que `ChunkLatency` se puede copiar por valor sin efectos secundarios (es un struct value). Verificar que `StageTimings` backing array se reusa correctamente al hacer `[:0]`. (requiere nada)
- [x] TASK-018: IMPL - LAT: `internal/app/roomsvc/latency.go` — crear structs:
  - Constantes: `StageCapture`, `StageDecode`, `StageTranslate`, `StageEncode`, `StageSend`
  - `ChunkLatency` (value struct): `ChunkID string`, `StageTimings []StageTiming`, `Total time.Duration`
  - `StageTiming`: `Stage string`, `StartAt time.Time`, `EndAt time.Time`, `Duration time.Duration`
  (requiere TASK-017)
- [x] TASK-019: TEST - LAT: `TestLatencyTracker_StartEndStages`, `TestLatencyTracker_ChunkIDIncrements`, `TestLatencyTracker_PoolReusesInstances` — tres tests unitarios:
  - StartStage→EndStage produce Duration correcto
  - Reset incrementa chunkID, trackers independientes
  - sync.Pool retorna el mismo puntero después de Emit+Reset
  (requiere TASK-018)
- [x] TASK-020: IMPL - LAT: `internal/app/roomsvc/latency.go` — implementar `LatencyTracker` struct con `sync.Pool`:
  - `NewLatencyTracker()`: pool con `New` que retorna `&ChunkLatency{StageTimings: make([]StageTiming, 0, 5)}`
  - `StartStage(stage)`: mutex Lock, append StageTiming con StartAt=time.Now()
  - `EndStage(stage)`: mutex Lock, buscar último StageTiming sin EndAt, setear EndAt y calcular Duration
  - `Reset()`: chunkID++, pool.Get(), ChunkID = strconv.FormatInt, StageTimings[:0], Total=0
  - `Emit(ctx, half, roomID, status)`: calcular Total (suma de durations), emitir `slog.LogAttrs(ctx, slog.LevelInfo, "chunk_latency", ...)` con todos los campos estructurados, pool.Put(current)
  (requiere TASK-019)
- [x] TASK-021: TEST - LAT: `BenchmarkLatencyTracker_Allocations` — benchmark con `testing.AllocsPerRun` ≥1000 iteraciones, verifica allocs < 3 por chunk. `TestLatencyTracker_ConcurrentSafety` — dos trackers operan independientemente en paralelo. (requiere TASK-020)
- [x] TASK-022: IMPL - LAT: `internal/domain/session/session.go` — agregar campo `ErrorCount int` al struct `Session`. Este campo se incrementa en el pipeline cuando una etapa falla y se usa en el log `session_error`. (requiere nada — domain change aislado)

---

## Fase 4: Pipeline instrumentation — stages + contadores atómicos

Archivos: `internal/app/roomsvc/pipeline.go`, `internal/app/roomsvc/pipeline_test.go` (MODIFICADO)

- [x] TASK-023: TEST - PIPELINE: `TestPipelineHalf_StructWithTracker` — test de compilación que verifica que `pipelineHalf` tiene campos `tracker *LatencyTracker`, `totalChunks atomic.Int64`, `errorChunks atomic.Int64`, `dir string`. (requiere TASK-020, TASK-022)
- [x] TASK-024: IMPL - PIPELINE: modificar `pipelineHalf` struct en `pipeline.go`:
  - Agregar `tracker *LatencyTracker`
  - Agregar `totalChunks atomic.Int64`, `errorChunks atomic.Int64`
  - Agregar `dir string` (valor `"AtoB"` o `"BtoA"`)
  - Importar `sync/atomic`
  (requiere TASK-023)
- [x] TASK-025: TEST - PIPELINE: `TestStartPipeline_EmitsSessionEvents` — test que captura logs durante `startPipeline` y verifica que se emiten `session_event` con `session_start` y `pipeline_start`. Verificar campos: `session_id`, `user_id`, `lang`, `room_id`, `component`. (requiere TASK-024)
- [x] TASK-026: IMPL - PIPELINE: modificar `startPipeline()` en `pipeline.go` para emitir eventos de sesión:
  - Wire `NewLatencyTracker()` en ambos `pipelineHalf` (AtoB y BtoA)
  - Emitir `session_start` para sessA y sessB con `slog.LogAttrs` (room_id, session_id, user_id, lang, component: "service")
  - Emitir `pipeline_start` con (room_id, sessA, sessB, langA, langB, component: "pipeline")
  - Agregar goroutine con `p.wg.Wait()` que emite `pipeline_stop` con `total_chunks_AtoB`, `total_chunks_BtoA`, component: "pipeline"
  (requiere TASK-025)
- [x] TASK-027: TEST - PIPELINE: `TestRunHalf_InstrumentedStages` — test con mocks rápidos que verifica que `runHalf()` llama a `StartStage`/`EndStage` para las 5 etapas (capture, decode, translate, encode, send). Capturar logs y verificar `chunk_latency` emitido con campos `total_ms`, `stages.*_ms`, `status`. (requiere TASK-026)
- [x] TASK-028: IMPL - PIPELINE: instrumentar `runHalf()` con las 5 etapas siguiendo el diseño "ABSOLUTELY FINAL DESIGN":
  - Capture: `tracker.Reset()` por frame, `StartStage(StageCapture)` → `EndStage(StageCapture)` (instantáneo)
  - Decode: `StartStage(StageDecode)` antes de `s.codec.Decode`, `EndStage(StageDecode)` después
  - Translate: `StartStage(StageTranslate)` antes de `s.translator.TranslateStream`, `EndStage(StageTranslate)` después
  - Encode: `StartStage(StageEncode)` antes de `s.codec.Encode`, `EndStage(StageEncode)` después
  - Send: `StartStage(StageSend)` antes de `s.peer.SendAudio`, `EndStage(StageSend)` después
  - En cada error: `half.errorChunks.Add(1)`, `tracker.Emit(ctx, half.dir, roomID, "error")`, return
  - En éxito: `half.totalChunks.Add(frameCount)`, `tracker.Emit(ctx, half.dir, roomID, "ok")`
  - session_error: `s.logSessionError()` helper en errores de translate/encode
  (requiere TASK-027)
- [x] TASK-029: TEST - PIPELINE: `TestPipeline_ErrorChunkLogged` — test con mock `AudioCodec.Decode` que falla, verifica que `chunk_latency` tiene `status: "error"` y `stages.decode_ms` refleja tiempo hasta el error. (requiere TASK-028)
- [x] TASK-030: IMPL - PIPELINE: agregar helper `logSessionError(sessionID, stage, err)` en `pipeline.go`:
  ```go
  func (s *Service) logSessionError(sessionID, stage string, err error) {
      s.mu.RLock()
      sess, ok := s.sessions[sessionID]
      s.mu.RUnlock()
      if !ok { return }
      sess.ErrorCount++
      slog.LogAttrs(context.Background(), slog.LevelError, "session_event",
          slog.String("event", "session_error"),
          slog.String("session_id", sessionID),
          slog.String("error", err.Error()),
          slog.Int("error_count", sess.ErrorCount),
          slog.String("stage", stage),
          slog.String("component", "pipeline"),
      )
  }
  ```
  (requiere TASK-029)

---

## Fase 5: Session events — session_end en LeaveRoom, OnDisconnect, grace timer

Archivos: `internal/app/roomsvc/service.go`, `internal/app/roomsvc/service_test.go`

- [x] TASK-031: TEST - SESSION: `TestLeaveRoom_EmitsSessionEnd` — test que captura logs durante `LeaveRoom` y verifica `session_event` con `event: "session_end"`, `reason: "voluntary"`, `duration_sec`. (requiere TASK-014 — logging modernizado)
- [x] TASK-032: IMPL - SESSION: en `Service.LeaveRoom()`, antes de la limpieza de sesión, emitir `session_end`:
  ```go
  slog.LogAttrs(ctx, slog.LevelInfo, "session_event",
      slog.String("event", "session_end"),
      slog.String("session_id", sessID),
      slog.Int64("duration_sec", int64(time.Since(sess.JoinedAt).Seconds())),
      slog.String("reason", "voluntary"),
      slog.String("component", "service"),
  )
  ```
  (requiere TASK-031)
- [x] TASK-033: TEST - SESSION: `TestOnDisconnect_EmitsSessionEnd` — test que llama `OnDisconnect` con sessionID válida, verifica `session_event` con `event: "session_end"`, `reason: "disconnect"`. (requiere TASK-014)
- [x] TASK-034: IMPL - SESSION: en `Service.OnDisconnect()`, después de lookup de sesión exitoso, emitir `session_end` con `reason: "disconnect"`. En el callback del grace timer (`time.AfterFunc`), emitir `session_end` con `reason: "timeout"` antes de `DeleteRoom`. (requiere TASK-033)
- [x] TASK-035: TEST - SESSION: `TestGraceTimer_EmitsSessionEndTimeout` — test con `GracePeriod=1ms` que verifica que el grace timer emite `session_end` con `reason: "timeout"` al expirar. (requiere TASK-034)
- [x] TASK-036: TEST - PIPELINE: `TestPipeline_Counters_StopEvent` — test que verifica que `pipeline_stop` contiene `total_chunks_AtoB` y `total_chunks_BtoA` correctos. Pipeline con 3 frames ok en AtoB, 2 frames ok en BtoA → `total_chunks_AtoB == 3`, `total_chunks_BtoA == 2`. (requiere TASK-028, TASK-030)

---

## Fase 6: Log capture infrastructure + tests de integración

Archivos: `internal/app/roomsvc/latency_test.go`, `internal/app/roomsvc/pipeline_test.go`, `internal/app/roomsvc/service_test.go`

- [x] TASK-037: IMPL - TEST: Crear helper `newLogCapture(t) (*bytes.Buffer, *slog.Logger)` en `latency_test.go`:
  ```go
  func newLogCapture(t *testing.T) (*bytes.Buffer, *slog.Logger) {
      t.Helper()
      var buf bytes.Buffer
      logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
      return &buf, logger
  }
  ```
  Helper `withLogCapture(t, fn)` que setea default logger temporalmente con defer restore. (requiere TASK-020)
- [x] TASK-038: TEST - LAT: `TestLatencyTracker_EmitLogsCorrectFields` — usar `newLogCapture`, emitir un chunk, verificar JSON:
  - `msg == "chunk_latency"`
  - `component == "pipeline"`
  - `total_ms` presente y > 0
  - `stages` contiene `capture_ms`, `decode_ms`, `translate_ms`, `encode_ms`, `send_ms`
  - `chunk_id` presente
  - `status` presente
  (requiere TASK-037)
- [x] TASK-039: TEST - LAT: `TestLatencyTracker_EmitStatusError` — emitir con status `"error"`, verificar que el log contiene `status: "error"` y que campos de stages están presentes aunque algunos sean 0. (requiere TASK-037)
- [x] TASK-040: TEST - SESSION: Integrar `newLogCapture` en tests de sesión — `TestStartPipeline_EmitsSessionStart`, `TestPipelineStart_EmitsCorrectFields`, `TestPipelineStop_EmitsChunkCounters` como tests en `pipeline_test.go` que capturan logs reales del pipeline instrumentado. (requiere TASK-026, TASK-028, TASK-037)
- [x] TASK-041: TEST - INTEGRATION: `TestFullPipeline_LogCapture` — test de integración que: (1) crea service con mocks, (2) join room con 2 peers, (3) espera pipeline procese frames, (4) captura logs, (5) verifica que existen logs con `msg: "session_event"` y `msg: "chunk_latency"`, (6) verifica pipeline_stop tiene contadores. (requiere TASK-040)
- [x] TASK-042: VERIFY - NFR: Ejecutar `go vet ./...` y `golangci-lint run ./...` — confirmar zero issues en todos los archivos modificados. Ejecutar `go test -race -count=1 ./internal/...` — todos los tests pasan. Verificar coverage ≥ 80% en archivos modificados con `go test -cover`. (requiere TASK-008, TASK-012, TASK-014, TASK-016, TASK-028, TASK-041)

---

## Fase 7: Loadgen peer (Workstream B — Network Testing)

Archivos: `cmd/loadgen/main.go` (NUEVO), `cmd/loadgen/session.go` (NUEVO), `cmd/loadgen/audio.go` (NUEVO), `cmd/loadgen/report.go` (NUEVO)

- [x] TASK-043: IMPL - LOADGEN: `cmd/loadgen/main.go` — entry point, flag parsing:
  ```
  -server string      (default "localhost:8080")
  -room string        (default: auto-generado)
  -lang string        (default "es")
  -duration duration  (default 30s)
  -profile string     (default "wifi-home")
  -output string      (default: stdout)
  ```
  Resolver URL: si `-server` es `localhost:8080`, construir `ws://localhost:8080/ws/<roomID>`. Si `-room` vacío, crear room via POST `http://localhost:8080/rooms`. (requiere nada — workstream independiente)
- [x] TASK-044: IMPL - LOADGEN: `cmd/loadgen/session.go` — WebSocket connect + signaling protocol:
  - Conectar WS a `ws://<server>/ws/<roomID>`
  - Enviar `join {user_id, room_id, lang}`
  - Recibir `joined {sessionID}`
  - Enviar `offer` con SDP dummy
  - Recibir `answer`
  - Loop de pings a 20ms (50Hz) midiendo RTT
  - Enviar `leave` al finalizar, cerrar WS
  - RTT measurement: timestamp antes de send, timestamp después de receive
  (requiere TASK-043)
- [x] TASK-045: IMPL - LOADGEN: `cmd/loadgen/audio.go` — generador de tono sinusoidal PCM sintético:
  - Frecuencia: 440Hz (A4), sample rate 24kHz, frames de 20ms (480 samples)
  - Formato: PCM mono 16-bit signed little-endian
  - Función `GenerateFrame() []byte` que produce un frame
  - NO se usa en MVP WebSocket-only — para futuro WebRTC
  (requiere TASK-043)
- [x] TASK-046: IMPL - LOADGEN: `cmd/loadgen/report.go` — cálculo de estadísticas RTT:
  - `avg_rtt_ms`, `min_rtt_ms`, `max_rtt_ms`
  - `p50_rtt_ms`, `p90_rtt_ms` (sort slice y percentil)
  - `packet_loss_pct` = (sent - received) / sent * 100
  - `total_messages`, `duration_sec`, `errors[]`
  - Emitir JSON report a stdout (o archivo via `-output`)
  (requiere TASK-044)
- [x] TASK-047: TEST - LOADGEN: `TestLoadgen_ReportComputation` — test unitario que simula RTT measurements y verifica que p50/p90 se calculan correctamente. Dataset conocido: [85, 95, 100, 115, 130, 200, 450] → avg=167.9, min=85, max=450, p50=115, p90=200. (requiere TASK-046)
- [x] TASK-048: TEST - LOADGEN: `TestLoadgen_AudioFrame` — test que `GenerateFrame()` produce 480 samples (960 bytes para 16-bit stereo, o 480*2=960 bytes para mono 16-bit). Verificar que la amplitud máxima está dentro de rango [-32768, 32767]. (requiere TASK-045)
- [x] TASK-049: VERIFY - LOADGEN: Build check: `go build ./cmd/loadgen/` compila sin errores. `go vet ./cmd/loadgen/` pasa. Verificar que `cmd/loadgen/` NO importa ningún paquete `internal/` (NFR-08). (requiere TASK-046)

---

## Fase 8: Network simulation scripts (Workstream B — Network Testing)

Archivos: `scripts/network-test/simulate-4g.ps1` (NUEVO), `scripts/network-test/simulate-4g.sh` (NUEVO), `scripts/network-test/configs/4g.yml` (NUEVO), `scripts/network-test/configs/wifi-cafe.yml` (NUEVO), `scripts/network-test/configs/wifi-home.yml` (NUEVO), `scripts/network-test/configs/wan-lossy.yml` (NUEVO)

- [x] TASK-050: IMPL - NET: `scripts/network-test/configs/4g.yml` — perfil 4G: bandwidth 10Mbps, RTT 100ms, loss 5%, jitter 10ms.
- [x] TASK-051: IMPL - NET: `scripts/network-test/configs/wifi-cafe.yml` — perfil WiFi café: bandwidth 5Mbps, RTT 150ms, loss 8%, jitter 20ms.
- [x] TASK-052: IMPL - NET: `scripts/network-test/configs/wifi-home.yml` — perfil WiFi hogar: bandwidth 50Mbps, RTT 20ms, loss 1%, jitter 2ms.
- [x] TASK-053: IMPL - NET: `scripts/network-test/configs/wan-lossy.yml` — perfil WAN pérdida severa: bandwidth 2Mbps, RTT 300ms, loss 15%, jitter 30ms.
- [x] TASK-054: IMPL - NET: `scripts/network-test/simulate-4g.ps1` — Windows PowerShell script:
  - Parámetros: `-Profile`, `-Bandwidth`, `-LatencyMs`, `-LossPct`, `-Reset`
  - Verificar admin (require -RunAsAdministrator)
  - `-Reset`: restaurar `netsh int tcp set global autotuninglevel=normal`
  - Aplicar: `netsh int tcp set global autotuninglevel=disabled`, limitar ancho de banda vía advfirewall
  - Documentar limitación: netsh NO soporta pérdida/latencia (solo bandwidth)
  - Parseo YAML: PowerShell `ConvertFrom-Yaml` o parsing manual básico
  (requiere TASK-050)
- [x] TASK-055: IMPL - NET: `scripts/network-test/simulate-4g.sh` — Linux bash script:
  - Parámetros: `-Profile`, `-Interface` (default eth0), `-LatencyMs`, `-LossPct`, `-RateMbps`, `-Reset`
  - Verificar `sudo` y disponibilidad de `tc`
  - `-Reset`: `tc qdisc del dev $IFACE root 2>/dev/null`
  - Aplicar: `tc qdisc add dev $IFACE root handle 1: htb default 30` → `tc class add ... rate ${RATE}mbit` → `tc qdisc add ... netem delay ${LATENCY}ms loss ${LOSS}% 25%`
  - Parseo YAML con grep+awk (sin Python):
    ```bash
    parse_yaml_value() { grep "^${1}:" "$2" | awk '{print $2}'; }
    ```
  (requiere TASK-050)

---

## Fase 9: run-test-session script + network-test README (Workstream B)

Archivos: `scripts/network-test/run-test-session.sh` (NUEVO), `scripts/network-test/README.md` (NUEVO)

- [x] TASK-056: IMPL - NET: `scripts/network-test/run-test-session.sh` — script de automatización completa:
  1. Parsear args: `-Profile`, `-Duration`, `-Output`, `-SkipSimulation`
  2. Si no `-SkipSimulation`: aplicar perfil de red vía `simulate-4g.sh -Profile $PROFILE`
  3. Iniciar server: `go run ./cmd/server &` (capturar PID, trap para cleanup)
  4. `sleep 2` (esperar que server esté listo)
  5. Ejecutar loadgen: `go run ./cmd/loadgen -server localhost:8080 -duration $DURATION -profile $PROFILE > loadgen-report.json`
  6. Parsear logs del server con `jq`:
     - `jq 'select(.msg == "chunk_latency") | .total_ms' server.log | sort -n` → p50/p90
  7. Generar reporte consolidado JSON con `server_logs` + `loadgen` sections
  8. Status: `"ok"` (error_rate ≤ 5%, latency_p90 ≤ 1500ms), `"degraded"` (5-15%, 1500-2500ms), `"failed"` (>15% o >2500ms o loadgen error)
  9. Cleanup: `kill $SERVER_PID`, `simulate-4g.sh -Reset`
  (requiere TASK-046, TASK-055)
- [x] TASK-057: TEST - NET: `TestReport_StatusLogic` — test unitario que verifica la lógica de status del reporte:
  - error_rate=2%, latency_p90=800ms → `"ok"`
  - error_rate=10%, latency_p90=1800ms → `"degraded"`
  - error_rate=20% → `"failed"`
  - latency_p90=3000ms → `"failed"`
  - loadgen connection fail → `"failed"`
  Implementar en bash o Go script validation. (requiere TASK-056)
- [x] TASK-058: IMPL - NET: `scripts/network-test/README.md` — documentar:
  - Prerequisitos: Go 1.23+, Linux con iproute2 (o Windows con netsh), `jq`
  - Cómo usar `simulate-4g.sh` / `simulate-4g.ps1`
  - Cómo correr `run-test-session.sh` con ejemplos
  - Perfiles de red disponibles y qué simulan
  - Limitaciones: Windows no soporta pérdida/latencia, usar WSL2 o Linux
  - Cómo interpretar el reporte JSON
  - Troubleshooting: tc permission denied, netsh admin required, server port conflict
  (requiere TASK-054, TASK-055, TASK-056)

---

## Fase 10: Onboarding Documentation (Workstream C)

Archivos: `README.md` (MODIFICADO)

- [x] TASK-059: IMPL - DOCS: `README.md` — agregar secciones:
  1. **Prerequisites**: Go 1.23+, Node.js 20+, React Native CLI, Android SDK, Xcode, OpenAI API key
  2. **Quick Start**: clone → `make setup` → configurar `.env` → `go run ./cmd/server`
  3. **Architecture Overview**: Diagrama hexagonal ASCII:
     ```
     Driving Ports (HTTP/WS) → Service Layer (roomsvc) → Driven Ports (WebRTC/Translator/Codec/Repo)
     ```
     Explicación de capas (domain → ports → adapters → app), driving vs driven
  4. **Make Targets**: `make test`, `make lint`, `make run`, `make build`
  5. **Running Tests**: `go test -race ./...`, `go test -cover ./...`, `go test -v -run TestLatency`
  6. **Logging**: Formato JSON, filtrado con `jq`:
     ```bash
     go run ./cmd/server | jq 'select(.msg == "chunk_latency") | {total_ms, stages}'
     ```
  7. **Network Testing**: cómo usar `run-test-session.sh`, qué métricas esperar
  8. **Troubleshooting**: Pion ICE failures, OpenAI API key, CGO_ENABLED, port conflicts
  (requiere TASK-042 — Workstream A estable)
- [x] TASK-060: VERIFY - DOCS: Revisar README completo: verificar que todos los comandos funcionan, que los links son válidos, que el diagrama de arquitectura es correcto. `grep` para confirmar que no hay placeholders. (requiere TASK-059)

---

## Resumen de tasks por fase

| Fase | Workstream | Tasks | Descripción |
|------|-----------|-------|-------------|
| 1 | A | TASK-001..008 | Bug Fixes Sprint 3 (CRIT-01, WARN-01, WARN-03, WARN-06) |
| 2 | A | TASK-009..016 | Logging Modernization (JSON handler, snake_case, component) |
| 3 | A | TASK-017..022 | LatencyTracker struct + sync.Pool (latency.go) |
| 4 | A | TASK-023..030 | Pipeline instrumentation (stages, contadores, session_error) |
| 5 | A | TASK-031..036 | Session events (session_end voluntary/disconnect/timeout) |
| 6 | A | TASK-037..042 | Tests + log capture infrastructure + NFR verification |
| 7 | B | TASK-043..049 | Loadgen peer (cmd/loadgen/) |
| 8 | B | TASK-050..055 | Network simulation scripts (Windows .ps1, Linux .sh, configs) |
| 9 | B | TASK-056..058 | run-test-session script + network-test README |
| 10 | C | TASK-059..060 | Onboarding Documentation (README expansion) |

**Total: 60 tasks** (38 pares TDD + 22 standalone impl/verify)

---

## Total de tests por fase

| Fase | Tests | Tipo |
|------|-------|------|
| 1 | TASK-001, TASK-003, TASK-005, TASK-007 | Verificación lint + formato |
| 2 | TASK-009, TASK-011, TASK-013, TASK-015 | Log capture + snake_case assertions |
| 3 | TASK-017, TASK-019, TASK-021 | Unit tests: tracker, pool, allocs, concurrency |
| 4 | TASK-023, TASK-025, TASK-027, TASK-029 | Pipeline instrumentación + log fields |
| 5 | TASK-031, TASK-033, TASK-035, TASK-036 | Session events emission |
| 6 | TASK-037, TASK-038, TASK-039, TASK-040, TASK-041, TASK-042 | Log capture infrastructure + integración |
| 7 | TASK-047, TASK-048, TASK-049 | Loadgen report math + audio frame + build check |
| 8 | — | Scripts (no tests Go — validación manual) |
| 9 | TASK-057 | Report status logic |
| 10 | TASK-060 | README review |

**Total tests: 24** (unitarios + integración + verificación)

## Acceptance Criteria coverage

| CA | Descripción | Tasks que lo cubren |
|----|-------------|---------------------|
| CA-01 | Latencia e2e p50 < 1.0s medida con timestamps | TASK-020, TASK-028, TASK-038, TASK-041 |
| CA-02 | Latencia e2e p90 < 1.5s medida con timestamps | TASK-020, TASK-028, TASK-038, TASK-041 |
| CA-05 | Errores de pipeline < 2% del total de chunks | TASK-028, TASK-029, TASK-030, TASK-036, TASK-039 |
| CA-07 | Sistema funciona correctamente en redes 4G y WiFi | TASK-046, TASK-054, TASK-055, TASK-056, TASK-057 |
| NFR-01 | `go vet` y `golangci-lint` pasan con zero issues | TASK-001, TASK-002, TASK-003, TASK-004, TASK-005, TASK-006, TASK-007, TASK-008, TASK-042 |
| NFR-02 | Coverage ≥ 80% en archivos modificados | TASK-042 |

## Scenario coverage

| Scenario | Tasks |
|----------|-------|
| SC-LOG-01-01: JSON handler configurado | TASK-009, TASK-010 |
| SC-LOG-01-02: Log level configurable | TASK-009, TASK-010 |
| SC-LOG-02-01: Mensajes snake_case | TASK-011, TASK-012, TASK-013, TASK-014, TASK-015, TASK-016 |
| SC-LOG-02-02: Sin frases lenguaje natural | TASK-011, TASK-012, TASK-013, TASK-014, TASK-015, TASK-016 |
| SC-LOG-03-01: Component field presente | TASK-011, TASK-012, TASK-013, TASK-014, TASK-015, TASK-016 |
| SC-LOG-03-02: Contexto de sala incluye room_id | TASK-012, TASK-014, TASK-025, TASK-026 |
| SC-LOG-03-03: Operaciones medidas incluyen duration_ms | TASK-038, TASK-039 |
| SC-LAT-02-01: sync.Pool reutiliza instancias | TASK-019, TASK-020, TASK-021 |
| SC-LAT-02-02: Tracker captura Start/End | TASK-019, TASK-020 |
| SC-LAT-02-03: Emit retorna struct al pool | TASK-019, TASK-020 |
| SC-LAT-03-01: Pipeline completo emite chunk_latency | TASK-027, TASK-028, TASK-038, TASK-041 |
| SC-LAT-03-02: Etapa falla — status error | TASK-029, TASK-030 |
| SC-LAT-03-03: ChunkID incrementa secuencialmente | TASK-019, TASK-020 |
| SC-LAT-04-01: session_start emitido | TASK-025, TASK-026 |
| SC-LAT-04-02: session_end voluntario | TASK-031, TASK-032 |
| SC-LAT-04-03: session_end desconexión | TASK-033, TASK-034 |
| SC-LAT-04-04: session_error en fallo | TASK-029, TASK-030 |
| SC-LAT-04-05: pipeline_start emitido | TASK-025, TASK-026 |
| SC-LAT-04-06: pipeline_stop emitido | TASK-026, TASK-036 |
| SC-LAT-05-01: Contadores se incrementan | TASK-028, TASK-036 |
| SC-NET-01-01: Loadgen conecta y handshake | TASK-043, TASK-044 |
| SC-NET-01-02: Loadgen emite reporte | TASK-046, TASK-047 |
| SC-NET-01-03: Loadgen reporta error si server no disponible | TASK-044, TASK-056 |
| SC-NET-02-01: simulate-4g.ps1 aplica restricciones | TASK-054 |
| SC-NET-02-02: simulate-4g.sh aplica y remueve | TASK-055 |
| SC-NET-03-01: Todos los perfiles existen | TASK-050, TASK-051, TASK-052, TASK-053 |
| SC-NET-04-01: run-test-session produce reporte | TASK-056, TASK-057 |
| SC-NET-04-02: run-test-session maneja error de conexión | TASK-056, TASK-057 |
| SC-FIX-01-01: ListExpired firma actualizada | TASK-001, TASK-002 |
| SC-FIX-01-02: golangci-lint sin copylocks | TASK-001, TASK-002, TASK-042 |
| SC-FIX-02-01: gofmt produce zero cambios | TASK-003, TASK-004 |
| SC-FIX-03-01: exitAfterDefer eliminado | TASK-005, TASK-006 |
| SC-FIX-04-01: Test usa t.Errorf | TASK-007, TASK-008 |
