# Sprint 3 Spec: UX y Edge Cases

**Change**: sprint-3
**Status**: spec
**Date**: 2026-06-11

---

## Overview

Sprint 3 entrega la primera experiencia end-to-end usable de TalkGo. El backend recibe cuatro fixes críticos de robustez (notificación de desconexión, período de gracia, expiración por inactividad, y short codes). El cliente móvil React Native implementa la pantalla de conversación activa con VU meters, reconexión automática, background mode en iOS/Android, y fallbacks de error.

Los escenarios de este spec son directamente traducibles a funciones de test: los de backend a table-driven tests en Go, los de mobile a Jest + React Native Testing Library.

---

## Workstream A: Backend Go — Fixes Críticos

### REQ-A01: Notificación de desconexión de peer

El Hub MUST notificar al peer restante en la misma sala cuando un cliente se desconecta, tanto de forma voluntaria (mensaje `leave`) como abrupta (WebSocket caído sin `leave`).

La interfaz `driving.SignalingHandler` MUST exponer un método `OnDisconnect(ctx context.Context, sessionID string) error` para que el Hub lo invoque en ambos casos. No se MUST usar mensajes `leave` sintéticos — un método de ciclo de vida explícito es la separación hexagonal correcta.

El mensaje enviado al peer restante MUST tener la forma:
```json
{"type": "peer-left", "session_id": "<departed_session_id>"}
```

El Hub MUST enviar esta notificación vía el canal `send` del cliente destino usando `select` con `default` para evitar blocking si el buffer está lleno.

**Escenarios**:

**SC-A01-01: Peer se desconecta voluntariamente (send `leave`)**
```
Given: dos clientes A y B conectados a la misma sala
When:  el cliente A envía {"type":"leave","session_id":"<A>"}
Then:  el cliente B recibe {"type":"peer-left","session_id":"<A>"}
And:   la sesión de A es limpiada en el Service
And:   el canal send de A es cerrado por el Hub
```

**SC-A01-02: Peer se desconecta abruptamente (WS cae sin `leave`)**
```
Given: dos clientes A y B conectados a la misma sala
When:  la conexión WebSocket de A cae (readPump retorna sin mensaje leave)
Then:  el Hub envía al channel unregister con el client A
And:   el Hub llama OnDisconnect(ctx, A.sessionID) en el Service
And:   el cliente B recibe {"type":"peer-left","session_id":"<A>"} en ≤100ms
```

**SC-A01-03: Solo un participante en la sala**
```
Given: solo el cliente A está conectado a una sala
When:  A se desconecta (voluntaria o abruptamente)
Then:  no se envía ningún mensaje peer-left (no hay destinatario)
And:   la sesión de A es limpiada correctamente
```

**SC-A01-04: Session ID vacío en unregister**
```
Given: un cliente sin sessionID (nunca completó el join) se desconecta
When:  el Hub lo procesa en el case unregister
Then:  OnDisconnect NO es llamado (sessionID == "")
And:   el client es eliminado de h.clients sin errores
```

---

### REQ-A02: Período de gracia de 30 segundos

Cuando un peer abandona una sala que aún tiene un participante restante, el Service MUST iniciar un timer de 30 segundos antes de cerrar la sala. Si el peer que se fue reconecta (hace `join` nuevamente) dentro de ese período, el timer MUST ser cancelado y la sala continúa activa.

El `Service` MUST aceptar un `ServiceConfig` con un campo `GracePeriod time.Duration` (default `30 * time.Second`) para que los tests puedan configurar tiempos cortos (1ms).

Los timers activos MUST ser almacenados en un `map[string]*time.Timer` indexado por `roomID` en el Service, protegido por el mutex existente.

Cuando el timer dispara MUST:
1. Llamar `DeleteRoom` para cerrar la sala
2. Notificar al participante restante con `{"type":"room-closed","reason":"peer-timeout"}`

**Escenarios**:

**SC-A02-01: Peer reconecta antes del vencimiento del timer**
```
Given: sala con clientes A y B
And:   A se desconecta → timer de gracia inicia
When:  A se reconecta (join) antes de los 30s
Then:  el timer es cancelado (timer.Stop() retorna true)
And:   la sala sigue activa (room.Active == true)
And:   B NO recibe ningún mensaje room-closed
```

**SC-A02-02: Timer vence sin reconexión**
```
Given: sala con clientes A y B
And:   A se desconecta → timer de gracia inicia (config: 1ms en tests)
When:  el timer dispara sin que A reconecte
Then:  DeleteRoom es llamado para la sala
And:   B recibe {"type":"room-closed","reason":"peer-timeout"}
And:   la entrada de graceTimers[roomID] es eliminada
```

**SC-A02-03: Ambos peers se desconectan simultáneamente**
```
Given: sala con A y B
When:  A y B se desconectan en el mismo ciclo del Hub
Then:  el período de gracia NO inicia (sala ya sin participantes)
And:   la sala es eliminada directamente
```

**SC-A02-04: Timer de gracia no inicia cuando la sala queda vacía**
```
Given: sala con solo el cliente A (B nunca llegó a unirse)
When:  A se desconecta
Then:  graceTimers[roomID] NO tiene entrada (no hay peer que esperar)
And:   LeaveRoom limpia la sesión de A normalmente
```

---

### REQ-A03: Expiración de sala por inactividad

El dominio `room.Room` MUST incluir un campo `LastActivity time.Time` que se actualiza en cada `Join` y `Leave` exitosos.

El `Service` MUST ejecutar un goroutine de sweep que, cada 60 segundos (configurable vía `ServiceConfig.SweepInterval`), cierra las salas donde `time.Since(r.LastActivity) > expiryDuration` (default `10 * time.Minute`, configurable vía `ServiceConfig.RoomExpiry`).

Cuando un cliente intenta unirse a una sala expirada (cerrada), el Service MUST retornar `room.ErrRoomClosed`. La capa HTTP MUST mapear este error a `HTTP 410 Gone` con body `{"error":"Esta sala expiró. Creá una nueva."}`.

**Escenarios**:

**SC-A03-01: Sala expira por inactividad**
```
Given: sala creada hace más de RoomExpiry sin actividad
When:  el goroutine de sweep ejecuta
Then:  room.Close() es llamado en la sala
And:   la sala es eliminada del repositorio
And:   los participantes activos (si los hay) reciben room-closed
```

**SC-A03-02: Sala activa no es expirada**
```
Given: sala con LastActivity hace menos de RoomExpiry
When:  el goroutine de sweep ejecuta
Then:  la sala NO es cerrada ni eliminada
```

**SC-A03-03: Cliente intenta unirse a sala expirada**
```
Given: sala cuyo room.Active == false
When:  un cliente envía {"type":"join","room_id":"<expired>"}
Then:  JoinRoom retorna room.ErrRoomClosed
And:   HandleSignaling retorna {"type":"error","message":"room is closed"}
And:   la capa HTTP responde HTTP 410 con {"error":"Esta sala expiró. Creá una nueva."}
```

**SC-A03-04: LastActivity se actualiza al hacer Join**
```
Given: sala recién creada con LastActivity = T0
When:  un usuario hace Join
Then:  room.LastActivity > T0
```

**SC-A03-05: Sweep con múltiples salas mixtas**
```
Given: sala A expirada + sala B activa
When:  el sweep ejecuta
Then:  sala A es cerrada y eliminada
And:   sala B permanece intacta
```

---

### REQ-A04: Short codes alfanuméricos de 6 caracteres

`CreateRoom` MUST generar y almacenar un short code de 6 caracteres como campo `ShortCode string` en el dominio `room.Room`. El código MUST usar el alfabeto `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (excluye 0, O, 1, I para evitar ambigüedad verbal). La generación MUST usar `crypto/rand`.

El `driven.RoomRepository` MUST incluir `FindByShortCode(ctx context.Context, code string) (*Room, error)`.

La generación MUST incluir detección de colisiones con un máximo de 5 reintentos. Si tras 5 intentos el código colisiona, `CreateRoom` MUST retornar `ErrShortCodeExhausted`.

La respuesta de `POST /rooms` MUST incluir el short code:
```json
{"room_id": "<uuid>", "short_code": "<6-char>"}
```

El endpoint `GET /rooms/code/{code}` MUST:
- Retornar `{"room_id":"<uuid>","short_code":"<code>"}` con HTTP 200 si existe
- Retornar HTTP 404 con `{"error":"room not found"}` si no existe

**Escenarios**:

**SC-A04-01: Generación de short code en CreateRoom**
```
Given: repositorio vacío
When:  CreateRoom("es","en") es llamado
Then:  el room creado tiene un ShortCode de exactamente 6 caracteres
And:   todos los caracteres pertenecen al alfabeto ABCDEFGHJKLMNPQRSTUVWXYZ23456789
And:   la respuesta HTTP incluye {"room_id":"...","short_code":"..."}
```

**SC-A04-02: Lookup por short code (hit)**
```
Given: sala existente con short_code "A3X7K9"
When:  GET /rooms/code/A3X7K9
Then:  HTTP 200 con {"room_id":"<uuid>","short_code":"A3X7K9"}
```

**SC-A04-03: Lookup por short code (miss)**
```
Given: no existe sala con short_code "ZZZZZZ"
When:  GET /rooms/code/ZZZZZZ
Then:  HTTP 404 con {"error":"room not found"}
```

**SC-A04-04: Código case-insensitive en lookup**
```
Given: sala con short_code "A3X7K9"
When:  GET /rooms/code/a3x7k9
Then:  HTTP 200 (el handler normaliza a uppercase antes de buscar)
```

**SC-A04-05: Colisión en generación — reintento exitoso**
```
Given: mock crypto/rand que genera "AAAAAA" las primeras 2 veces
And:   ya existe una sala con short_code "AAAAAA"
When:  CreateRoom es llamado
Then:  el sistema reintenta y eventualmente genera un código único
And:   la sala es creada exitosamente
```

**SC-A04-06: Colisión agotada (5 intentos)**
```
Given: mock que siempre genera el mismo código colisionante
When:  CreateRoom es llamado
Then:  retorna ErrShortCodeExhausted después de 5 intentos
And:   ninguna sala es persistida
```

---

### REQ-A05: Error de sala llena (CA-11)

Cuando un tercer usuario intenta unirse a una sala que ya tiene 2 participantes, el Service MUST retornar `room.ErrRoomFull`. La capa HTTP MUST mapear este error a `HTTP 409 Conflict` con body `{"error":"Esta sala ya tiene 2 participantes"}`.

Via WebSocket, `HandleSignaling` MUST retornar `{"type":"error","message":"Esta sala ya tiene 2 participantes"}` cuando `JoinRoom` falla con `ErrRoomFull`.

**Escenarios**:

**SC-A05-01: Tercer usuario vía HTTP**
```
Given: sala con 2 participantes activos
When:  POST /rooms/{id}/join con un tercer userID
Then:  HTTP 409 con {"error":"Esta sala ya tiene 2 participantes"}
```

**SC-A05-02: Tercer usuario vía WebSocket**
```
Given: sala con 2 participantes activos y sesiones WebSocket abiertas
When:  un tercer cliente envía {"type":"join","room_id":"<id>"}
Then:  recibe {"type":"error","message":"Esta sala ya tiene 2 participantes"}
And:   la conexión WebSocket permanece abierta (el error no cierra el WS)
```

---

## Workstream B: React Native — Cliente Móvil

### REQ-B01: Setup del proyecto React Native

El proyecto MUST ser inicializado como React Native CLI bare workflow en `mobile/` dentro del repositorio `TalkGo`. NOT Expo managed (incompatible con `react-native-webrtc`).

La estructura de `mobile/src/` MUST seguir:
```
src/
├── screens/ConversationScreen.tsx
├── hooks/
│   ├── useWebRTC.ts
│   ├── useSignaling.ts
│   ├── useAudioLevel.ts
│   └── useReconnection.ts
├── services/
│   ├── SignalingService.ts
│   └── AudioService.ts
├── state/sessionStore.ts
└── types/signaling.ts
```

TypeScript MUST estar configurado en modo strict (`"strict": true`). El proyecto MUST compilar con zero errores de TypeScript.

**Escenarios**:

**SC-B01-01: Build exitoso en iOS simulator**
```
Given: proyecto inicializado con npx react-native init
And:   dependencias instaladas (react-native-webrtc, zustand, react-native-keep-awake)
When:  npx react-native run-ios
Then:  el simulador muestra la app sin errores de build
```

**SC-B01-02: Build exitoso en Android emulator**
```
Given: proyecto inicializado y dependencias instaladas
When:  npx react-native run-android
Then:  el emulador muestra la app sin errores de build
And:   react-native-webrtc está correctamente linkedeado (no "NativeModule not found")
```

**SC-B01-03: TypeScript strict mode**
```
Given: código fuente en mobile/src/
When:  npx tsc --noEmit
Then:  zero errores de tipo
```

---

### REQ-B02: Conexión WebSocket y handshake WebRTC

El cliente MUST conectar al backend Go siguiendo el protocolo existente. El `SignalingService` MUST implementar la máquina de estados:

```
DISCONNECTED → CONNECTING → CONNECTED → (en sala) → JOINED → LIVE
```

**Protocolo a consumir** (servidor Go existente):
- `POST /rooms` → `{"room_id":"<uuid>","short_code":"<6-char>"}`
- `GET /ws/{roomID}` → WebSocket upgrade
- Flujo de mensajes:
  1. Cliente → `{"type":"join","room_id":"...","user_id":"...","lang":"es"}`
  2. Server → `{"type":"joined","session_id":"..."}`
  3. Cliente crea `RTCPeerConnection`, añade track de audio local
  4. Cliente → `{"type":"offer","session_id":"...","sdp":"..."}`
  5. Server → `{"type":"answer","session_id":"...","sdp":"..."}`
  6. ICE candidates bidireccionales → `{"type":"ice-candidate","session_id":"...","candidate":"..."}`

**El servidor actúa como SFU** — los clientes NO se conectan entre sí peer-to-peer.

**Escenarios**:

**SC-B02-01: Flujo join exitoso**
```
Given: servidor Go corriendo en localhost:8080
And:   sala creada con roomID conocido
When:  SignalingService.connect(roomID, userID, "es") es llamado
Then:  WebSocket se conecta a ws://localhost:8080/ws/{roomID}
And:   mensaje join es enviado
And:   se recibe joined con sessionID no vacío
And:   el estado cambia a JOINED
```

**SC-B02-02: Flujo offer/answer completo**
```
Given: estado JOINED con sessionID
When:  useWebRTC.startCall() es llamado
Then:  RTCPeerConnection es creado con track de audio local
And:   offer SDP es generado y enviado al servidor
And:   se recibe answer SDP del servidor
And:   RTCPeerConnection.setRemoteDescription es llamado con el answer
And:   el estado cambia a LIVE
```

**SC-B02-03: WebSocket no puede conectar**
```
Given: servidor no disponible en la URL configurada
When:  SignalingService.connect() es llamado
Then:  el error es capturado y el estado va a FAILED
And:   el hook useReconnection inicia el backoff (ver REQ-B05)
```

**SC-B02-04: Mensaje desconocido del servidor**
```
Given: conexión WebSocket activa
When:  el servidor envía {"type":"unknown-future-type"}
Then:  el cliente lo ignora sin crashear
And:   el estado de conexión no cambia
```

---

### REQ-B03: Pantalla de conversación activa (CA-01, CA-02, CA-03, CA-04)

La `ConversationScreen` MUST mostrar los siguientes elementos:
- VU meters (uno local, uno remoto) que reflejen actividad de audio
- Indicador de estado de conexión (`Conectando...` / `En línea` / `Reconectando...` / `Conexión perdida`)
- Toggle de mute con estado visual claro (ícono de micrófono tachado cuando muteado)
- Timer de duración de la sesión en formato `MM:SS` actualizándose cada segundo
- Botón "Finalizar" que al presionarse MUST mostrar un dialog de confirmación

La pantalla MUST mantener la pantalla activa usando `react-native-keep-awake` mientras la sesión esté activa (CA-01).

**Escenarios**:

**SC-B03-01: Dialog de confirmación al finalizar (CA-03)**
```
Given: ConversationScreen renderizada con estado LIVE
When:  el usuario presiona el botón "Finalizar"
Then:  aparece un Alert/Modal con título "¿Terminar conversación?"
And:   tiene botones "Cancelar" y "Confirmar"
When:  el usuario presiona "Cancelar"
Then:  el dialog se cierra y la conversación continúa
```

**SC-B03-02: Finalizar confirma y desconecta**
```
Given: dialog de confirmación visible
When:  el usuario presiona "Confirmar"
Then:  SignalingService.disconnect() es llamado
And:   mensaje leave es enviado al servidor
And:   RTCPeerConnection es cerrado
And:   la pantalla navega fuera de ConversationScreen
```

**SC-B03-03: Peer remoto finaliza la llamada (CA-04)**
```
Given: sesión activa en estado LIVE
When:  el servidor envía {"type":"peer-left","session_id":"<remote>"}
Then:  el indicador de estado muestra "El otro participante se desconectó"
And:   la sesión local es finalizada limpiamente
And:   ambas desconexiones (voluntaria e involuntaria) activan este flujo
```

**SC-B03-04: Sala cerrada por el servidor (room-closed)**
```
Given: sesión activa
When:  el servidor envía {"type":"room-closed","reason":"peer-timeout"}
Then:  se muestra mensaje "La sala fue cerrada"
And:   la conexión es terminada limpiamente
```

**SC-B03-05: Timer de sesión incrementa cada segundo**
```
Given: ConversationScreen con estado LIVE desde T=0
When:  pasan 65 segundos
Then:  el timer muestra "01:05"
```

**SC-B03-06: Keep-awake activo durante sesión**
```
Given: ConversationScreen montada con estado LIVE
Then:  activateKeepAwake() fue llamado al montar
When:  el componente se desmonta (sesión termina)
Then:  deactivateKeepAwake() es llamado
```

---

### REQ-B04: VU meters en tiempo real (CA-02)

Los VU meters MUST responder visualmente a la actividad de audio del stream local y el stream remoto. La implementación inicial MUST usar el approach `getStats()` + VAD (Voice Activity Detection) — indicador binario speaking/not-speaking.

Si `getStats()` no provee suficiente fidelidad visual, el fallback escalable es un módulo nativo con RMS; pero esto está fuera del scope de Sprint 3.

El hook `useAudioLevel` MUST exponer `{ localLevel: number, remoteLevel: number }` donde el valor es `0.0` (silencio) a `1.0` (máximo), actualizándose a una tasa de ≥10Hz.

El componente VU meter MUST renderizar de forma eficiente — usando `Zustand` con selectores para evitar re-renders del árbol completo en cada tick de audio.

**Escenarios**:

**SC-B04-01: VU meter local responde a voz**
```
Given: RTCPeerConnection con track de audio local activo
When:  el usuario habla (audio detectado en el stream local)
Then:  localLevel > 0 dentro de los próximos 100ms
```

**SC-B04-02: VU meter en silencio**
```
Given: RTCPeerConnection activo sin audio
When:  useAudioLevel se ejecuta con getStats()
Then:  localLevel == 0 (o cercano a 0, threshold < 0.05)
```

**SC-B04-03: Mute desactiva VU meter local**
```
Given: usuario muteado (track.enabled = false)
When:  useAudioLevel consulta el nivel
Then:  localLevel == 0 (track deshabilitado → sin audio → sin nivel)
```

**SC-B04-04: VU meter no causa re-render de ConversationScreen completa**
```
Given: Zustand sessionStore con selector localLevel
When:  localLevel cambia 60 veces por segundo
Then:  solo el componente VUMeter se re-renderiza
And:   ConversationScreen no re-renderiza en cada tick (selector isolation)
```

---

### REQ-B05: Reconexión automática con backoff exponencial (CA-05)

El hook `useReconnection` MUST implementar la siguiente máquina de estados:

```
CONNECTED → RECONNECTING (intento 1: espera 1s)
                       → RECONNECTING (intento 2: espera 2s)
                       → RECONNECTING (intento 3: espera 4s)
                       → FAILED
```

En cada intento de reconexión MUST:
1. Reconectar el WebSocket con `SignalingService.connect()`
2. Re-enviar el mensaje `join`
3. Re-crear el `RTCPeerConnection` con `iceRestart: true` en el offer

Si los 3 intentos fallan, el estado MUST ir a `FAILED` y la UI MUST mostrar `"Conexión perdida"` con un botón para intentar manualmente.

Si la reconexión es exitosa en cualquier intento, el contador MUST resetearse a 0.

**Escenarios**:

**SC-B05-01: Reconexión exitosa en primer intento**
```
Given: estado CONNECTED
When:  el WebSocket se cierra inesperadamente
Then:  estado cambia a RECONNECTING
And:   después de 1s se intenta reconectar
And:   si la reconexión es exitosa → estado vuelve a CONNECTED
And:   attemptCount se resetea a 0
```

**SC-B05-02: Backoff exponencial en fallos**
```
Given: servidor no disponible
When:  la conexión cae y useReconnection inicia
Then:  intento 1 ocurre después de ~1000ms (±100ms tolerancia)
And:   intento 2 ocurre después de ~2000ms del intento 1
And:   intento 3 ocurre después de ~4000ms del intento 2
And:   después del intento 3 fallido → estado == FAILED
```

**SC-B05-03: FAILED muestra UI correcta**
```
Given: 3 intentos de reconexión fallidos
Then:  la UI muestra el texto "Conexión perdida"
And:   hay un botón "Reintentar" visible
When:  el usuario presiona "Reintentar"
Then:  useReconnection reinicia desde el intento 1
```

**SC-B05-04: ICE restart en reconexión**
```
Given: SignalingService reconectado exitosamente
When:  useWebRTC intenta restablecer la sesión
Then:  RTCPeerConnection.createOffer({ iceRestart: true }) es llamado
And:   el nuevo offer es enviado al servidor
```

**SC-B05-05: No reconectar si la desconexión fue intencional**
```
Given: el usuario presionó "Finalizar" y confirmó
When:  la sesión es cerrada limpiamente
Then:  useReconnection NO inicia (disconnectReason == "user_initiated")
```

---

### REQ-B06: Background mode iOS (CA-09)

`Info.plist` MUST incluir `UIBackgroundModes: ["audio"]`.

La sesión de audio MUST configurarse como:
- Category: `.playAndRecord`
- Mode: `.voiceChat`
- Options: `.allowBluetooth`, `.allowBluetoothA2DP`, `.defaultToSpeaker`

La activación MUST ocurrir al inicio de la sesión WebRTC y la desactivación al terminarla.

**Escenarios**:

**SC-B06-01: Sesión continúa al ir a background (dispositivo físico)**
```
Given: sesión LIVE en un dispositivo iOS físico
When:  el usuario presiona el botón Home (app pasa a background)
Then:  el audio continúa fluyendo (micro y speaker)
And:   la sesión WebRTC permanece conectada
And:   al volver al foreground, la UI muestra el estado correcto
```

**SC-B06-02: AVAudioSession configurada correctamente**
```
Given: inicio de sesión WebRTC
When:  AudioService.activate() es llamado
Then:  AVAudioSession.sharedInstance().setCategory(.playAndRecord) es configurado
And:   la sesión de audio está activa
```

**SC-B06-03: AVAudioSession desactivada al terminar**
```
Given: sesión WebRTC terminada
When:  AudioService.deactivate() es llamado
Then:  AVAudioSession.sharedInstance().setActive(false) es llamado
And:   otros procesos de audio del sistema pueden recuperar la sesión
```

---

### REQ-B07: Background mode Android — Foreground Service (CA-10)

`AndroidManifest.xml` MUST declarar:
- `<uses-permission android:name="android.permission.FOREGROUND_SERVICE" />`
- `<uses-permission android:name="android.permission.FOREGROUND_SERVICE_MICROPHONE" />` (requerido Android 14+)
- `<uses-permission android:name="android.permission.RECORD_AUDIO" />`

El `CallForegroundService` MUST:
- Mostrar una notificación persistente: título `"TalkGo"`, texto `"Conversación activa"`
- Declararse como tipo `microphone` (`ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE`)
- Mantener un partial wake lock mientras esté activo
- Exponer un bridge JS via `NativeModules`: `start()` y `stop()`

**Escenarios**:

**SC-B07-01: Foreground Service arranca con la sesión**
```
Given: sesión WebRTC iniciada en Android
When:  AudioService.activate() es llamado
Then:  CallForegroundService.start() es invocado via NativeModules
And:   una notificación persistente aparece en la barra de estado
And:   la app puede pasar a background y el audio continúa
```

**SC-B07-02: Foreground Service se detiene al terminar**
```
Given: Foreground Service activo
When:  la sesión termina (finalizar confirmado o peer-left)
Then:  CallForegroundService.stop() es invocado
And:   la notificación persistente desaparece
And:   el wake lock es liberado
```

**SC-B07-03: Sesión continúa al ir a background (dispositivo físico Android 14+)**
```
Given: sesión LIVE en Android 14+ con Foreground Service activo
When:  el usuario presiona Home (app pasa a background)
Then:  el audio continúa (micro capturando, speaker reproduciendo)
And:   la notificación persiste en la barra de estado
```

---

### REQ-B08: Manejo de errores del pipeline con fallback visual (CA-07)

Cuando el servidor envía un mensaje de error del pipeline de traducción (e.g., `{"type":"error","reason":"pipeline_failure"}`), la UI MUST:
1. Mostrar un indicador visual de fallo: banner `"Error de traducción — mostrando texto original"`
2. Si el error persiste por más de 3 mensajes consecutivos, mostrar un fallback de texto en pantalla con el audio original (sin traducir) si está disponible
3. Si el pipeline se recupera (llega audio traducido nuevamente), ocultar el indicador automáticamente

**Escenarios**:

**SC-B08-01: Error único de pipeline**
```
Given: sesión LIVE
When:  el servidor envía un error de tipo pipeline_failure
Then:  aparece el banner "Error de traducción"
And:   el audio (si llega) sigue reproduciéndose
```

**SC-B08-02: Error persistente (3+ consecutivos)**
```
Given: 3 mensajes de error pipeline_failure consecutivos
Then:  el UI muestra el fallback de texto visible en pantalla
And:   el banner permanece visible
```

**SC-B08-03: Pipeline se recupera**
```
Given: banner de error visible
When:  llega un mensaje de audio/traducción exitoso
Then:  el banner desaparece automáticamente
And:   el estado de error consecutivo se resetea a 0
```

---

### REQ-B09: Bluetooth fallback al micrófono integrado (CA-08)

El `AudioService` MUST suscribirse a los eventos de cambio de ruta de audio del dispositivo. Cuando se detecta la desconexión de un dispositivo Bluetooth, MUST:
1. Redirigir el audio al micrófono y speaker integrados automáticamente
2. Mostrar una notificación toast: `"Bluetooth desconectado — usando micrófono integrado"`
3. La sesión WebRTC MUST continuar sin interrupción

**Plataformas**:
- **iOS**: `AVAudioSession.routeChangeNotification` con reason `.oldDeviceUnavailable`
- **Android**: `AudioManager.ACTION_SCO_AUDIO_STATE_UPDATED` o `AudioDeviceCallback`

**Escenarios**:

**SC-B09-01: Bluetooth se desconecta mid-sesión (iOS)**
```
Given: sesión LIVE con audio rutado a auriculares Bluetooth
When:  los auriculares se apagan o se desconectan
Then:  AVAudioSession detecta routeChangeNotification
And:   el audio se redirige al micrófono/speaker integrado automáticamente
And:   se muestra toast "Bluetooth desconectado — usando micrófono integrado"
And:   la sesión WebRTC continúa sin dropear la conexión
```

**SC-B09-02: Bluetooth se desconecta mid-sesión (Android)**
```
Given: sesión LIVE con SCO Bluetooth activo
When:  el dispositivo Bluetooth se desconecta
Then:  AudioManager detecta el evento de desconexión SCO
And:   el audio se redirige al micrófono/speaker integrado
And:   se muestra toast correspondiente
```

**SC-B09-03: Conexión Bluetooth nueva mid-sesión**
```
Given: sesión LIVE usando micrófono integrado
When:  el usuario conecta auriculares Bluetooth
Then:  el audio se redirige automáticamente al Bluetooth
And:   la sesión WebRTC continúa sin interrupción
```

---

## Acceptance Criteria Matrix

Cada fila es un criterio de aceptación del Sprint. Cada columna es un REQ que lo cubre total o parcialmente.

| CA  | Descripción                                                  | REQ-A01 | REQ-A02 | REQ-A03 | REQ-A04 | REQ-A05 | REQ-B01 | REQ-B02 | REQ-B03 | REQ-B04 | REQ-B05 | REQ-B06 | REQ-B07 | REQ-B08 | REQ-B09 |
|-----|--------------------------------------------------------------|:-------:|:-------:|:-------:|:-------:|:-------:|:-------:|:-------:|:-------:|:-------:|:-------:|:-------:|:-------:|:-------:|:-------:|
| CA-01 | Pantalla activa (no se apaga) durante sesión              |         |         |         |         |         |         |         | ✓       |         |         |         |         |         |         |
| CA-02 | VU meters responden en tiempo real                         |         |         |         |         |         |         |         |         | ✓       |         |         |         |         |         |
| CA-03 | "Finalizar" requiere confirmación                          |         |         |         |         |         |         |         | ✓       |         |         |         |         |         |         |
| CA-04 | Ambos dispositivos reciben notificación de fin             | ✓       |         |         |         |         |         |         | ✓       |         |         |         |         |         |         |
| CA-05 | Reconexión automática 3 intentos (1s, 2s, 4s)             |         |         |         |         |         |         |         |         |         | ✓       |         |         |         |         |
| CA-06 | Período de gracia 30s antes de cerrar sala                 |         | ✓       |         |         |         |         |         |         |         |         |         |         |         |         |
| CA-07 | API falla persistentemente → fallback visual               |         |         |         |         |         |         |         |         |         |         |         |         | ✓       |         |
| CA-08 | Bluetooth disconnect → fallback micrófono integrado        |         |         |         |         |         |         |         |         |         |         |         |         |         | ✓       |
| CA-09 | iOS: sesión continúa en background                         |         |         |         |         |         |         |         |         |         |         | ✓       |         |         |         |
| CA-10 | Android: Foreground Service mantiene sesión activa         |         |         |         |         |         |         |         |         |         |         |         | ✓       |         |         |
| CA-11 | Tercer usuario → error "Esta sala ya tiene 2 participantes"|         |         |         |         | ✓       |         |         |         |         |         |         |         |         |         |
| CA-12 | Sala expirada → error "Esta sala expiró"                   |         |         | ✓       |         |         |         |         |         |         |         |         |         |         |         |

---

## Constraints & NFRs

| ID     | Constraint |
|--------|------------|
| NFR-01 | Backend: `go vet ./...` y `golangci-lint run` MUST pasar con zero issues en todos los archivos modificados |
| NFR-02 | Backend: coverage ≥ 80% en archivos modificados, medido con `go test -cover` |
| NFR-03 | Backend: ningún nuevo módulo externo — solo stdlib (`time`, `crypto/rand`, `sync`) |
| NFR-04 | Mobile: TypeScript strict mode, zero errores en `tsc --noEmit` |
| NFR-05 | Mobile: el proyecto MUST buildear sin errores en iOS simulator y Android emulator |
| NFR-06 | Mobile: background mode y Bluetooth fallback MUST ser verificados en dispositivos físicos (no solo emulador) |
| NFR-07 | Arquitectura Hexagonal: `internal/domain/` MUST NOT importar `internal/adapters/` — verificado por `golangci-lint` (depguard) |
| NFR-08 | Strict TDD Mode: los escenarios de backend MUST tener tests escritos ANTES de la implementación |

---

## Open Questions

| # | Pregunta | Impacto | Owner |
|---|----------|---------|-------|
| OQ-01 | ¿`react-native-webrtc` soporta RN 0.76 estable? | Bloquea workstream B completo | Verificar antes de iniciar REQ-B01 |
| OQ-02 | ¿El backend necesita `OnDisconnect` en la interfaz `SignalingHandler` o alcanza con que Hub llame `LeaveRoom` directamente vía el Service? | Afecta limpieza de la interfaz hexagonal | Decisión en REQ-A01 |
| OQ-03 | ¿Short codes son case-sensitive en lookup? | Afecta UX del usuario dictando el código verbalmente | Propuesta: normalizar a uppercase en el handler |
