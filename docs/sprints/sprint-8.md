# Sprint 8 — Pipeline STT → Translate → TTS

**Objetivo:** Reemplazar `gpt-realtime` por un pipeline de tres etapas desacopladas que garantice traducción correcta y síntesis de voz real.

**Estado:** En progreso
**ADR:** [ADR-0004](../adr/0004-stt-translate-tts-pipeline.md)

---

## Contexto

`gpt-realtime` actúa como asistente conversacional ignorando las instrucciones de traducción y no devuelve audio sintetizado (`audio_len=0` siempre). Se reemplaza por un pipeline controlado de tres etapas.

## Pipeline objetivo

```
mic → Opus (WebRTC)
    → Decode PCM (OpusCodec existente)
    → gpt-realtime-whisper (STT streaming WebSocket)
         ↓ transcript final
    → gpt-4o (traducción texto → texto, REST)
         ↓ texto traducido
    → gpt-4o-mini-tts (síntesis voz, REST PCM 24kHz)
         ↓ PCM
    → Encode Opus (OpusCodec existente)
    → SendAudio WebRTC → auricular
    + NotifySession "transcript" → pantalla
```

---

## Requerimientos

### REQ-STT-01 — Adapter STT (gpt-realtime-whisper)
Reemplazar `openai_realtime.go` para usar `gpt-realtime-whisper`.

**Session update:**
```json
{
  "type": "session.update",
  "session": {
    "type": "transcription",
    "audio": {
      "input": {
        "format": { "type": "audio/pcm", "rate": 24000 },
        "transcription": { "model": "gpt-realtime-whisper" }
      }
    }
  }
}
```

**Eventos a escuchar:**
- `conversation.item.input_audio_transcription.delta` → acumular texto parcial
- `conversation.item.input_audio_transcription.completed` → emitir transcript final

**Criterio de aceptación:**
- El transcript refleja lo que realmente se dijo (no alucinaciones de chatbot)
- Los deltas llegan mientras se habla (no solo al final)

### REQ-TRANS-01 — Adapter Translate (gpt-4o)
Nuevo adapter REST que traduce texto de `sourceLang` a `targetLang`.

**Prompt del sistema:**
```
You are a professional interpreter. Translate the following text from {sourceLang} to {targetLang}.
Output ONLY the translation. No explanations, no alternatives, no notes.
```

**Criterio de aceptación:**
- Traduce el texto exacto recibido sin agregar contenido
- Maneja textos cortos (1 palabra) y largos (párrafos)

### REQ-TTS-01 — Integración TTS en pipeline
El adapter TTS (`openai_tts.go`) ya existe. Integrarlo después de la traducción.

**Criterio de aceptación:**
- Se escucha la voz traducida en el celular receptor
- El TTS usa `gpt-4o-mini-tts-2025-12-15`
- El audio PCM retorna a 24kHz mono para compatibilidad con OpusCodec

### REQ-PIPE-01 — Orquestación en TranslateStream
El port `driven.Translator` no cambia. `TranslateStream` orquesta internamente STT → Translate → TTS.

**Criterio de aceptación:**
- `pipeline.go` no requiere cambios
- Los canales `Audio` y `Transcript` de `TranslateResult` funcionan igual que antes

---

## Tareas

### FASE 1 — STT Adapter
- [ ] TASK-STT-01: Reescribir `openai_realtime.go` como cliente STT puro (Whisper)
- [ ] TASK-STT-02: Manejar eventos `transcription.delta` y `transcription.completed`
- [ ] TASK-STT-03: Actualizar tests del adapter translator

### FASE 2 — Translate Adapter
- [ ] TASK-TRANS-01: Crear `internal/adapters/translator/openai_translate.go`
- [ ] TASK-TRANS-02: Tests del adapter translate

### FASE 3 — Orquestación
- [ ] TASK-PIPE-01: Crear `internal/adapters/translator/pipeline_translator.go` que une STT → Translate → TTS
- [ ] TASK-PIPE-02: Implementa `driven.Translator` — orquesta las tres etapas
- [ ] TASK-PIPE-03: Actualizar `cmd/server/main.go` para usar `PipelineTranslator`

### FASE 4 — Verificación
- [ ] TASK-VER-01: Tests de integración del pipeline completo
- [ ] TASK-VER-02: Deploy y prueba en VPS con dos dispositivos
- [ ] TASK-VER-03: Verificar transcript correcto en pantalla
- [ ] TASK-VER-04: Verificar audio de voz en auricular

---

## Riesgos

| Riesgo | Probabilidad | Mitigación |
|--------|-------------|-----------|
| `gpt-realtime-whisper` session format diferente al documentado | Media | Loggear todos los eventos y ajustar según respuesta real |
| Latencia STT → translate → TTS demasiado alta | Media | Medir con logs; si >3s, explorar streaming de traducción |
| PCM rate mismatch entre TTS y codec | Baja | TTS ya configurado en 24kHz en el adapter existente |
| `gpt-4o-mini-tts` no disponible en el proyecto | Baja | Fallback a `tts-1` si el ID falla |
