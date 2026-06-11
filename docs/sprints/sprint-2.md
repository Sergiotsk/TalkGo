# Sprint 2: Pipeline de Traducción

## Objetivo
Integrar el pipeline de traducción en tiempo real sobre la infraestructura WebRTC del Sprint 1. Cada canal de audio recibe su propia goroutine de procesamiento, se conecta con la OpenAI Realtime Translate API (speech-to-speech), y el audio traducido se devuelve como stream mono al dispositivo del interlocutor.

## Enfoque
- Implementar el port `TranslationPort` en `internal/domain/` con interface abstracta del pipeline.
- Crear el adapter `internal/adapters/translation/openai_realtime.go` que conecta con OpenAI Realtime Translate API.
- Implementar goroutines + channels para procesamiento paralelo e independiente de ambos canales de audio.
- Enrutar el audio traducido al WebRTC track del interlocutor correspondiente.
- Seguir estrictamente TDD: tests unitarios con mocks del TranslationPort antes de integrar con la API real.

## Criterios de Aceptación

Según el PRD (RF-05 y RF-06):

| ID | Criterio |
|----|---------|
| CA-01 | Latencia end-to-end (voz → audio traducido) < 1.5s en el p90 |
| CA-02 | La traducción preserva sentido contextual, no es literal palabra-por-palabra |
| CA-03 | Si A y B hablan al mismo tiempo, ambos canales se procesan en paralelo sin bloqueo mutuo |
| CA-04 | Si la traducción falla (timeout, error API), se muestra indicador visual discreto sin interrumpir el flujo |
| CA-05 | Si la API responde lento (>3s), se descarta el chunk antiguo — no se acumula backlog |
| CA-06 | El audio traducido llega como stream mono completo al dispositivo del interlocutor |

## Entregables
- `internal/domain/translation/` — port interface + tipos de dominio (AudioChunk, TranslationResult)
- `internal/adapters/translation/openai_realtime.go` — implementación del adapter
- `internal/adapters/translation/mock_translation.go` — mock para tests
- Pipeline de goroutines en el hub de sala: una goroutine por canal de audio
- Tests de integración: conversación real entre 2 participantes con idiomas diferentes

## Decisión Arquitectónica Pendiente

> Ver PRD sección 7: Opción A (pipeline 3 pasos) vs. Opción B (OpenAI Realtime) vs. Opción C (híbrida).
>
> **Recomendación MVP**: Empezar con Opción B (OpenAI Realtime) para minimizar complejidad y latencia.
> La abstracción via `TranslationPort` permite cambiar de proveedor sin tocar el dominio.
