# Sprint 3 Spec: UX y Edge Cases (Archival Summary)

**Change**: sprint-3
**Status**: archived
**Date**: 2026-06-11

---

## Requirements Summary

### Workstream A: Backend Go — Fixes Críticos

#### REQ-A01: Notificación de desconexión de peer
- Hub notifica al peer restante cuando un cliente se desconecta
- Interfaz `driving.SignalingHandler` expone `OnDisconnect(ctx, sessionID)`
- Mensaje formato: `{"type": "peer-left", "session_id": "..."}`

#### REQ-A02: Período de gracia de 30 segundos
- Disconnect inicia timer de 30s si quedan participantes
- Reconexión cancela el timer y mantiene sala activa
- `ServiceConfig.GracePeriod` configurable para tests (default 30s)
- Al vencer: notifica peer restante con `room-closed` y elimina sala

#### REQ-A03: Expiración de sala por inactividad
- Room struct incluye `LastActivity time.Time`
- Service ejecuta sweep cada 60s, cierra salas inactivas >10 minutos
- Cliente intentando unirse a sala expirada recibe HTTP 410 Gone
- `ServiceConfig.RoomTTL` y `SweepInterval` configurables

#### REQ-A04: Short codes alfanuméricos de 6 caracteres
- Código genera mediante `GenerateShortCode()` con alfabeto `ABCDEFGHJKLMNPQRSTUVWXYZ23456789`
- Stored en Room struct como `ShortCode string`
- Detección de colisiones con max 5 reintentos
- Endpoint `GET /rooms/code/{code}` retorna room_id + short_code o 404
- `POST /rooms` retorna tanto room_id como short_code

#### REQ-A05: Error de sala llena (HTTP 409)
- Tercer usuario intentando unirse → `ErrRoomFull`
- HTTP mapeo a 409 Conflict con mensaje "Esta sala ya tiene 2 participantes"
- WS error path también retorna mismo mensaje

### Workstream B: React Native — Cliente Móvil

#### REQ-B01: Setup React Native
- Proyecto bare workflow en `mobile/`
- TypeScript strict mode, zero tsc errors
- Estructura: `src/screens/`, `src/hooks/`, `src/components/`, `src/store/`, `src/services/`, `src/types/`

#### REQ-B02: Conexión WebSocket y handshake WebRTC
- SignalingService conecta a `ws://<host>/ws/{roomID}`
- Protocolo: join → joined → offer → answer → ice-candidates
- Servidor actúa como SFU (no peer-to-peer)

#### REQ-B03: Pantalla de conversación activa
- VU meters (local + remoto)
- Indicador estado conexión
- Toggle mute
- Timer MM:SS
- Botón "Finalizar" con dialog confirmación
- Keep-awake activo durante sesión

#### REQ-B04: VU meters en tiempo real
- useAudioLevel hook usa getStats() + VAD
- Speaking/not-speaking boolean (no float) para evitar re-renders
- Zustand selectors para aislamiento de componentes

#### REQ-B05: Reconexión automática con backoff
- Estado: CONNECTED → RECONNECTING → CONNECTED | FAILED
- Delays: 1s, 2s, 4s (max 3 intentos)
- ICE restart en cada reconexión
- No reconectar si desconexión fue intencional

#### REQ-B06: Background mode iOS
- Info.plist: UIBackgroundModes: ["audio"]
- AVAudioSession: .playAndRecord, .voiceChat, .allowBluetooth, .defaultToSpeaker

#### REQ-B07: Background mode Android
- Foreground Service con notificación persistente
- Permisos: FOREGROUND_SERVICE, FOREGROUND_SERVICE_MICROPHONE, RECORD_AUDIO
- NativeModules bridge para start/stop servicio

#### REQ-B08: Manejo de errores del pipeline
- Banner de error visible en fallos de traducción
- Fallback visual si 3+ errores consecutivos
- Auto-ocultarse cuando pipeline se recupera

#### REQ-B09: Bluetooth fallback
- Detect desconexión Bluetooth mid-sesión
- Fallback automático a micrófono integrado
- Toast notificación al usuario

---

## Acceptance Criteria (12 total)

| CA  | Descripción |
|-----|-------------|
| CA-01 | Pantalla activa (no se apaga) durante sesión |
| CA-02 | VU meters responden en tiempo real |
| CA-03 | "Finalizar" requiere confirmación |
| CA-04 | Ambos dispositivos reciben notificación de fin |
| CA-05 | Reconexión automática 3 intentos (1s, 2s, 4s) |
| CA-06 | Período de gracia 30s antes de cerrar sala |
| CA-07 | API falla persistentemente → fallback visual |
| CA-08 | Bluetooth disconnect → fallback micrófono integrado |
| CA-09 | iOS: sesión continúa en background |
| CA-10 | Android: Foreground Service mantiene sesión activa |
| CA-11 | Tercer usuario → error "Esta sala ya tiene 2 participantes" |
| CA-12 | Sala expirada → error "Esta sala expiró" |

---

## Non-Functional Requirements

| NFR | Requirement |
|-----|-------------|
| NFR-01 | Backend: `go vet ./...` y `golangci-lint run` MUST pasar con zero issues |
| NFR-02 | Backend: coverage ≥ 80% en archivos modificados |
| NFR-03 | Backend: ningún nuevo módulo externo — solo stdlib |
| NFR-04 | Mobile: TypeScript strict mode, zero errores en `tsc --noEmit` |
| NFR-05 | Mobile: MUST buildear sin errores en iOS simulator y Android emulator |
| NFR-06 | Mobile: background mode y Bluetooth MUST ser verificados en dispositivos físicos |
| NFR-07 | Arquitectura Hexagonal: `internal/domain/` MUST NOT importar `internal/adapters/` |
| NFR-08 | Strict TDD Mode: escenarios de backend con tests ANTES de implementación |

---

## Testing Strategy Summary

### Go: Strict TDD — tests first
- Domain tests: GenerateShortCode, LastActivity, TouchActivity
- Repository tests: FindByShortCode, UpdateLastActivity, ListExpired
- Service tests: OnDisconnect grace period, expiration sweep, short code collision retry
- Hub tests: peer-left notification, OnDisconnect lifecycle
- HTTP tests: createRoom with short_code, findByShortCode, room full 409

### React Native: Jest + RNTL
- Store tests: connect, disconnect, tick, errors
- Hook tests: useReconnection backoff, useSignaling message dispatch, useWebRTC lifecycle
- Component tests: VUMeter, ConnectionStatus, EndCallButton confirmation, PipelineErrorBanner
- Integration tests: full flow with WS mock

---

## Key Decisions

- **OnDisconnect**: Método explícito en `SignalingHandler`, no mensajes sintéticos
- **Grace timer**: Service layer, not Hub (separation of concerns)
- **Sweep expiry**: Single goroutine, no per-room timers
- **VU meters**: Boolean state (not float) para evitar re-renders frecuentes
- **Reconnection**: Same JoinRoom path con ICE restart, no nuevo message type
- **Short codes**: Normalized uppercase lookup, case-insensitive para UX verbal

---

## Open Questions (Resolved in Design)

- **OQ-01**: react-native-webrtc RN 0.76 ✓ Compatible, pin ^118.0.7
- **OQ-02**: OnDisconnect method ✓ Sí, método explícito
- **OQ-03**: Short codes case-sensitive ✓ No, normalized uppercase
