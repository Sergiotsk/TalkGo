# ADR-0004 — Reemplazo de gpt-realtime por pipeline STT → Translate → TTS

**Estado:** Aceptado
**Fecha:** 2026-06-14
**Sprint:** 8

---

## Contexto

El pipeline original usaba `gpt-realtime` como caja negra: recibía audio Opus, devolvía transcript + audio traducido en una sola conexión WebSocket. En la práctica:

- `audio_len` siempre 0 — el modelo no devuelve audio sintetizado
- El modelo ignora las instrucciones de "solo traducir" y actúa como asistente (responde preguntas, mantiene contexto conversacional, habla de cafeteras)
- `turn_detection` y otros parámetros de sesión no son soportados por la variante disponible
- Sin control sobre la calidad ni el comportamiento de traducción

## Decisión

Reemplazar `gpt-realtime` por un pipeline de tres etapas desacopladas:

```
Audio (Opus WebRTC)
    → Decode PCM (OpusCodec)
    → STT (gpt-realtime-whisper WebSocket)     ← transcripción streaming
    → Translate (gpt-4o REST)                  ← texto → texto, controlado
    → TTS (gpt-4o-mini-tts REST, PCM 24kHz)   ← síntesis de voz
    → Encode Opus (OpusCodec)
    → SendAudio (WebRTC)
    + NotifySession "transcript"               ← texto en pantalla
```

## Alternativas descartadas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| Seguir con `gpt-realtime` | No sintetiza audio, ignora instrucciones de traducción |
| `gpt-realtime-2` (gpt-4o-realtime) | No disponible en el proyecto de OpenAI actual |
| ElevenLabs TTS | Costo adicional, latencia mayor, dependencia externa extra |
| Whisper REST + GPT-4o + TTS | Whisper REST no es streaming — latencia inaceptable para tiempo real |

## Consecuencias

**Positivas:**
- Control total sobre el texto traducido — se puede loggear, validar, ajustar
- Sin comportamiento de chatbot — GPT-4o en modo translate es predecible
- TTS real con `gpt-4o-mini-tts` — voz sintetizada en el auricular
- Streaming de transcripción progresiva con deltas de Whisper
- Cada etapa es reemplazable de forma independiente

**Negativas:**
- Tres llamadas a la API por utterance en vez de una
- Latencia adicional: STT → translate → TTS en serie (~1-2s extra)
- Más superficie de error — hay que manejar fallo en cada etapa

## Modelos usados

| Etapa | Modelo | API |
|-------|--------|-----|
| STT | `gpt-realtime-whisper` | WebSocket Realtime |
| Translate | `gpt-4o` | REST `/v1/chat/completions` |
| TTS | `gpt-4o-mini-tts-2025-12-15` | REST `/v1/audio/speech` (PCM) |

## Impacto en el código

| Archivo | Cambio |
|---------|--------|
| `internal/adapters/translator/openai_realtime.go` | Reemplazar completamente — pasa a ser solo el cliente STT de Whisper |
| `internal/ports/driven/translator.go` | Sin cambios — la interfaz `TranslateStream` se mantiene |
| `internal/adapters/translator/openai_translate.go` | Nuevo — cliente REST para traducción texto→texto con GPT-4o |
| `internal/adapters/tts/openai_tts.go` | Ya existe — adapter TTS creado en Sprint 5 |
| `internal/app/roomsvc/pipeline.go` | El orquestador del pipeline no cambia — consume la misma interfaz |
