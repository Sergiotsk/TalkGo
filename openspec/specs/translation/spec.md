# Sprint 2 Spec: Pipeline de Traducción

## Metadata

- Change: sprint-2
- Status: Draft
- Date: 2026-06-10
- Author: sdd-spec agent

---

## Scope

### In scope

- Pipeline de traducción bidireccional entre dos participantes de una sala
- Port `driven.AudioCodec` con operaciones `Decode` (Opus→PCM) y `Encode` (PCM→Opus)
- Extensión del port `driven.WebRTCPeer` con `OnAudioTrack` y `SendAudio`
- Port `driven.Translator` (ya existente) — wiring con el pipeline
- Adapter `internal/adapters/codec/opus_codec.go`
- Adapter `internal/adapters/translator/openai_realtime.go`
- Campo `Lang string` en `Session` y en `SignalingMessage`
- Campos `SourceLang`, `TargetLang string` en `Chunk`
- Orquestación del pipeline en `Service.JoinRoom`, `Service.LeaveRoom` y `Service.DeleteRoom`
- Backpressure: descarte de chunks cuando el translator supera 3 segundos
- Notificación al cliente via `SignalingMessage` cuando el translator falla
- Cancelación limpia del pipeline sin goroutine leaks

### Out of scope

- AudioMixer (mezcla de múltiples fuentes de audio)
- Fallback Whisper + GPT + ElevenLabs
- Frontend / UI de errores
- Persistencia de traducciones en base de datos
- Métricas, trazas y observabilidad (OpenTelemetry, Prometheus, etc.)
- Reconexión automática hacia la API de OpenAI cuando cae la conexión
- TURN server / ICE relay

---

## Requirements (RFC 2119)

### REQ-01: Pipeline bidireccional

Cuando dos participantes están en la misma sala, el servicio MUST iniciar exactamente dos pipelines independientes: uno que transporte el audio de A traducido hacia B, y otro que transporte el audio de B traducido hacia A. Cada pipeline corre en su propia goroutine y no comparte estado mutable con el otro.

### REQ-02: Extracción de audio por participante

El port `driven.WebRTCPeer` MUST exponer `OnAudioTrack(ctx context.Context, sessionID string, handler func(<-chan []byte)) error`. Este método MUST registrar el handler antes de que llegue el primer frame de audio. El channel devuelto al handler MUST cerrarse cuando el contexto se cancela o cuando el peer cierra la pista de audio.

### REQ-03: Decodificación Opus→PCM

El port `driven.AudioCodec` MUST exponer `Decode(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error)`. La implementación MUST convertir cada frame Opus a PCM16 a 24 kHz mono antes de enviarlo al canal de salida. El canal de salida MUST cerrarse cuando `opusIn` se cierra o el contexto se cancela.

### REQ-04: Traducción de audio

El port `driven.Translator` MUST exponer `TranslateStream(ctx context.Context, audio <-chan []byte, sourceLang, targetLang string) (<-chan []byte, error)`. El Translator MUST recibir PCM16 como entrada y devolver PCM16 traducido como salida. Si el contexto se cancela, el canal de salida MUST cerrarse limpiamente.

### REQ-05: Codificación PCM→Opus

El port `driven.AudioCodec` MUST exponer `Encode(ctx context.Context, pcmIn <-chan []byte) (<-chan []byte, error)`. La implementación MUST convertir cada frame PCM16 a Opus antes de enviarlo al canal de salida. El canal de salida MUST cerrarse cuando `pcmIn` se cierra o el contexto se cancela.

### REQ-06: Envío de audio traducido al interlocutor

El port `driven.WebRTCPeer` MUST exponer `SendAudio(ctx context.Context, sessionID string, audio <-chan []byte) error`. El servicio MUST llamar a `SendAudio` con el `sessionID` del participante destinatario (no del origen). Si el contexto se cancela, el método MUST retornar sin enviar frames adicionales.

### REQ-07: Procesamiento paralelo sin bloqueo mutuo

Si el participante A y el participante B hablan simultáneamente, los dos pipelines MUST procesar sus respectivos audios de forma concurrente. Un pipeline bloqueado (por ejemplo, esperando respuesta del translator) MUST NOT bloquear el otro pipeline.

### REQ-08: Backpressure — descarte de chunks lentos

Si el canal de entrada del translator tiene un chunk pendiente y el translator tarda más de 3 segundos en procesarlo, el servicio MUST descartar el chunk más antiguo del buffer y continuar procesando el siguiente. El servicio MUST NOT acumular un backlog ilimitado de chunks. El umbral de 3 segundos MUST ser configurable vía parámetro de construcción del pipeline (valor por defecto: 3s).

### REQ-09: Notificación de error del translator

Si `TranslateStream` retorna un error o cierra el canal de salida prematuramente, el servicio MUST enviar un `SignalingMessage` de tipo `"error"` al cliente afectado con un campo `Reason` descriptivo. El pipeline del participante afectado MUST continuar intentando procesar nuevos chunks (estrategia de reinicio por chunk). El pipeline del participante opuesto MUST NOT verse afectado.

### REQ-10: Cancelación limpia del pipeline

Cuando `Service.LeaveRoom` es llamado para un participante, el servicio MUST cancelar el contexto de los pipelines en los que ese participante participa. Cuando `Service.DeleteRoom` es llamado, MUST cancelar los contextos de todos los pipelines de la sala. En ambos casos, todas las goroutines del pipeline MUST terminar antes de que el método retorne (o en un plazo máximo de 5 segundos con timeout de cierre). No MUST quedar goroutine leaks verificables con `goleak`.

### REQ-11: Derivación de la dirección de traducción

La dirección de traducción MUST derivarse del campo `Lang` del participante origen (registrado en `Session.Lang`) y de los campos `SourceLang` / `TargetLang` del `Room`. Si `Session.Lang` coincide con `Room.SourceLang`, la traducción va de `SourceLang` a `TargetLang`. Si coincide con `Room.TargetLang`, va en sentido inverso. Si no coincide con ninguno, el servicio MUST retornar un error descriptivo al unirse (`JoinRoom`).

### REQ-12: Session con campo Lang

`domain.Session` MUST incluir el campo `Lang string`. `NewSession` MUST aceptar `lang string` como parámetro y almacenarlo. `Service.JoinRoom` MUST extraer `Lang` del campo homónimo del `SignalingMessage` recibido y pasarlo a `NewSession`. Si `Lang` está vacío, `JoinRoom` MUST retornar `ErrMissingLang`.

### REQ-13: Wiring en el constructor del Service

`NewService` MUST aceptar `Translator driven.Translator` y `AudioCodec driven.AudioCodec` como parámetros. Si alguno es `nil`, MUST retornar `ErrNilDependency`. El `Service` MUST almacenarlos para usarlos en `startPipeline`.

### REQ-14: Pipeline iniciado solo con sala completa

`Service.JoinRoom` MUST llamar a `startPipeline` únicamente cuando hay exactamente 2 participantes en la sala después del join. Si hay menos de 2, el pipeline NO MUST iniciarse. Si la sala ya tenía 2 participantes y se intenta un tercer join, MUST retornarse `ErrRoomFull`.

---

## Scenarios

### SC-01: Happy path — audio de A llega traducido a B

**Given** una sala con `SourceLang: "es"`, `TargetLang: "en"`,
  y el participante A con `Lang: "es"` ya unido y con ICE completo,
  y el participante B con `Lang: "en"` se une y completa ICE,
**When** A envía un frame de audio Opus por su pista de WebRTC,
**Then** el pipeline de A:
  1. recibe el frame via `OnAudioTrack` en el channel de Opus,
  2. lo decodifica a PCM via `AudioCodec.Decode`,
  3. lo traduce de "es" a "en" via `Translator.TranslateStream`,
  4. lo codifica a Opus via `AudioCodec.Encode`,
  5. lo entrega a B via `WebRTCPeer.SendAudio(sessionID_B, ...)`.

### SC-02: Happy path — audio de B llega traducido a A

**Given** la misma sala del SC-01, con A y B unidos y ambos pipelines activos,
**When** B envía un frame de audio Opus por su pista de WebRTC,
**Then** el pipeline de B:
  1. recibe el frame via `OnAudioTrack` en el channel de Opus de B,
  2. lo decodifica a PCM,
  3. lo traduce de "en" a "es",
  4. lo codifica a Opus,
  5. lo entrega a A via `WebRTCPeer.SendAudio(sessionID_A, ...)`.

### SC-03: Habla simultánea — ambos pipelines procesan en paralelo

**Given** una sala con A y B unidos y ambos pipelines activos,
**When** A y B envían frames de audio al mismo tiempo,
**Then** ambos pipelines procesan sus respectivos frames de forma concurrente;
  el frame de A aparece en B y el frame de B aparece en A sin que ninguno bloquee al otro;
  el mock de `Translator` para A y el mock para B son llamados concurrentemente (verificable con `sync.WaitGroup` en el test).

### SC-04: Backpressure — translator lento, chunk viejo descartado

**Given** una sala con A y B unidos,
  y el mock de `Translator` para el pipeline de A tiene un delay artificial de 4 segundos por chunk,
**When** A envía 3 frames de audio consecutivos,
**Then** el pipeline descarta al menos el segundo frame (el más antiguo en el buffer cuando expira el timeout de 3s);
  el tercer frame es procesado o también descartado si llega durante el delay;
  el pipeline de B MUST NOT verse afectado ni bloqueado por la lentitud del translator de A.

### SC-05: Error de API — translator falla, notificación enviada, pipeline continúa

**Given** una sala con A y B unidos,
  y el mock de `Translator` para el pipeline de A está configurado para retornar `errors.New("openai: rate limit exceeded")` en la primera llamada y éxito en la segunda,
**When** A envía un primer frame de audio (que falla) y luego un segundo frame de audio,
**Then**:
  - se envía un `SignalingMessage{Type: "error", Reason: "translation failed: openai: rate limit exceeded"}` a A;
  - el pipeline de A continúa activo y procesa el segundo frame sin error;
  - B recibe el audio traducido del segundo frame;
  - el pipeline de B MUST NOT haberse interrumpido en ningún momento.

### SC-06: Participante sale — pipeline cancelado limpiamente

**Given** una sala con A y B unidos y ambos pipelines activos,
**When** `Service.LeaveRoom(sessionID_A)` es llamado,
**Then**:
  - el contexto de los pipelines que involucran a A es cancelado;
  - todas las goroutines de esos pipelines terminan (verificable con `goleak.VerifyNone`);
  - B recibe un `SignalingMessage{Type: "peer_left"}` (comportamiento ya existente del Sprint 1);
  - el método `LeaveRoom` retorna `nil`.

### SC-07: Room eliminada — ambos pipelines cancelados

**Given** una sala con A y B unidos y ambos pipelines activos,
**When** `Service.DeleteRoom(roomID)` es llamado,
**Then**:
  - el contexto de todos los pipelines de la sala es cancelado;
  - todas las goroutines de ambos pipelines terminan (verificable con `goleak.VerifyNone`);
  - el método `DeleteRoom` retorna `nil`.

### SC-08: Lang incorrecto en join — error claro retornado

**Given** una sala con `SourceLang: "es"`, `TargetLang: "en"`,
**When** un participante intenta unirse con `SignalingMessage{Lang: "fr"}`,
**Then** `Service.JoinRoom` retorna un error que contiene el texto `"lang fr not supported in room"` (o equivalente);
  la sala MUST NOT quedar en estado inconsistente;
  el participante MUST NOT aparecer como unido en la sala.

### SC-09: Segundo participante se une antes de que el primero complete ICE — pipeline espera estado correcto

**Given** A se une a la sala pero su estado ICE todavía está en `"checking"` (no `"connected"`),
**When** B se une a la sala (completando el conteo de 2 participantes),
**Then** `startPipeline` es invocado pero MUST NOT llamar a `OnAudioTrack` ni `SendAudio` para A hasta que el estado ICE de A transite a `"connected"`;
  una vez A completa ICE, el pipeline arranca normalmente y el audio fluye end-to-end.

### SC-10: AudioCodec decode falla — error propagado correctamente

**Given** una sala con A y B unidos,
  y el mock de `AudioCodec.Decode` está configurado para retornar `errors.New("codec: unsupported frame size")` al ser llamado,
**When** A envía un frame de audio Opus,
**Then**:
  - el pipeline de A captura el error de `Decode`;
  - se envía un `SignalingMessage{Type: "error", Reason: "audio decode failed: codec: unsupported frame size"}` a A;
  - el pipeline de A intenta continuar con el siguiente frame (no termina abruptamente);
  - no quedan goroutines colgadas (verificable con `goleak.VerifyNone`).

---

## Interface Contracts

Las siguientes firmas son normativas. La implementación MUST respetar estos contratos exactos.

```go
// driven/audio_codec.go
type AudioCodec interface {
    Decode(ctx context.Context, opusIn <-chan []byte) (<-chan []byte, error)
    Encode(ctx context.Context, pcmIn  <-chan []byte) (<-chan []byte, error)
}

// driven/webrtc_peer.go (extensión)
type WebRTCPeer interface {
    // ... métodos existentes del Sprint 1 ...
    OnAudioTrack(ctx context.Context, sessionID string, handler func(<-chan []byte)) error
    SendAudio(ctx context.Context, sessionID string, audio <-chan []byte) error
}

// driven/translator.go
type Translator interface {
    TranslateStream(ctx context.Context, audio <-chan []byte, sourceLang, targetLang string) (<-chan []byte, error)
}
```

```go
// domain/session.go (campo agregado)
type Session struct {
    ID     string
    RoomID string
    Lang   string // nuevo
}

// domain/chunk.go (campos agregados)
type Chunk struct {
    Data       []byte
    SourceLang string // nuevo
    TargetLang string // nuevo
}

// domain/signaling_message.go (campo agregado)
type SignalingMessage struct {
    Type   string
    Lang   string // nuevo
    Reason string // nuevo (para mensajes de error)
    // ... campos existentes ...
}
```

---

## Error Catalog

| Sentinel / Constante | Situación |
|----------------------|-----------|
| `ErrMissingLang` | `SignalingMessage.Lang` vacío en `JoinRoom` |
| `ErrLangNotSupported` | `Lang` no coincide con `SourceLang` ni `TargetLang` del Room |
| `ErrRoomFull` | Se intenta unir un tercer participante a una sala de 2 |
| `ErrNilDependency` | `NewService` recibe `Translator` o `AudioCodec` nil |

---

## Test Infrastructure

- Todos los ports driven MUST tener mocks generados con `mockery` o escritos a mano en `internal/adapters/mock/`.
- Los tests de pipeline MUST usar `goleak.VerifyNone(t)` como `defer` al inicio de cada test que arranque goroutines.
- Los tests de backpressure MUST usar canales con buffer controlado y timers deterministas (sin `time.Sleep` en el SUT; usar clock inyectable si es necesario).
- Cada escenario SC-01 a SC-10 MUST tener al menos un test `TestService_<SC>` en `internal/domain/service_test.go` usando table-driven format cuando aplique.
