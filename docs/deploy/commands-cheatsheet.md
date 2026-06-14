# TalkGo — Commands Cheatsheet

## VPS — Docker

| Comando | Para qué sirve |
|---------|----------------|
| `docker logs talkgo-talkgo-1 -f --since 0s 2>&1` | Ver logs en tiempo real desde ahora |
| `docker logs talkgo-talkgo-1 --since 5m 2>&1` | Ver logs de los últimos 5 minutos |
| `docker logs talkgo-talkgo-1 --tail 100 2>&1` | Ver las últimas 100 líneas |
| `docker compose up --build -d` | Rebuild y levantar en background |
| `docker compose down` | Bajar todos los containers |
| `docker compose restart talkgo` | Reiniciar solo el container del server |
| `docker compose ps` | Ver estado de los containers |
| `docker stats` | Ver uso de CPU/memoria en tiempo real |
| `docker exec -it talkgo-talkgo-1 sh` | Shell dentro del container |

## VPS — Deploy

| Comando | Para qué sirve |
|---------|----------------|
| `git pull && docker compose up --build -d` | Pull + rebuild + deploy (el comando de siempre) |
| `git pull` | Solo actualizar código (sin rebuild) |
| `git log --oneline -10` | Ver últimos 10 commits en el VPS |

## Git — Local

| Comando | Para qué sirve |
|---------|----------------|
| `git status` | Ver archivos modificados |
| `git diff` | Ver cambios sin stagear |
| `git add -p` | Stagear cambios interactivamente (recomendado) |
| `git commit -m "tipo: descripción"` | Commit con conventional commits |
| `git push` | Push a origin (dispara CI) |
| `git log --oneline -10` | Ver últimos 10 commits |

## Go — Local

| Comando | Para qué sirve |
|---------|----------------|
| `go build ./...` | Verificar que compila (siempre antes de push) |
| `gofmt -w ./...` | Formatear todo el proyecto (evita fallas de CI) |
| `gofmt -d archivo.go` | Ver diff de formato sin aplicar |
| `go test -race -cover ./...` | Correr todos los tests con detector de race conditions |
| `go test ./internal/adapters/translator/...` | Correr tests de un paquete específico |
| `go test -run TestNombre ./...` | Correr un test específico por nombre |
| `go vet ./...` | Análisis estático rápido |

## Coturn (TURN server)

| Comando | Para qué sirve |
|---------|----------------|
| `docker logs talkgo-coturn-1 -f` | Ver logs del TURN server |
| `docker exec talkgo-coturn-1 turnadmin -l` | Ver sesiones TURN activas |

## Filtrar logs — Tips

```bash
# Ver solo errores
docker logs talkgo-talkgo-1 --since 0s 2>&1 | grep '"level":"ERROR"'

# Ver solo transcripts
docker logs talkgo-talkgo-1 --since 0s 2>&1 | grep 'pipeline_transcript'

# Ver audio sending (para verificar que llega a OpenAI)
docker logs talkgo-talkgo-1 --since 0s 2>&1 | grep 'openai_audio_sending'

# Ver solo una dirección (ej: español → inglés)
docker logs talkgo-talkgo-1 --since 0s 2>&1 | grep 'es.*en'
```

## Mobile — Expo

| Comando | Para qué sirve |
|---------|----------------|
| `npx expo start` | Levantar Metro bundler |
| `npx expo start --clear` | Levantar limpiando caché (cuando algo no actualiza) |
| `npx expo run:android` | Build nativo Android (requiere Android Studio) |
| `eas build --profile development --platform android` | Build APK de desarrollo en la nube |
| `eas build --profile preview --platform android` | Build APK de preview para testing |

## Diagnóstico rápido de sesión

Cuando creás una sala nueva, buscá estos eventos en orden para verificar que todo funciona:

```
1. session_start (x2)          → ambos usuarios conectados
2. pipeline_start               → pipeline iniciado
3. openai_realtime_connected    → conectado a OpenAI (x2 direcciones)
4. openai_session_ready         → sesión OpenAI lista
5. openai_audio_sending frame:1 bytes:960  → audio Opus puro llegando
6. pipeline_transcript          → traducción generada
7. notify_session_sent type:transcript → enviada al celular
```

Si alguno no aparece, ahí está el problema.
