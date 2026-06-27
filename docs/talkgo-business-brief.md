# TalkGo — Business Brief para análisis de rentabilidad

> Fecha: Junio 2026  
> Propósito: Deep research sobre viabilidad, costos y posicionamiento competitivo

---

## ¿Qué es TalkGo?

Plataforma de **traducción de audio en tiempo real** para conversaciones presenciales multilingüe.
Dos personas que hablan idiomas distintos se unen a una sala; cada una escucha al otro con su voz
traducida en tiempo real a su propio idioma.

**Stack técnico:**
- Backend: Go 1.22+, Arquitectura Hexagonal estricta
- Audio P2P: WebRTC via Pion (Go) + Opus codec (CGO, libopus)
- Signaling: WebSocket
- NAT traversal: TURN server (coturn)
- Mobile: React Native / Expo (iOS + Android)
- Deploy: Docker (talkgo + coturn + caddy)

---

## Pipeline de audio actual

La arquitectura es **Hexagonal** — el adaptador de traducción es intercambiable sin modificar nada más.
El puerto `driven.Translator` define una sola interfaz:

```go
TranslateStream(ctx, audioIn <-chan []byte, sourceLang, targetLang string) (TranslateResult, error)
```

Hay dos implementaciones activas:

### Opción A — `OpenAIRealtimeTranslator` (1 etapa)

```
Opus RTP → PCM → [OpenAI Realtime API: gpt-4o-realtime-preview] → PCM → Opus
```

STT + traducción + TTS en un solo WebSocket. Más simple, más costoso.

### Opción B — `PipelineTranslator` (3 etapas, activa hoy)

```
Opus RTP → PCM
  → STT:       gpt-4o-mini-transcribe   (WebSocket Realtime)
  → Translate: gpt-4o                   (REST Chat Completions)
  → TTS:       gpt-4o-mini-tts          (REST Audio)
→ PCM → Opus
```

---

## Estimación de costos actuales (100% OpenAI)

### Opción A — gpt-4o-realtime-preview

| Item | Precio OpenAI | Por minuto de sala |
|------|--------------|-------------------|
| Audio input | ~$40/1M tokens audio | ~$0.06/min |
| Audio output | ~$80/1M tokens audio | ~$0.24/min |
| **Total/sala** | | **~$0.30/min = $18/hora por sala** |

### Opción B — Pipeline 3 etapas (activo)

| Etapa | Modelo | Costo estimado |
|-------|--------|----------------|
| STT | gpt-4o-mini-transcribe | ~$0.003/min |
| Translate | gpt-4o ($2.50/1M in, $10/1M out) | ~$0.01/min (10 frases/min, ~100 tokens c/u) |
| TTS | gpt-4o-mini-tts ($15/1M chars) | ~$0.015/min |
| **Total/sala** | | **~$0.03/min = $1.80/hora por sala** |

> El Pipeline es ~10x más barato que Realtime, pero sigue siendo caro a escala.
> Con ~$2 gastados en pruebas, el costo por sala activa es insostenible sin modelo de pricing.

---

## Alternativas técnicas (para evaluar en deep research)

### STT (Speech-to-Text)

| Opción | Precio | Latencia | Notas |
|--------|--------|----------|-------|
| OpenAI gpt-4o-mini-transcribe | ~$0.003/min | Media | Actual |
| OpenAI Whisper API (batch) | $0.006/min | Alta (~2s) | No viable para real-time |
| Deepgram Nova-2 (streaming) | $0.0043/min | Baja (<300ms) | Mejor precio/latencia |
| AssemblyAI Streaming | $0.0067/min | Baja | Buena precisión |
| Google Cloud STT | $0.016/min | Baja | Más caro |
| **Whisper local (self-hosted)** | **$0** (costo infra) | Variable | Viable en B2B |

### Traducción (Text)

| Opción | Precio | Calidad |
|--------|--------|---------|
| GPT-4o | $2.50/$10 por 1M tokens | Alta — actual |
| GPT-4o-mini | $0.15/$0.60 por 1M tokens | Buena — 16x más barato |
| DeepL API | $5.49/1M chars | Muy alta para EU/ES |
| Google Translate API | $20/1M chars | Alta |
| LibreTranslate (self-hosted) | $0 | Media |
| **OPUS-MT Helsinki NLP (self-hosted)** | **$0** | Media-Alta |

### TTS (Text-to-Speech)

| Opción | Precio | Naturalidad |
|--------|--------|-------------|
| OpenAI TTS mini | $15/1M chars | Alta — actual |
| ElevenLabs | $0.18/1M chars (Creator plan) | Excelente |
| Google Cloud TTS Neural2 | $4/1M chars | Alta |
| Azure Neural TTS | $16/1M chars | Alta |
| **Coqui TTS (self-hosted)** | **$0** | Media |
| **Kokoro (self-hosted)** | **$0** | Alta |

### Combinación más económica estimada (sin self-hosting)

| Etapa | Alternativa | Costo/min |
|-------|------------|-----------|
| STT | Deepgram Nova-2 | $0.0043 |
| Translate | GPT-4o-mini | ~$0.001 |
| TTS | ElevenLabs | ~$0.003 |
| **Total** | | **~$0.008/min = $0.50/hora por sala** |

> vs $1.80/hora actual = reducción del ~72% sin self-hosting.

---

## Análisis competitivo

### Competidores directos gratuitos

#### Google Translate — Live Translate (2026)
- Traducción en tiempo real vía auriculares (expandida a iOS en marzo 2026)
- Motor: **Gemini**
- Auto-detecta quién habla, cambia idioma de salida automáticamente
- Compatible con cualquier auricular Bluetooth 5.4
- **Precio: $0**
- Modelo: 1 teléfono + auriculares (no multi-dispositivo nativo)

#### Microsoft Translator — Multi-device Conversation
- **El competidor más parecido a TalkGo**
- Cada persona entra desde su propio dispositivo (iOS, Android, Web, Windows)
- Todos hablan en su idioma y reciben traducción en el suyo
- +70 idiomas, transcripción en tiempo real
- **Precio: $0**
- API disponible vía Azure Cognitive Services (Speech Service)

### Tabla comparativa

| | Google Live Translate | Microsoft Translator | TalkGo (actual) |
|---|---|---|---|
| Dispositivos | 1 teléfono + auriculares | Múltiples (por sesión) | Múltiples (WebRTC) |
| Ecosistema requerido | Google | Microsoft | **Ninguno** |
| Self-hosteable | No | No | **Sí** |
| API/SDK white-label | No | Sí (Azure) | **Potencial** |
| Control de datos | Google cloud | MS cloud | **Tu infra** |
| Costo para usuario final | Free | Free | ? |
| Latencia objetivo | Media | Media | **<500ms (Opus streaming)** |
| Codec audio | Propietario | Propietario | **Opus (estándar abierto)** |
| Privacidad de datos | ❌ Google cloud | ❌ MS cloud | ✅ Self-hosteable |

---

## Diagnóstico de posicionamiento

### Mercado consumer: NO competir

Google y Microsoft tienen distribución global, marca y precio $0.
En el caso de uso turista-turista o amigo-amigo, TalkGo no puede ganar.

### Mercado con potencial: B2B con restricciones de privacidad

Organizaciones que **no pueden mandar audio a Google o Microsoft** por compliance:

| Vertical | Regulación | Dolor |
|----------|-----------|-------|
| Hospitales / clínicas | HIPAA (EE.UU.), RGPD (EU) | Audio de pacientes en cloud externo = violación |
| Estudios jurídicos | Privilegio abogado-cliente | Confidencialidad de conversaciones |
| Gobiernos / consulados | Soberanía de datos | No pueden usar cloud extranjero |
| Hotelería enterprise | RGPD, datos de huéspedes | Responsabilidad legal |
| Educación corporativa | Políticas internas | Datos de empleados |
| Call centers multilingüe | Compliance sectorial | Grabación + traducción en infra propia |

### Ventaja diferencial de TalkGo

1. **Self-hosteable**: Docker image lista — se despliega en infra del cliente
2. **Sin lock-in de ecosistema**: No requiere cuenta Google/Microsoft/Apple
3. **Control total del audio**: El audio nunca sale de la red del cliente
4. **Arquitectura hexagonal**: El proveedor de AI es intercambiable (Deepgram, Azure, on-premise)
5. **Open codec**: Opus es estándar abierto — interoperable con cualquier sistema de videoconferencia
6. **White-label SDK**: Potencial de integrarse en apps existentes de los clientes

---

## Modelos de negocio posibles

### SaaS B2B (recomendado para explorar)
- Precio por sala activa/mes o por minuto consumido
- Tiers por volumen de uso
- Add-on: transcripción persistente, analytics, integración con EHR/CRM

### On-premise / Private Cloud
- Licencia anual + soporte
- El cliente hostea en su propia infra
- Especialmente atractivo para gobierno y salud

### White-label SDK
- Vender el SDK a ISVs (software de hotelería, telemedicina, etc.)
- Revenue share o licencia flat

### Pay-per-use para consumer (freemium)
- Free: N minutos/mes
- Pro: precio/mes con minutos incluidos
- Riesgo: compite directo con Google/MS gratuito

---

## Preguntas clave para el deep research

1. ¿A qué volumen de salas concurrentes conviene self-hosting (Whisper + OPUS-MT + Kokoro) vs API externa?
2. ¿La combinación Deepgram + GPT-4o-mini + ElevenLabs reduce costos ~72% sin sacrificar calidad perceptible?
3. ¿Existe algún modelo multimodal open source (audio-in → audio-out) viable para self-hosting con latencia <500ms?
4. ¿Cuál es el willingness-to-pay real en verticales B2B (salud, legal, gobierno) para una solución self-hosteable?
5. ¿Microsoft Azure Speech multi-device conversation tiene restricciones de datos que abran oportunidad para TalkGo?
6. ¿Cuál es el costo total de ownership de un stack 100% self-hosted vs API en escenario de 100 salas concurrentes?
7. ¿Hay funding o grants para herramientas de comunicación accesible/multilingüe (ONG, gobierno)?

---

## Archivos clave del proyecto

| Archivo | Propósito |
|---------|-----------|
| `internal/adapters/translator/pipeline_translator.go` | Orquesta las 3 etapas STT→Translate→TTS |
| `internal/adapters/translator/openai_realtime.go` | Alternativa 1-etapa con Realtime API |
| `internal/adapters/translator/openai_whisper_stt.go` | STT via gpt-4o-mini-transcribe |
| `internal/adapters/translator/openai_translate.go` | Traducción via gpt-4o Chat Completions |
| `internal/adapters/tts/openai_tts.go` | TTS via gpt-4o-mini-tts |
| `internal/ports/driven/` | Interfaces — aquí se define el contrato del Translator |
| `internal/app/roomsvc/` | Orquestación de sala + pipeline |
