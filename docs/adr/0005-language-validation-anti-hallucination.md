# ADR-0005 — Validación de idioma y reducción de alucinaciones en STT

**Estado:** Aceptado  
**Fecha:** 2026-06-16  
**Sprint:** 9

---

## Contexto

Durante pruebas con audio de YouTube en inglés reproduciéndose en el lado A, el STT
del lado B (`stt:es`) recibía bleed acústico (audio ambiente del speaker del otro
dispositivo). Whisper, al recibir audio en un idioma distinto al configurado, alucinaba
transcripciones en idiomas incorrectos:

```
whisper_transcript lang:"es" → "もうこれかさでしょ。"      (japonés)
whisper_transcript lang:"es" → "in den Vereinigten Staaten."  (alemán)
```

Estas transcripciones llegaban a GPT-4o, eran "traducidas" y enviadas al usuario como
frases reales. En una conversación presencial esto es un bug crítico de UX.

El filtro previo (`len([]rune) >= 3`) era insuficiente — no detecta idioma, solo longitud.

## Causas identificadas

1. **Bleed acústico**: el micrófono del usuario B captura el audio del dispositivo A.
2. **Whisper sin ancla de idioma fuerte**: configurar `language: "es"` reduce pero no
   elimina alucinaciones cuando el audio entrante es mayoritariamente de otro idioma.
3. **Sin validación post-STT**: el pipeline enviaba cualquier transcript a traducción
   sin verificar que el idioma detectado coincidiera con el esperado.

## Decisión

### Medida 1 — Prompt de anclaje en la sesión STT (Whisper)

Se agrega un campo `Prompt` a `sttTranscription` pasado en `session.update`.
Whisper usa el prompt como contexto previo para reducir alucinaciones:

```json
{ "transcription": { "model": "gpt-4o-mini-transcribe", "language": "es",
                     "prompt": "Conversación en español." } }
```

Si la API no soporta el campo, genera una advertencia `unknown_parameter` que ya
está manejada como no-fatal en el receptor.

### Medida 2 — Detección de idioma con `lingua-go`

Se agrega `github.com/pemistahl/lingua-go` al pipeline de traducción.
Antes de enviar un transcript a GPT-4o, se detecta el idioma del texto:

- Si coincide con `sourceLang` → continúa normal
- Si no coincide → se descarta con un log `pipeline_lang_mismatch_dropped`
- Textos de menos de 20 caracteres se omiten del filtro (señal insuficiente para detectar)
- Si la detección no es concluyente → se deja pasar (no bloqueante)

El detector usa `WithLowAccuracyMode()` para mantener baja latencia (<1ms por frase).

## Alternativas descartadas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| Solo filtro por charset Unicode | No distingue alemán de español (ambos latín) |
| `whatlanggo` | Menos preciso que lingua-go, mantenimiento intermitente |
| Cancelación de eco por software | Complejidad alta, requiere acceso al audio de referencia |
| Aumentar umbral de longitud mínima | No resuelve el problema — "in den Vereinigten Staaten" es largo |

## Consecuencias

**Positivas:**
- Elimina alucinaciones de idiomas incorrectos en condiciones de bleed acústico
- Cero latencia adicional: lingua-go en modo bajo consumo <1ms
- No agrega llamadas API — procesamiento local
- Prompt de anclaje mejora transcripción aun sin bleed

**Negativas:**
- Nueva dependencia: `lingua-go v1.4.0` (~8MB de modelos de lenguaje embebidos)
- El binario crece ~8MB (aceptable dado que ya es CGO con libopus)
- Frases cortas (<20 chars) en idioma incorrecto pueden pasar igualmente

## Impacto en el código

| Archivo | Cambio |
|---------|--------|
| `internal/adapters/translator/openai_whisper_stt.go` | Agregar `Prompt` a `sttTranscription`, helper `sttPromptForLang` |
| `internal/adapters/translator/pipeline_translator.go` | Agregar `langDetector` de lingua-go, método `isExpectedLanguage` |
| `go.mod` / `go.sum` | Nueva dependencia `lingua-go v1.4.0` |
