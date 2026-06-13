# Sprint 5 Spec — Alpha con Usuarios Finales

**Change**: sprint-5
**Status**: spec
**Date**: 2026-06-12

---

## Overview

Sprint 5 lleva TalkGo a una alpha desplegable con usuarios reales. Las cinco áreas de trabajo son:

1. **REQ-COD** — Codec Opus real (reemplaza PassthroughCodec en producción)
2. **REQ-NET** — Soporte TURN/Coturn + TLS vía Caddy
3. **REQ-OPS** — Dockerfile, docker-compose, docs de despliegue
4. **REQ-RATE** — Rate limiting HTTP (stdlib only, sin dependencias externas)
5. **REQ-UX** — Mensajes de error estructurados + endpoint /feedback
6. **REQ-MOB** — Distribución Expo Go a testers

Cada escenario es concreto y verificable. Los escenarios de backend (REQ-COD, REQ-NET, REQ-RATE, REQ-UX) se traducen a Go tests. Los de infraestructura (REQ-OPS) se verifican mediante inspección de archivos y comandos Docker. Los de mobile (REQ-MOB) son verificación manual + inspección de config.

---

## REQ-COD — Opus Codec

### REQ-COD-01: OpusCodec decodifica Opus → PCM16 24kHz mono

**Given** un `OpusCodec` inicializado con un decoder Opus 24kHz mono
**When** se llama `Decode(ctx, opusFrame)` con un frame Opus válido
**Then** retorna bytes PCM16 little-endian a 24kHz mono, sin error

**Acceptance criteria:**
- [ ] `OpusCodec` implementa `driven.AudioCodec` (compilación lo garantiza)
- [ ] `Decode` retorna `[]byte` con largo `> 0` para un frame Opus de prueba conocido
- [ ] El output es PCM16 (cada sample = 2 bytes, little-endian)
- [ ] No hay goroutine leak: al cancelar el context, el codec no deja goroutines colgadas
- [ ] El test usa un frame Opus sintético (20ms, 24kHz mono) generado en el setup

**Test approach:** unit — `internal/adapters/codec/opus_codec_test.go`. Genera un frame Opus sintético con un encoder de prueba, pásalo al decoder, verifica el largo y que los primeros 2 bytes sean parseable como int16. Usa `goleak` o verifica goroutines con `runtime.NumGoroutine()` antes/después.

---

### REQ-COD-02: OpusCodec codifica PCM16 24kHz mono → Opus

**Given** un `OpusCodec` inicializado con un encoder Opus 24kHz mono
**When** se llama `Encode(ctx, pcm16Bytes)` con samples PCM16 válidos (múltiplo de 2 bytes)
**Then** retorna un frame Opus válido (`len > 0`), sin error

**Acceptance criteria:**
- [ ] `Encode` retorna `[]byte` con largo `> 0` para un input PCM16 de prueba
- [ ] El output es decodificable por un decoder Opus (round-trip: encode → decode → PCM distinto de cero)
- [ ] `Encode` retorna error si el input no es múltiplo de 2 bytes (sample incompleto)
- [ ] `Encode` con PCM de silencio (todos ceros) retorna un frame Opus válido (no error)

**Test approach:** unit — mismo archivo `opus_codec_test.go`. Round-trip: genera PCM16 de tono 440Hz, codifica, decodifica, verifica que el PCM resultante no sea todo ceros. Test de error con input de largo impar.

---

### REQ-COD-03: main.go selecciona codec según CODEC_MODE

**Given** la variable de entorno `CODEC_MODE`
**When** `CODEC_MODE=opus` (o ausente, valor por defecto)
**Then** se instancia `OpusCodec` y se inyecta en el `Service`

**When** `CODEC_MODE=passthrough`
**Then** se instancia `PassthroughCodec` y se inyecta en el `Service`

**Acceptance criteria:**
- [ ] `main.go` lee `CODEC_MODE` con `os.Getenv`, default `"opus"`
- [ ] `switch` o `if` selecciona entre `OpusCodec` y `PassthroughCodec`
- [ ] Si `CODEC_MODE` es cualquier otro valor, el servidor imprime error y termina con `os.Exit(1)`
- [ ] El log de arranque incluye `codec_mode` en el campo estructurado (`slog.Info`)

**Test approach:** integration manual — arrancar el servidor con `CODEC_MODE=passthrough`, verificar en los logs que dice `codec_mode=passthrough`. La lógica de selección es tan simple que no requiere test unitario adicional; la cobertura viene de los tests de integración de arranque (`cmd/server/main_test.go` si existe).

---

### REQ-COD-04: PassthroughCodec sigue disponible

**Given** el entorno de desarrollo o testing
**When** `CODEC_MODE=passthrough`
**Then** `PassthroughCodec` retorna los bytes de entrada sin modificación, sin error

**Acceptance criteria:**
- [ ] `PassthroughCodec.Encode(ctx, data)` retorna `data` sin modificación
- [ ] `PassthroughCodec.Decode(ctx, data)` retorna `data` sin modificación
- [ ] `PassthroughCodec` continúa en `internal/adapters/codec/passthrough.go` (no se mueve ni elimina)
- [ ] Los tests existentes de `PassthroughCodec` pasan sin cambios

**Test approach:** unit — los tests existentes validan esto. Si no existen, agregar uno trivial que verifique el round-trip identidad.

---

### REQ-COD-05: Codec maneja cierre de canal y cancelación de context

**Given** un `OpusCodec` en uso
**When** el context es cancelado (ej.: sala cerrada) mientras hay operaciones en vuelo
**Then** `Encode`/`Decode` retorna `ctx.Err()` sin panic, y no quedan goroutines huérfanas

**Acceptance criteria:**
- [ ] `Encode` y `Decode` respetan `ctx.Done()`: si el context está cancelado al inicio, retornan inmediatamente con error
- [ ] No se producen panics por write en canal cerrado
- [ ] `goleak.VerifyNone(t)` (o equivalente) pasa al final del test que cancela el context
- [ ] El codec no lanza goroutines propias (es síncrono); si usa goroutines internas, deben terminar al cancelar el context

**Test approach:** unit — test que cancela el context antes de llamar `Encode`/`Decode`, verifica error y ausencia de goroutine leak. Si el codec es puramente síncrono, documentar que no aplica goroutine leak y verificar solo el retorno de error.

---

## REQ-NET — Network / TURN

### REQ-NET-01: webrtc.Config acepta TURN URLs y credenciales

**Given** la configuración de Pion WebRTC en el servidor
**When** se construye `webrtc.Configuration`
**Then** el struct acepta `ICEServers` con tipo `TURN` incluyendo URLs, Username y Credential

**Acceptance criteria:**
- [ ] Existe una función (ej.: `buildICEConfig(turnURLs, user, pass string) webrtc.Configuration`) en un archivo dentro de `internal/adapters/` o `cmd/server/`
- [ ] La función acepta URLs TURN y retorna una `webrtc.Configuration` con `ICEServers` poblado
- [ ] Los campos `Username` y `Credential` son seteados en el `ICEServer`
- [ ] El código compila sin errores

**Test approach:** unit — test que llama `buildICEConfig` con valores de prueba y verifica que `config.ICEServers[0].URLs` contenga la URL TURN y que `Username`/`Credential` sean los esperados.

---

### REQ-NET-02: TURN_URLS env var agrega servidores TURN a la config ICE

**Given** la variable de entorno `TURN_URLS` seteada (ej.: `turn:mi-servidor.com:3478`)
**When** el servidor arranca
**Then** la `webrtc.Configuration` usada para crear `PeerConnection` incluye ese servidor TURN

**Acceptance criteria:**
- [ ] `main.go` lee `TURN_URLS`, `TURN_USERNAME`, `TURN_PASSWORD` via `os.Getenv`
- [ ] Si `TURN_URLS` es no-vacío, se agrega al menos un `ICEServer` con `URLs` populado
- [ ] El log de arranque incluye `turn_urls_set=true` o similar cuando hay TURN configurado
- [ ] `TURN_URLS` acepta múltiples URLs separadas por coma

**Test approach:** unit sobre `buildICEConfig`. Integration manual: arrancar con `TURN_URLS=turn:localhost:3478` y verificar en logs.

---

### REQ-NET-03: Sin TURN_URLS, comportamiento es STUN-only (backward compatible)

**Given** `TURN_URLS` no seteada o vacía
**When** el servidor arranca
**Then** la `webrtc.Configuration` usa solo servidores STUN (comportamiento actual)

**Acceptance criteria:**
- [ ] Si `TURN_URLS` está vacío, `ICEServers` contiene solo servidores STUN (ej.: `stun:stun.l.google.com:19302`)
- [ ] No se produce error ni warning por ausencia de TURN
- [ ] El comportamiento de Sprint 1–4 no cambia cuando `TURN_URLS` está ausente

**Test approach:** unit — `buildICEConfig("", "", "")` retorna config con exactamente los STUN servers por defecto y sin ICEServers de tipo TURN.

---

### REQ-NET-04: Coturn Docker acepta allocaciones TURN

**Given** el servicio `coturn` definido en `docker-compose.yml`
**When** se ejecuta `docker compose up coturn`
**Then** el contenedor arranca y responde a una allocación TURN en el puerto configurado

**Acceptance criteria:**
- [ ] `docker-compose.yml` define el servicio `coturn` con imagen oficial `coturn/coturn` (o equivalente)
- [ ] El servicio expone el puerto TURN (default 3478 UDP/TCP)
- [ ] El archivo `coturn.conf` (o config inline) define `realm`, `user`, y `lt-cred-mech`
- [ ] Manual: `docker compose up coturn` no termina en error inmediatamente
- [ ] Manual: `turnutils_uclient` (o equivalente) puede hacer una allocación exitosa contra `localhost:3478`

**Test approach:** manual + inspección de archivos Docker. No se escribe Go test para esto.

---

### REQ-NET-05: Caddy termina TLS y proxea HTTP/WebSocket al servidor Go

**Given** el servicio `caddy` en `docker-compose.yml`
**When** `docker compose up`
**Then** Caddy acepta HTTPS en el puerto 443 y proxea a `talkgo:8080`

**Acceptance criteria:**
- [ ] `docker-compose.yml` define el servicio `caddy` con imagen `caddy:alpine` (o equivalente)
- [ ] `Caddyfile` (o config inline) define el bloque de site con `reverse_proxy talkgo:8080`
- [ ] El `Caddyfile` maneja tanto peticiones HTTP normales como upgrades WebSocket
- [ ] Manual en VPS: `curl https://mi-dominio/health` retorna `200 OK`
- [ ] Manual en VPS: cliente WebSocket puede conectar via `wss://mi-dominio/ws/{roomID}`

**Test approach:** inspección de archivos de config + prueba manual en VPS. No se escribe Go test.

---

## REQ-OPS — Operations / Deployment

### REQ-OPS-01: Dockerfile produce imagen Go binaria mínima (<20MB)

**Given** el `Dockerfile` en la raíz del repositorio
**When** se ejecuta `docker build -t talkgo .`
**Then** la imagen resultante pesa menos de 20MB

**Acceptance criteria:**
- [ ] `Dockerfile` usa build multi-stage: primera etapa `golang:1.23-alpine` compila el binario, segunda etapa `scratch` o `alpine` solo copia el binario
- [ ] El binario se compila con `CGO_ENABLED=0 GOOS=linux`
- [ ] `docker images talkgo --format "{{.Size}}"` reporta < 20MB
- [ ] El contenedor arranca con `docker run --rm talkgo` sin error de runtime

**Test approach:** inspección del `Dockerfile` + `docker build` + `docker images` manual.

---

### REQ-OPS-02: docker-compose.yml define los tres servicios

**Given** el archivo `docker-compose.yml` en la raíz
**When** se inspecciona el archivo
**Then** contiene los servicios `talkgo`, `coturn`, y `caddy` correctamente configurados

**Acceptance criteria:**
- [ ] Servicio `talkgo`: usa el `Dockerfile` local, expone el puerto interno 8080, tiene `restart: unless-stopped`
- [ ] Servicio `coturn`: imagen pública de Coturn, expone 3478 UDP+TCP, monta config de credenciales
- [ ] Servicio `caddy`: imagen Caddy, expone 80 y 443, monta `Caddyfile`, depende de `talkgo`
- [ ] Los servicios están en la misma red Docker para comunicación interna por nombre
- [ ] Las variables de entorno sensibles (`OPENAI_API_KEY`, `TURN_PASSWORD`) se referencian vía `.env` o `env_file`

**Test approach:** inspección de archivo. Manual: `docker compose config` no reporta errores de sintaxis.

---

### REQ-OPS-03: Toda la configuración es via variables de entorno con defaults sensatos

**Given** el servidor TalkGo
**When** se arranca sin ninguna variable de entorno seteada
**Then** arranca correctamente con configuración por defecto (excepto `OPENAI_API_KEY` que es requerida)

**Acceptance criteria:**
- [ ] `PORT` default `8080`
- [ ] `CODEC_MODE` default `"opus"`
- [ ] `TURN_URLS` default `""` (STUN-only)
- [ ] `RATE_LIMIT_ROOMS` default razonable (ej.: `10` requests/minuto por IP)
- [ ] `RATE_LIMIT_WS` default razonable (ej.: `20` conexiones/minuto por IP)
- [ ] Si `OPENAI_API_KEY` está ausente, el servidor imprime un error claro y termina con `os.Exit(1)`
- [ ] `main.go` tiene una función `loadConfig()` (o equivalente) que centraliza la lectura de env vars

**Test approach:** inspección de `main.go` + manual: arrancar sin env vars (excepto `OPENAI_API_KEY`) y verificar que no crashea.

---

### REQ-OPS-04: `docker compose up` levanta todos los servicios y se comunican

**Given** un VPS con Docker instalado y las env vars configuradas
**When** se ejecuta `docker compose up -d`
**Then** los tres servicios están `healthy` y el servidor Go responde en `localhost:8080`

**Acceptance criteria:**
- [ ] `docker compose ps` muestra los tres servicios en estado `running`
- [ ] `curl http://localhost:8080/health` retorna `200 OK` desde el host (Go directo)
- [ ] `curl https://mi-dominio/health` retorna `200 OK` via Caddy+TLS
- [ ] Coturn responde en el puerto 3478 (verificado con `nc -u localhost 3478`)
- [ ] Los logs de `talkgo` no muestran errores de conexión con Coturn

**Test approach:** prueba manual completa en VPS después del despliegue.

---

### REQ-OPS-05: Docs de despliegue en docs/deploy/

**Given** el directorio `docs/deploy/`
**When** un desarrollador nuevo quiere desplegar TalkGo en un VPS
**Then** encuentra documentación suficiente para hacerlo sin ayuda adicional

**Acceptance criteria:**
- [ ] Existe `docs/deploy/README.md` o `docs/deploy/vps-setup.md` con pasos de VPS setup (requisitos, Docker install)
- [ ] La guía cubre: DNS apuntando al VPS, configuración de `.env`, `docker compose up`
- [ ] La guía cubre configuración de Coturn: `realm`, credenciales, puerto firewall
- [ ] La guía cubre renovación automática de certificados via Caddy
- [ ] La guía incluye comandos de troubleshooting (ver logs, reiniciar servicios)

**Test approach:** inspección de archivos en `docs/deploy/`. Revisión humana de completitud.

---

### REQ-OPS-06: GET /health reporta TURN y API key

**Given** el endpoint `GET /health`
**When** se llama estando el servidor en funcionamiento
**Then** retorna un JSON con el estado de TURN y presencia de API key

**Acceptance criteria:**
- [ ] `GET /health` retorna `200 OK` con `Content-Type: application/json`
- [ ] El body incluye `"turn_configured": true/false` según si `TURN_URLS` está seteado
- [ ] El body incluye `"api_key_present": true/false` según si `OPENAI_API_KEY` está seteado (nunca expone el valor)
- [ ] El body incluye `"status": "ok"`
- [ ] El endpoint no requiere autenticación
- [ ] Existe un test en `internal/adapters/http/server_test.go` que verifica los tres campos

**Test approach:** unit — test HTTP con `httptest.NewRecorder` que verifica el body JSON del `/health`. También prueba manual.

---

## REQ-RATE — Rate Limiting

### REQ-RATE-01: Rate limiting en POST /rooms por IP

**Given** el middleware de rate limiting activo en `POST /rooms`
**When** una misma IP hace más requests que el límite configurado en la ventana de tiempo
**Then** las requests excedentes reciben `429 Too Many Requests`

**Acceptance criteria:**
- [ ] Las primeras N requests (N = `RATE_LIMIT_ROOMS`, default 10/min) retornan `200` o `201`
- [ ] La request N+1 en la misma ventana retorna `429 Too Many Requests`
- [ ] La respuesta 429 incluye header `Retry-After` con segundos hasta reset
- [ ] La respuesta 429 incluye body JSON `{"error": "rate-limited", "retry_after_seconds": N}`
- [ ] Requests de IPs distintas tienen buckets independientes

**Test approach:** unit — test en `internal/adapters/http/server_test.go`. Simula N+1 requests desde la misma IP (usando `X-Forwarded-For` o `RemoteAddr`), verifica que la última retorna `429` con el body correcto.

---

### REQ-RATE-02: Rate limiting en GET /ws/{roomID} por IP

**Given** el middleware de rate limiting activo en `GET /ws/{roomID}`
**When** una misma IP intenta más conexiones WebSocket que el límite configurado
**Then** las conexiones excedentes son rechazadas con `429 Too Many Requests` antes del upgrade

**Acceptance criteria:**
- [ ] Las primeras M intentos de upgrade (M = `RATE_LIMIT_WS`, default 20/min) son procesados
- [ ] El intento M+1 retorna `429` con el mismo formato JSON que REQ-RATE-01
- [ ] El rate limit de `/ws/` es independiente del de `/rooms`
- [ ] El rechazo ocurre ANTES del WebSocket upgrade (el `429` es una respuesta HTTP normal)

**Test approach:** unit — test con `httptest` que hace M+1 requests a `/ws/testroom`, verifica el código HTTP de la última sin completar el WebSocket upgrade.

---

### REQ-RATE-03: Límites configurables via variables de entorno

**Given** las variables `RATE_LIMIT_ROOMS` y `RATE_LIMIT_WS`
**When** se setean a valores específicos antes de arrancar
**Then** el rate limiter usa esos valores como límite de requests por ventana

**Acceptance criteria:**
- [ ] `RATE_LIMIT_ROOMS` parsea a `int`; si el parsing falla, usa el default y loguea warning
- [ ] `RATE_LIMIT_WS` parsea a `int`; misma regla
- [ ] El log de arranque incluye `rate_limit_rooms=N` y `rate_limit_ws=M`
- [ ] Si `RATE_LIMIT_ROOMS=0`, se interpreta como "sin límite" (deshabilita el rate limiting para ese endpoint)

**Test approach:** unit — test que instancia el rate limiter con distintos valores y verifica el comportamiento. Verificación de parsing en `loadConfig`.

---

### REQ-RATE-04: Rate limiter usa solo stdlib (sin dependencias externas)

**Given** la implementación del rate limiter
**When** se inspecciona el código y el `go.mod`
**Then** no se agregan nuevas dependencias externas de terceros para rate limiting

**Acceptance criteria:**
- [ ] El rate limiter está implementado con `sync.Mutex` + `map[string]*bucket` (token bucket o sliding window) usando solo `time` y `sync` del stdlib
- [ ] `go.mod` no agrega nuevas entradas en `require` para esta feature
- [ ] El código del rate limiter está en `internal/adapters/http/ratelimit.go` (u otro path en el mismo paquete)

**Test approach:** inspección de `go.mod` antes/después de la implementación. Code review del archivo del rate limiter.

---

### REQ-RATE-05: Rate limiter limpia entradas stale para evitar memory leak

**Given** el rate limiter con entradas por IP
**When** una IP no hace requests durante más de la ventana de tiempo configurada (ej.: 2x la ventana)
**Then** su entrada es eliminada del mapa interno

**Acceptance criteria:**
- [ ] El rate limiter tiene un goroutine de cleanup (o limpieza lazy en cada request) que elimina entries más viejas que la ventana
- [ ] Si se usa cleanup en goroutine: se inicia con `context.Context` y termina cuando el context se cancela (sin goroutine leak)
- [ ] Test: crear N entradas, avanzar el tiempo virtual o esperar el TTL, verificar que `len(limiter.entries)` vuelve a 0
- [ ] No se produce OOM con 10,000 IPs distintas haciendo una sola request cada una a lo largo del tiempo

**Test approach:** unit — test que agrega entradas, avanza el tiempo (usando clock injectable o `time.Sleep` corto), verifica que el mapa se limpia. Si el cleanup es lazy, verificar que una request posterior de la misma IP no usa el entry viejo.

---

## REQ-UX — Error UX + Feedback

### REQ-UX-01: Server envía `error:ice-failed` cuando ICE falla

**Given** una conexión WebRTC en progreso
**When** el ICE connection state pasa a `Failed`
**Then** el servidor envía un mensaje de error estructurado al cliente vía WebSocket

**Acceptance criteria:**
- [ ] El mensaje tiene la forma: `{"type": "error", "code": "ice-failed", "message": "ICE connection failed", "session_id": "<id>"}`
- [ ] El campo `session_id` está presente cuando la sesión fue establecida; ausente o `""` si no
- [ ] El mensaje es enviado antes de cerrar la conexión WebSocket
- [ ] Existe un test que simula el callback de ICE state change con `Failed` y verifica el mensaje enviado

**Test approach:** unit — en el handler de `onICEConnectionStateChange` del `PeerConnection`, mock del canal WebSocket de envío. Verificar que el mensaje correcto es encolado.

---

### REQ-UX-02: Server envía `error:translation` cuando OpenAI API falla

**Given** el pipeline de traducción activo
**When** la llamada a la OpenAI Realtime API retorna un error (timeout, 5xx, auth error)
**Then** el servidor envía `{"type": "error", "code": "translation", "message": "...", "session_id": "<id>"}` al cliente

**Acceptance criteria:**
- [ ] El mensaje tiene `"code": "translation"` y un `"message"` legible (no el error interno de Go)
- [ ] El `session_id` está incluido
- [ ] El error de OpenAI es logueado internamente con nivel `slog.Error` (para debugging)
- [ ] El cliente NO recibe el error interno de Go (no se expone stack trace ni mensaje crudo de la API)
- [ ] Existe un test que inyecta un `Translator` mock que retorna error y verifica el mensaje WebSocket

**Test approach:** unit — test en `internal/app/roomsvc/service_test.go` o `pipeline_test.go` con `Translator` mock. Verificar mensaje enviado via `EventNotifier` mock.

---

### REQ-UX-03: Server envía `error:codec` cuando Opus falla

**Given** el pipeline de audio activo
**When** `OpusCodec.Decode` o `OpusCodec.Encode` retorna error
**Then** el servidor envía `{"type": "error", "code": "codec", "message": "...", "session_id": "<id>"}` al cliente

**Acceptance criteria:**
- [ ] El mensaje tiene `"code": "codec"` y un `"message"` legible (ej.: `"audio codec error"`)
- [ ] El error de codec es logueado con `slog.Error`
- [ ] El pipeline no entra en pánico ni queda colgado después de un error de codec
- [ ] Existe un test con `AudioCodec` mock que retorna error y verifica el mensaje enviado

**Test approach:** unit — test en `pipeline_test.go` con `AudioCodec` mock que retorna error en `Decode`. Verifica que `EventNotifier.Notify` es llamado con el mensaje de error correcto.

---

### REQ-UX-04: Server envía `error:rate-limited` con `retry_after_seconds`

**Given** el middleware de rate limiting
**When** una request es rechazada por exceder el límite
**Then** la respuesta incluye el código de error estructurado

**Acceptance criteria:**
- [ ] Body JSON: `{"type": "error", "code": "rate-limited", "retry_after_seconds": N}` donde N es el tiempo hasta reset en segundos (entero positivo)
- [ ] Header HTTP: `Retry-After: N` (RFC 7231)
- [ ] Status HTTP: `429 Too Many Requests`
- [ ] El `retry_after_seconds` coincide con el valor del header `Retry-After`
- [ ] Existe un test que verifica el body JSON completo de la respuesta 429

**Test approach:** unit — mismo test de REQ-RATE-01/02, extender para verificar el body JSON y el header `Retry-After`.

---

### REQ-UX-05: POST /feedback acepta feedback válido y lo loguea

**Given** el endpoint `POST /feedback`
**When** se envía un body JSON válido con los campos requeridos
**Then** el servidor retorna `200 OK` y loguea el feedback con `slog.Info`

**Acceptance criteria:**
- [ ] El endpoint acepta `Content-Type: application/json`
- [ ] Body mínimo: `{"session_id": "...", "rating": 1-5, "comment": "..."}` (comment opcional)
- [ ] `session_id` es requerido y no vacío; si falta, retorna `400 Bad Request`
- [ ] `rating` debe ser entero entre 1 y 5 inclusive; si fuera de rango, retorna `400`
- [ ] En éxito, retorna `{"status": "ok"}` con `200 OK`
- [ ] El feedback es logueado: `slog.Info("feedback received", "session_id", ..., "rating", ..., "comment", ...)`

**Test approach:** unit — test en `server_test.go`. Tabla de casos: válido (200), sin session_id (400), rating=0 (400), rating=6 (400), body inválido JSON (400).

---

### REQ-UX-06: POST /feedback rechaza input inválido

**Given** el endpoint `POST /feedback`
**When** se envía un body malformado o con valores fuera de rango
**Then** retorna `400 Bad Request` con un mensaje de error descriptivo

**Acceptance criteria:**
- [ ] Body vacío → `400` con `{"error": "invalid request body"}`
- [ ] JSON inválido → `400` con `{"error": "invalid request body"}`
- [ ] `rating` < 1 → `400` con `{"error": "rating must be between 1 and 5"}`
- [ ] `rating` > 5 → `400` con `{"error": "rating must be between 1 and 5"}`
- [ ] `session_id` ausente o vacío → `400` con `{"error": "session_id is required"}`

**Test approach:** unit — tabla de tests en `server_test.go` cubriendo todos los casos de error anteriores. Verificar el body JSON de la respuesta de error en cada caso.

---

### REQ-UX-07: Todos los mensajes de error incluyen session_id cuando disponible

**Given** cualquier mensaje de error enviado por WebSocket (REQ-UX-01 a REQ-UX-03)
**When** la sesión ha sido establecida (el cliente completó el `join`)
**Then** el campo `session_id` está presente y es no-vacío en el mensaje de error

**Acceptance criteria:**
- [ ] El tipo `ErrorMessage` (o equivalente struct) tiene campo `SessionID string \`json:"session_id,omitempty"\``
- [ ] La función que construye mensajes de error recibe el `sessionID` como parámetro
- [ ] Si `sessionID == ""` (sesión no establecida), el campo es omitido del JSON (via `omitempty`)
- [ ] Los tests de REQ-UX-01, REQ-UX-02, REQ-UX-03 verifican que `session_id` está presente en el JSON

**Test approach:** unit — los tests de REQ-UX-01/02/03 ya cubren esto. Agregar asserción explícita de `session_id` en cada uno.

---

## REQ-MOB — Mobile / Expo Go

### REQ-MOB-01: Cliente React Native configurado con URL WSS de producción

**Given** el cliente React Native (en `client/` o directorio equivalente)
**When** se inspecciona la configuración del cliente
**Then** la URL del servidor WebSocket apunta al endpoint de producción con WSS

**Acceptance criteria:**
- [ ] Existe un archivo de configuración (ej.: `client/src/config.ts` o `.env`) con `WS_URL=wss://mi-dominio/ws`
- [ ] La URL usa `wss://` (WebSocket Secure), no `ws://`
- [ ] El cliente NO tiene hardcodeada `localhost` como URL de producción
- [ ] El switch dev/prod se hace via variable de entorno (`EXPO_PUBLIC_*` o equivalente), no por comentar/descomentar código

**Test approach:** inspección de archivos de config del cliente. No se escribe Go test para esto.

---

### REQ-MOB-02: docs/deploy/expo-go-guide.md documenta el flujo de tester

**Given** el archivo `docs/deploy/expo-go-guide.md`
**When** un tester sin experiencia en desarrollo quiere probar TalkGo
**Then** puede seguir la guía sin necesidad de asistencia adicional

**Acceptance criteria:**
- [ ] El archivo existe en `docs/deploy/expo-go-guide.md`
- [ ] La guía cubre: instalar Expo Go desde App Store/Play Store
- [ ] La guía cubre: escanear el QR code o ingresar la URL manualmente
- [ ] La guía cubre: permisos de micrófono requeridos
- [ ] La guía cubre: qué hacer si la conexión falla (troubleshooting básico)
- [ ] La guía NO requiere que el tester instale Node.js, npm, ni ningún SDK de desarrollo

**Test approach:** inspección del archivo + revisión humana de completitud y claridad.

---

### REQ-MOB-03: Servidor acepta conexiones WebSocket de clientes Expo Go (sin bloqueo por origen)

**Given** el servidor TalkGo en producción
**When** un cliente Expo Go intenta conectar a `wss://mi-dominio/ws/{roomID}`
**Then** la conexión es aceptada sin rechazo por validación de origen (origin check)

**Acceptance criteria:**
- [ ] El servidor NO valida el header `Origin` contra una whitelist estricta para el endpoint `/ws/`
- [ ] Si el upgrader Gorilla/WebSocket tiene `CheckOrigin`, está seteado a `func(r *http.Request) bool { return true }` o equivalente permisivo
- [ ] Existe un test que simula un WebSocket upgrade con `Origin: null` o un origen arbitrario y verifica que es aceptado (no recibe 403)
- [ ] El comportamiento permisivo está comentado en el código con una nota sobre la decisión de seguridad (Expo Go no envía origin predecible)

**Test approach:** unit — en `server_test.go`, test de upgrade WebSocket con header `Origin` seteado a un valor arbitrario. Verifica que retorna `101 Switching Protocols`, no `403 Forbidden`. También verificación de la config del upgrader en code review.

---

## Resumen de Tests Requeridos

| Req | Tipo | Archivo |
|-----|------|---------|
| REQ-COD-01 | unit | `internal/adapters/codec/opus_codec_test.go` |
| REQ-COD-02 | unit | `internal/adapters/codec/opus_codec_test.go` |
| REQ-COD-03 | manual | — |
| REQ-COD-04 | unit | `internal/adapters/codec/passthrough_test.go` (existente) |
| REQ-COD-05 | unit | `internal/adapters/codec/opus_codec_test.go` |
| REQ-NET-01 | unit | `internal/adapters/webrtc/config_test.go` (nuevo) |
| REQ-NET-02 | unit | `internal/adapters/webrtc/config_test.go` |
| REQ-NET-03 | unit | `internal/adapters/webrtc/config_test.go` |
| REQ-NET-04 | manual | — |
| REQ-NET-05 | manual | — |
| REQ-OPS-01 | manual | — |
| REQ-OPS-02 | manual | — |
| REQ-OPS-03 | manual | — |
| REQ-OPS-04 | manual | — |
| REQ-OPS-05 | inspección | — |
| REQ-OPS-06 | unit + manual | `internal/adapters/http/server_test.go` |
| REQ-RATE-01 | unit | `internal/adapters/http/server_test.go` |
| REQ-RATE-02 | unit | `internal/adapters/http/server_test.go` |
| REQ-RATE-03 | unit | `internal/adapters/http/ratelimit_test.go` (nuevo) |
| REQ-RATE-04 | inspección | — |
| REQ-RATE-05 | unit | `internal/adapters/http/ratelimit_test.go` |
| REQ-UX-01 | unit | handler de PeerConnection (pipeline/service test) |
| REQ-UX-02 | unit | `internal/app/roomsvc/pipeline_test.go` |
| REQ-UX-03 | unit | `internal/app/roomsvc/pipeline_test.go` |
| REQ-UX-04 | unit | `internal/adapters/http/server_test.go` |
| REQ-UX-05 | unit | `internal/adapters/http/server_test.go` |
| REQ-UX-06 | unit | `internal/adapters/http/server_test.go` |
| REQ-UX-07 | unit | (cubierto por REQ-UX-01/02/03) |
| REQ-MOB-01 | inspección | — |
| REQ-MOB-02 | inspección | — |
| REQ-MOB-03 | unit | `internal/adapters/http/server_test.go` |
