# Sprint 3 Task Breakdown: UX & Edge Cases

**Change**: sprint-3
**Status**: tasks
**Date**: 2026-06-11

---

## Legend

- TDD pairs: `TEST` task precedes its `IMPL` task — never implement before the test exists
- Atomic: each task is ~30–90 minutes of focused work
- `(requires TASK-XXX)` = hard dependency, do not start before the prerequisite is done

---

## Fase 1: Domain Go — sin dependencias externas

Archivos: `internal/domain/room/room.go`, `internal/domain/room/room_test.go`

- [ ] TASK-001: TEST - domain Room: `TestGenerateShortCode_Length`, `TestGenerateShortCode_Alphabet`, `TestGenerateShortCode_Uniqueness` (1000 calls, table-driven)
- [ ] TASK-002: IMPL - domain Room: añadir `ShortCode string`, `GenerateShortCode()` con crypto/rand y alfabeto `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (requiere TASK-001)
- [ ] TASK-003: TEST - domain Room: `TestJoin_UpdatesLastActivity`, `TestLeave_UpdatesLastActivity`, `TestTouchActivity` (table-driven)
- [ ] TASK-004: IMPL - domain Room: añadir `LastActivity time.Time`, `TouchActivity()`, invocarla al final del happy path de `Join()` y `Leave()` (requiere TASK-003)
- [ ] TASK-005: IMPL - domain Room: añadir sentinel `ErrShortCodeExhausted` en `internal/domain/room/errors.go` (requiere TASK-002)

---

## Fase 2: Ports Go — depende de Fase 1

Archivos: `internal/ports/driving/signaling.go`, `internal/ports/driving/room_manager.go`, `internal/ports/driven/room_repository.go`

- [ ] TASK-006: IMPL - driving port: añadir `OnDisconnect(ctx context.Context, sessionID string) error` a la interfaz `SignalingHandler` (requiere TASK-004)
- [ ] TASK-007: IMPL - driving port: añadir `FindByShortCode(ctx, code string) (*room.Room, error)` y `UpdateLastActivity(ctx, roomID string) error` a `RoomManager` (requiere TASK-002)
- [ ] TASK-008: IMPL - driving port: cambiar `CreateRoom` para retornar `CreateRoomResult{RoomID, ShortCode}` en lugar de `string` — definir el struct en `room_manager.go` (requiere TASK-007)
- [ ] TASK-009: IMPL - driven port: añadir `FindByShortCode`, `UpdateLastActivity`, `ListExpired(ctx, before time.Time)` a la interfaz `RoomRepository` (requiere TASK-004)

---

## Fase 3: Repository Go — depende de Fase 2

Archivos: `internal/app/roomsvc/repository.go`, `internal/app/roomsvc/repository_test.go`

- [ ] TASK-010: TEST - `InMemoryRoomRepository`: `TestFindByShortCode_Hit`, `TestFindByShortCode_Miss`, `TestFindByShortCode_CaseInsensitive` (table-driven) (requiere TASK-009)
- [ ] TASK-011: IMPL - `InMemoryRoomRepository.FindByShortCode`: O(n) scan, normaliza a uppercase con `strings.ToUpper` (requiere TASK-010)
- [ ] TASK-012: TEST - `InMemoryRoomRepository`: `TestUpdateLastActivity_Success`, `TestUpdateLastActivity_NotFound` (table-driven) (requiere TASK-009)
- [ ] TASK-013: IMPL - `InMemoryRoomRepository.UpdateLastActivity`: mutex Lock, actualiza `rm.LastActivity = time.Now()` (requiere TASK-012)
- [ ] TASK-014: TEST - `InMemoryRoomRepository`: `TestListExpired_MixedRooms`, `TestListExpired_NoneExpired` (table-driven) (requiere TASK-009)
- [ ] TASK-015: IMPL - `InMemoryRoomRepository.ListExpired`: RLock, filtra por `rm.Active && rm.LastActivity.Before(before)` (requiere TASK-014)

---

## Fase 4: Service Go — depende de Fases 2 + 3

Archivos: `internal/app/roomsvc/service.go`, `internal/app/roomsvc/service_test.go`, `internal/app/roomsvc/service_signaling_test.go`

- [ ] TASK-016: IMPL - `ServiceConfig` struct con `GracePeriod`, `RoomTTL`, `SweepInterval`, `MaxShortCodeRetries` y `DefaultServiceConfig()` (requiere TASK-006)
- [ ] TASK-017: IMPL - modificar `Service` struct: añadir `cfg ServiceConfig` y `graceTimers map[string]*time.Timer`; modificar `NewService` para aceptar `cfg` como primer parámetro (requiere TASK-016)
- [ ] TASK-018: IMPL - actualizar todos los callers existentes de `NewService` en tests (`service_signaling_test.go` y demás) para pasar `ServiceConfig` como primer arg — compilación debe pasar (requiere TASK-017)
- [ ] TASK-019: TEST - service: `TestOnDisconnect_StartsGracePeriod`, `TestOnDisconnect_BothDisconnect_NoGrace`, `TestOnDisconnect_SessionNotFound` (table-driven, GracePeriod=1ms) (requiere TASK-017)
- [ ] TASK-020: IMPL - `Service.OnDisconnect`: lookup de sesión, conteo de peers restantes, start grace timer con `time.AfterFunc`; NO eliminar la sesión del mapa (requiere TASK-019)
- [ ] TASK-021: TEST - service: `TestOnDisconnect_ReconnectCancelsGrace`, `TestJoinRoom_CancelsGraceTimer` (GracePeriod=50ms, rejoin dentro de 10ms) (requiere TASK-020)
- [ ] TASK-022: IMPL - `Service.JoinRoom`: añadir cancelación del grace timer después del `r.Join(userID)` exitoso (requiere TASK-021)
- [ ] TASK-023: TEST - service: `TestExpirationSweep_DeletesExpiredRooms`, `TestExpirationSweep_ActiveRoomNotExpired` (RoomTTL=1ms, SweepInterval=1ms) (requiere TASK-015, TASK-017)
- [ ] TASK-024: IMPL - `Service.startExpirationSweep` + `StartExpirationSweep` (exportado): goroutine con ticker, `ListExpired`, `notifyRoomPeers`, `DeleteRoom` (requiere TASK-023)
- [ ] TASK-025: IMPL - `Service.notifyRoomPeers` helper: RLock, itera sessions por roomID, llama `notifier.NotifySession` (requiere TASK-020)
- [ ] TASK-026: TEST - service: `TestCreateRoom_GeneratesShortCode`, `TestCreateRoom_ShortCodeCollision`, `TestCreateRoom_ShortCodeExhausted` (mock repo, table-driven) (requiere TASK-008, TASK-011)
- [ ] TASK-027: IMPL - `Service.CreateRoom`: loop de reintentos con `GenerateShortCode()` + `repo.FindByShortCode`, retorna `CreateRoomResult`; falla con `ErrShortCodeExhausted` tras 5 colisiones (requiere TASK-026)
- [ ] TASK-028: TEST - service: `TestFindByShortCode_Active`, `TestFindByShortCode_Expired` (requiere TASK-011)
- [ ] TASK-029: IMPL - `Service.FindByShortCode`: delega a `repo.FindByShortCode(ctx, strings.ToUpper(code))`, verifica `r.Active` y retorna `ErrRoomClosed` si inactiva (requiere TASK-028)
- [ ] TASK-030: IMPL - `Service.UpdateLastActivity`: delega a `repo.UpdateLastActivity` (requiere TASK-013)

---

## Fase 5: Hub Go — depende de Fase 4

Archivos: `internal/adapters/signaling/hub.go`, `internal/adapters/signaling/hub_test.go`

- [ ] TASK-031: TEST - Hub unregister: `TestUnregister_NotifiesPeerLeft` — cliente A se desconecta, cliente B (misma sala) recibe `peer-left` (requiere TASK-006)
- [ ] TASK-032: TEST - Hub unregister: `TestUnregister_CallsOnDisconnect` — cliente con sessionID → `handler.OnDisconnect` es llamado; `TestUnregister_NoSessionID_NoOnDisconnect` — sin sessionID → no se llama (requiere TASK-006)
- [ ] TASK-033: IMPL - Hub `unregister` case: notificación `peer-left` via `select/default` a otros clientes en la misma sala; call a `handler.OnDisconnect` FUERA del mutex (requiere TASK-031, TASK-032)
- [ ] TASK-034: IMPL - Hub: añadir `getHandler()` helper que adquiere `mu.RLock` para leer el handler de forma segura (requiere TASK-033)
- [ ] TASK-035: IMPL - Hub `Run`: cambiar firma a `Run(ctx context.Context)` con case `<-ctx.Done()` para graceful shutdown (requiere TASK-033)

---

## Fase 6: HTTP Go — depende de Fases 4 + 5

Archivos: `internal/adapters/http/server.go`, `internal/adapters/http/server_test.go`, `cmd/server/main.go`

- [ ] TASK-036: TEST - HTTP: `TestCreateRoom_IncludesShortCode` — POST /rooms → response JSON incluye campo `short_code` (requiere TASK-027)
- [ ] TASK-037: IMPL - HTTP `createRoomHandler`: actualizar para usar `CreateRoomResult`, incluir `short_code` en la respuesta 201 (requiere TASK-036)
- [ ] TASK-038: TEST - HTTP: `TestFindByShortCode_200`, `TestFindByShortCode_404`, `TestFindByShortCode_410` — GET /rooms/code/{code} (requiere TASK-029)
- [ ] TASK-039: IMPL - HTTP: añadir `findByShortCodeHandler` y registrar ruta `GET /rooms/code/{code}`; mapear `ErrRoomNotFound→404`, `ErrRoomClosed→410` (requiere TASK-038)
- [ ] TASK-040: TEST - HTTP: `TestJoinRoom_Full_409` — join cuando sala tiene 2 participantes → 409 "Esta sala ya tiene 2 participantes" (requiere TASK-017)
- [ ] TASK-041: IMPL - HTTP: mapear `room.ErrRoomFull→409` en `wsHandler` y en signaling error path (requiere TASK-040)
- [ ] TASK-042: IMPL - `cmd/server/main.go`: wiring de `ServiceConfig`, `ctx/cancel`, `go svc.StartExpirationSweep(ctx)`, `go hub.Run(ctx)` (requiere TASK-024, TASK-035)

---

## Fase 7: React Native — Setup del proyecto

Archivos: `mobile/` (nuevo directorio), `mobile/package.json`, `mobile/tsconfig.json`, `mobile/index.js`, `mobile/App.tsx`

- [ ] TASK-043: IMPL - inicializar proyecto React Native CLI bare workflow: `npx react-native@latest init TalkGoMobile --directory mobile --template react-native-template-typescript` (requiere nada — workstream independiente)
- [ ] TASK-044: IMPL - instalar dependencias con version pins: `react-native-webrtc@^118.0.7`, `zustand@^5.0.0`, `react-native-keep-awake@^4.0.0` (requiere TASK-043)
- [ ] TASK-045: IMPL - configurar `tsconfig.json` en modo strict (`"strict": true`); verificar `npx tsc --noEmit` pasa sin errores (requiere TASK-043)
- [ ] TASK-046: IMPL - crear estructura de directorios `mobile/src/`: `screens/`, `hooks/`, `components/`, `store/`, `services/`, `types/`, `native/android/`, `native/ios/` (requiere TASK-043)
- [ ] TASK-047: IMPL - iOS: link manual de `react-native-webrtc` — `cd mobile/ios && pod install`; verificar que el build en simulador compila sin errores (requiere TASK-044)
- [ ] TASK-048: IMPL - iOS: añadir permisos de micrófono en `Info.plist` (`NSMicrophoneUsageDescription`) (requiere TASK-047)
- [ ] TASK-049: IMPL - Android: verificar auto-linking de `react-native-webrtc`; añadir permiso `RECORD_AUDIO` en `AndroidManifest.xml`; build exitoso en emulador (requiere TASK-044)

---

## Fase 8: Zustand store + tipos TypeScript

Archivos: `mobile/src/store/sessionStore.ts`, `mobile/src/types/signaling.ts`, `mobile/src/types/session.ts`

- [ ] TASK-050: TEST - `sessionStore`: `connect` setea todos los campos y `connectionState='connected'`; `disconnect` resetea a idle; `tick` incrementa `elapsedSeconds`; `incrementErrors`/`resetErrors` (Jest, table-driven) (requiere TASK-045)
- [ ] TASK-051: IMPL - `sessionStore.ts`: Zustand store completo — `SessionState` + `SessionActions`; `localSpeaking`/`peerSpeaking` como `boolean`, no float (requiere TASK-050)
- [ ] TASK-052: IMPL - `types/signaling.ts`: `SignalingMessage` interface con todos los types del protocolo Go (`join`, `joined`, `offer`, `answer`, `ice-candidate`, `leave`, `peer-left`, `room-closed`, `error`) (requiere TASK-045)
- [ ] TASK-053: IMPL - `types/session.ts`: tipos `ConnectionState`, `ReconnectionState` y tipos auxiliares de sesión (requiere TASK-045)

---

## Fase 9: Hooks React Native — depende de Fase 8

Archivos: `mobile/src/hooks/`

- [ ] TASK-054: TEST - `useSignaling`: dispatcha callback correcto para cada tipo de mensaje; `isConnected=false` en onClose (Jest + `renderHook`) (requiere TASK-052)
- [ ] TASK-055: IMPL - `useSignaling`: WebSocket client — open WS a `${serverUrl}/ws/${roomId}`, parsea mensajes, expone `sendJoin`, `sendOffer`, `sendIceCandidate`, `sendLeave`, `reconnect`, `close` (requiere TASK-054)
- [ ] TASK-056: TEST - `useWebRTC`: `RTCPeerConnection` creado en mount; `close()` cierra PC y detiene tracks; `createOffer` retorna SDP; mock de `react-native-webrtc` (requiere TASK-051)
- [ ] TASK-057: IMPL - `useWebRTC`: lifecycle completo — `RTCPeerConnection`, `mediaDevices.getUserMedia({audio: true, video: false})`, `ontrack→remoteStream`, `oniceconnectionstatechange`, `createOffer({iceRestart})`, `setRemoteAnswer`, `addIceCandidate`, cleanup en unmount (requiere TASK-056)
- [ ] TASK-058: TEST - `useReconnection`: `trigger` → estado `reconnecting`, attempt=1; tras 3 fallos → `failed`; `cancel` previene reconexión; delays son 1000/2000/4000ms (requiere TASK-053)
- [ ] TASK-059: IMPL - `useReconnection`: state machine `connected→reconnecting→failed`, backoff `baseDelay * 2^(attempt-1)`, `cancel()` previene reconexión con `disconnectReason='user_initiated'` (requiere TASK-058)
- [ ] TASK-060: TEST - `useAudioLevel`: con PC null → ambos `false`; mocked `getStats` con `voiceActivityFlag=true` → `localSpeaking=true`; mocked silencio → `localSpeaking=false` (requiere TASK-057)
- [ ] TASK-061: IMPL - `useAudioLevel`: `setInterval` a 100ms, llama `pc.getStats()`, extrae `voiceActivityFlag` de `outbound-rtp`/`inbound-rtp`, fallback a `audioLevel > 0.01`, evita re-render si no hay cambio (requiere TASK-060)
- [ ] TASK-062: IMPL - `useKeepAwake`: wrapper fino sobre `react-native-keep-awake` — `activate()` en mount, `deactivate()` en unmount (requiere TASK-045)
- [ ] TASK-063: IMPL - `useSessionTimer`: incrementa `elapsedSeconds` via `sessionStore.tick()` cada 1000ms mientras `connectionState === 'connected'`; para en unmount (requiere TASK-051)

---

## Fase 10: Componentes UI React Native — depende de Fases 8 + 9

Archivos: `mobile/src/components/`, `mobile/src/screens/ConversationScreen.tsx`

- [ ] TASK-064: TEST - `VUMeter`: renderiza sin crash; con `speaking=true` muestra indicador activo; con `speaking=false` muestra indicador inactivo (RNTL) (requiere TASK-051)
- [ ] TASK-065: IMPL - `VUMeter`: componente con Animated API, suscripción a `useSessionStore(s => s.localSpeaking)` o prop directa; re-render aislado via selector (requiere TASK-064)
- [ ] TASK-066: TEST - `ConnectionStatus`: muestra "Conectando..." en `connecting`; "En línea" en `connected`; "Reconectando..." en `reconnecting`; "Conexión perdida" en `failed` (RNTL, table-driven) (requiere TASK-051)
- [ ] TASK-067: IMPL - `ConnectionStatus`: componente presentacional, recibe `connectionState: ConnectionState` como prop (requiere TASK-066)
- [ ] TASK-068: TEST - `MuteButton`: renderiza ícono normal cuando `isMuted=false`; renderiza ícono tachado cuando `isMuted=true`; callback `onPress` es invocado (RNTL) (requiere TASK-051)
- [ ] TASK-069: IMPL - `MuteButton`: toggle visual con `isMuted` prop, ícono accesible (requiere TASK-068)
- [ ] TASK-070: TEST - `SessionTimer`: 0s → "00:00"; 65s → "01:05"; 3661s → "01:01:01" si se decide soportar horas, o 3661s → "61:01" si no (RNTL, table-driven) (requiere TASK-051)
- [ ] TASK-071: IMPL - `SessionTimer`: formatea `elapsedSeconds` a `MM:SS` via `useSessionStore(s => s.elapsedSeconds)` (requiere TASK-070)
- [ ] TASK-072: TEST - `EndCallButton` / dialog confirmación: press "Finalizar" → dialog aparece; press "Cancelar" → dialog cierra, conversación continúa; press "Confirmar" → `onConfirm` callback invocado (RNTL) (requiere TASK-051)
- [ ] TASK-073: IMPL - `EndCallButton`: botón + Alert/Modal con "¿Terminar conversación?" + botones "Cancelar" y "Confirmar" (requiere TASK-072)
- [ ] TASK-074: TEST - `PipelineErrorBanner`: con `pipelineError=null` → no visible; con error → banner visible con texto correcto; con `consecutiveErrors >= 3` → fallback texto visible (RNTL) (requiere TASK-051)
- [ ] TASK-075: IMPL - `PipelineErrorBanner`: banner condicional, text fallback en 3+ errores consecutivos, desaparece cuando `resetErrors` limpia el estado (requiere TASK-074)
- [ ] TASK-076: TEST - `ConversationScreen`: renderiza en estado `connected` — VU meters, timer, mute, botón Finalizar presentes; en estado `reconnecting` — `ConnectionStatus` muestra "Reconectando..."; en estado `failed` — muestra botón "Reintentar" (RNTL) (requiere TASK-065, TASK-067, TASK-069, TASK-071, TASK-073)
- [ ] TASK-077: IMPL - `ConversationScreen`: composición de todos los componentes + hooks (`useWebRTC`, `useSignaling`, `useReconnection`, `useAudioLevel`, `useKeepAwake`, `useSessionTimer`); maneja `peer-left` → cierra sesión; `room-closed` → cierra sesión (requiere TASK-076)

---

## Fase 11: Background mode — depende de Fase 7

Archivos: `mobile/ios/TalkGo/Info.plist`, `mobile/ios/TalkGo/AudioSessionManager.swift`, `mobile/android/app/src/main/AndroidManifest.xml`, `mobile/src/native/android/CallForegroundService.kt`, `mobile/src/native/android/CallServiceModule.kt`

- [ ] TASK-078: IMPL - iOS `Info.plist`: añadir `UIBackgroundModes: ["audio"]` (requiere TASK-047)
- [ ] TASK-079: IMPL - iOS `AudioSessionManager.swift`: native module Obj-C bridged — `activate()` configura `AVAudioSession` con `.playAndRecord`, `.voiceChat`, `.allowBluetooth`, `.defaultToSpeaker`; `deactivate()` llama `setActive(false, .notifyOthersOnDeactivation)` (requiere TASK-078)
- [ ] TASK-080: IMPL - Android `AndroidManifest.xml`: permisos `FOREGROUND_SERVICE`, `FOREGROUND_SERVICE_MICROPHONE`, `RECORD_AUDIO`, `WAKE_LOCK`; declarar `<service android:name=".CallForegroundService" android:foregroundServiceType="microphone" />` (requiere TASK-049)
- [ ] TASK-081: IMPL - Android `CallForegroundService.kt`: `Service` con `startForeground` tipo microphone, notificación persistente "TalkGo / Conversación activa", partial wake lock con 1h máximo, `START_STICKY` (requiere TASK-080)
- [ ] TASK-082: IMPL - Android `CallServiceModule.kt`: `ReactContextBaseJavaModule` con `@ReactMethod start()` y `stop()`, registrar en `MainApplication` (requiere TASK-081)
- [ ] TASK-083: IMPL - `services/signalingService.ts`: wrapper del `AudioService` que llama `AudioSessionManager.activate()` en iOS y `NativeModules.CallService.start()` en Android al iniciar sesión; inverso al terminar (requiere TASK-079, TASK-082)

---

## Fase 12: Integración + Wiring final — depende de Fases 9 + 10 + 11

Archivos: `mobile/src/services/api.ts`, wiring final en `ConversationScreen.tsx`, tests de integración

- [ ] TASK-084: TEST - `api.ts`: `createRoom` POST → retorna `{room_id, short_code}`; `findByShortCode` GET → 200 retorna room, 404 lanza error (Jest + fetch mock) (requiere TASK-052)
- [ ] TASK-085: IMPL - `api.ts`: HTTP client — `createRoom(sourceLang, targetLang)`, `findByShortCode(code)` con manejo de 404/410 explícito (requiere TASK-084)
- [ ] TASK-086: TEST - integración `ConversationScreen`: WS mockeado simula flujo completo join→joined→offer→answer→ice; verifica que `connectionState` llega a `connected`; verifica que `peer-left` dispara cierre de sesión (RNTL + WS mock) (requiere TASK-077, TASK-085)
- [ ] TASK-087: IMPL - wiring final en `ConversationScreen`: integrar `api.ts` para lookup de sala al montar; pasar `roomId`/`shortCode` al store; conectar `useSignaling` con `useWebRTC` en el flujo offer/answer/ice completo (requiere TASK-086)
- [ ] TASK-088: IMPL - `App.tsx`: punto de entrada limpio que monta `ConversationScreen` con props de prueba; verifica `npx tsc --noEmit` y `npx react-native run-ios` sin errores (requiere TASK-087)

---

## Resumen de tasks por fase

| Fase | Tasks | Descripción |
|------|-------|-------------|
| 1 | TASK-001..005 | Domain Go (ShortCode, LastActivity) |
| 2 | TASK-006..009 | Ports Go (driving + driven) |
| 3 | TASK-010..015 | Repository InMemory (3 métodos nuevos) |
| 4 | TASK-016..030 | Service (ServiceConfig, OnDisconnect, sweep, CreateRoom, FindByShortCode) |
| 5 | TASK-031..035 | Hub (peer-left, OnDisconnect, Run con ctx) |
| 6 | TASK-036..042 | HTTP + main.go wiring |
| 7 | TASK-043..049 | React Native setup (bare workflow, deps, permisos) |
| 8 | TASK-050..053 | Zustand store + tipos TypeScript |
| 9 | TASK-054..063 | Hooks (useSignaling, useWebRTC, useReconnection, useAudioLevel, useKeepAwake, useSessionTimer) |
| 10 | TASK-064..077 | Componentes UI + ConversationScreen |
| 11 | TASK-078..083 | Background mode iOS + Android |
| 12 | TASK-084..088 | Integración + wiring final |

**Total: 88 tasks** (44 pares TDD + 12 standalone impl + 32 impl sin par de test directo)

---

## Acceptance Criteria coverage

| CA | Tasks que lo cubren |
|----|---------------------|
| CA-01 (keep-awake) | TASK-062, TASK-077 |
| CA-02 (VU meters) | TASK-060, TASK-061, TASK-064, TASK-065 |
| CA-03 (confirmar Finalizar) | TASK-072, TASK-073 |
| CA-04 (notificación peer-left ambos lados) | TASK-031, TASK-033, TASK-076, TASK-077 |
| CA-05 (reconexión 3 intentos backoff) | TASK-058, TASK-059 |
| CA-06 (período gracia 30s) | TASK-019, TASK-020, TASK-021, TASK-022 |
| CA-07 (pipeline fallback visual) | TASK-074, TASK-075 |
| CA-08 (Bluetooth fallback) | TASK-079, TASK-083 |
| CA-09 (iOS background) | TASK-078, TASK-079, TASK-083 |
| CA-10 (Android Foreground Service) | TASK-080, TASK-081, TASK-082, TASK-083 |
| CA-11 (sala llena → 409) | TASK-040, TASK-041 |
| CA-12 (sala expirada → 410) | TASK-023, TASK-024, TASK-038, TASK-039 |
