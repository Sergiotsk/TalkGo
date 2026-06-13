# Sprint 5: Alpha con Usuarios Finales

## Objetivo
Desplegar TalkGo en un servidor real para que 5 usuarios no técnicos puedan probarlo. Reemplazar todos los stubs de desarrollo (PassthroughCodec, STUN-only, HTTP plano) con equivalentes de producción, y agregar los guardarraíles mínimos necesarios para no quemar el alpha.

## Enfoque
- Reemplazar `PassthroughCodec` con codec Opus real (`pion/opus`) para que la traducción de audio ocurra efectivamente.
- Integrar servidor TURN (Coturn) para conectividad en NAT simétrico (~35-40% de usuarios).
- HTTPS/WSS automático via Caddy como reverse proxy con Let's Encrypt.
- Deployment en VPS Hetzner CX11 São Paulo via Docker Compose (Go server + Coturn + Caddy).
- Rate limiting stdlib-only por IP para controlar costos de OpenAI.
- Eventos de error estructurados sobre el WebSocket existente para feedback al usuario.
- Endpoint `POST /feedback` para recolectar experiencias del alpha.
- Distribución mobile via Expo Go — sin TestFlight ni APK firmado.

## Criterios de Aceptación

| ID | Criterio |
|----|---------|
| CA-01 | Audio codificado con `OpusCodec.Encode()` y decodificado con `OpusCodec.Decode()` produce PCM16 válido (test de round-trip) |
| CA-02 | El pipeline end-to-end funciona con el codec real: captura → decode → translate → encode → envío |
| CA-03 | `CODEC_MODE=passthrough` restaura el comportamiento anterior sin romper tests existentes |
| CA-04 | El servidor acepta credenciales TURN desde variables de entorno; STUN-only cuando `TURN_URLS` está vacío |
| CA-05 | Un cliente detrás de NAT simétrico se conecta exitosamente vía relay Coturn |
| CA-06 | `https://<dominio>/health` devuelve 200; WebSocket conecta via `wss://<dominio>/ws/<roomID>` |
| CA-07 | `docker compose up -d` en VPS vacío levanta los 3 servicios (talkgo, coturn, caddy) sin errores |
| CA-08 | Superar el límite de `POST /rooms` o `GET /ws` por IP devuelve HTTP 429 con header `Retry-After` |
| CA-09 | El servidor envía `error:ice-failed`, `error:translation` y `error:codec` al cliente via WebSocket |
| CA-10 | `POST /feedback` con payload válido devuelve 201 y produce una entrada slog estructurada |
| CA-11 | `GET /health` reporta presencia de `TURN_URLS` y `OPENAI_API_KEY` sin exponer valores |
| CA-12 | Un tester en iOS o Android con Expo Go puede unirse a una sala y escuchar audio traducido |

## Entregables
- `OpusCodec` real en `internal/adapters/codec/opus.go` (único dep Go nuevo: `pion/opus`)
- Configuración TURN en `internal/adapters/webrtc/config.go` (`BuildICEConfig()`)
- Rate limiter middleware en `internal/adapters/http/ratelimit.go`
- Eventos de error estructurados via `NotifySession` + constantes en `internal/adapters/signaling/errors.go`
- Handler `POST /feedback` en el servidor HTTP
- `loadConfig()` centralizado en `cmd/server/main.go`
- Ops: `Dockerfile`, `docker-compose.yml`, `Caddyfile`, `coturn.conf`, `.env.example`
- Docs: `docs/deploy/vps-setup.md`, `docs/deploy/expo-go-guide.md`
