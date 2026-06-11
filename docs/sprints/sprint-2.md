# Sprint 2: Pipeline de Traducción

**Estado**: ✅ COMPLETADO — 2026-06-10
**Veredicto SDD**: PASS WITH WARNINGS
**Artefactos**: `openspec/changes/archive/2026-06-10-sprint-2/`

## Objetivo
Integrar el pipeline de traducción en tiempo real sobre la infraestructura WebRTC del Sprint 1. Cada canal de audio recibe su propia goroutine de procesamiento, se conecta con la OpenAI Realtime Translate API (speech-to-speech), y el audio traducido se devuelve como stream mono al dispositivo del interlocutor.

## Arquitectura implementada

```
PionPeer.OnTrack → [opus/8] → AudioCodec.Decode → [pcm] → drainOldest → [bp/1]
  → Translator.TranslateStream → [pcm] → AudioCodec.Encode → [opus] → PionPeer.SendAudio
```

Dos cadenas en paralelo (una por participante). 6 goroutines por pipeline. Orquestadas desde `Service.startPipeline`.

## Entregables implementados

| Entregable | Archivo | Estado |
|-----------|---------|--------|
| Port `AudioCodec` | `internal/ports/driven/audio_codec.go` | ✅ |
| Port `EventNotifier` | `internal/ports/driven/event_notifier.go` | ✅ |
| `WebRTCPeer` extendido | `internal/ports/driven/webrtc_peer.go` | ✅ |
| Pipeline logic | `internal/app/roomsvc/pipeline.go` | ✅ |
| SC-01 a SC-10 tests | `internal/app/roomsvc/pipeline_test.go` | ✅ |
| `PionPeer` SendRecv | `internal/adapters/webrtc/pion_peer.go` | ✅ |
| `OpenAIRealtimeTranslator` | `internal/adapters/translator/openai_realtime.go` | ✅ |
| `PassthroughCodec` (stub) | `internal/adapters/codec/opus_codec.go` | ⚠️ stub |
| Hub implementa EventNotifier | `internal/adapters/signaling/hub.go` | ✅ |

## Criterios de Aceptación

| ID | Criterio | Estado |
|----|---------|--------|
| CA-01 | Latencia end-to-end < 1.5s en p90 | ⏳ Verificar con dispositivos reales (Sprint 4) |
| CA-02 | Traducción preserva sentido contextual (OpenAI Realtime) | ✅ Por diseño de la API |
| CA-03 | Habla simultánea — procesamiento paralelo sin bloqueo | ✅ SC-03 |
| CA-04 | Si API falla, notificación sin interrumpir el flujo | ⚠️ Notifica pero el half muere (ver deuda técnica) |
| CA-05 | API lenta → descarte del chunk viejo | ✅ drainOldest, SC-04 |
| CA-06 | Audio traducido llega al interlocutor | ✅ SC-01, SC-02 |

## Deuda técnica

| # | Deuda | Prioridad |
|---|-------|-----------|
| DT-01 | `PassthroughCodec` → codec Opus real (libopus) en CI/Linux | Alta — bloqueante para producción |
| DT-02 | Reinicio por chunk en `runHalf` cuando translator falla | Media |
| DT-03 | `goleak.VerifyNone` en tests de goroutines | Baja |

## Cobertura final

- `internal/app/roomsvc`: **87.5%**
- `internal/domain/*`: **100%**
- Lint: ✅ zero warnings
- Race detector: ✅ zero races
