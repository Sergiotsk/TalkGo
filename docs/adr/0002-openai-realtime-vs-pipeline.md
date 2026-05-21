# ADR-0002: OpenAI Realtime Translate vs Pipeline de 3 Pasos (Whisper + GPT + ElevenLabs)

## Estado
Proposed

## Contexto
El core de TalkGo es la traducción simultánea por voz en tiempo real con baja latencia. 
Existen dos alternativas principales para implementarlo en el backend:
1. **API de OpenAI Realtime**: Conexión WebSocket persistente bidireccional que acepta audio, procesa la traducción y devuelve audio nativamente (traducción de audio a audio directa).
2. **Pipeline de 3 Pasos**: 
   - Transcribir (Speech-to-Text) con Faster Whisper o Whisper API.
   - Traducir (Text-to-Text) con GPT-4o.
   - Sintetizar (Text-to-Speech) con ElevenLabs o OpenAI TTS.

## Decisión
Proponemos utilizar la **API de OpenAI Realtime** como el motor principal de traducción simultánea para el MVP.
Sin embargo, dado el costo de la API y posibles limitaciones de rate-limiting o soporte de idiomas, diseñaremos el puerto `Translator` para que sea completamente genérico y permita intercambiarlo por una implementación de pipeline de 3 pasos (Whisper + GPT + ElevenLabs) en caso de contingencia o para optimizar costos de desarrollo local.

## Consecuencias
### Positivas
- **Latencia Mínima**: OpenAI Realtime reduce drásticamente la latencia al omitir los pasos intermedios de serialización de texto.
- **Simplicidad**: Un solo WebSocket maneja todo el flujo de traducción de audio a audio en lugar de orquestar 3 servicios diferentes.
- **Entonación**: Mantiene la entonación y matices de la voz de origen en la salida de audio.

### Negativas
- **Costos**: La API de OpenAI Realtime es sustancialmente más cara por token que los modelos de texto y TTS tradicionales.
- **Límites de uso**: Mayor propensión a topar límites de cuota (rate limits) en el MVP.
