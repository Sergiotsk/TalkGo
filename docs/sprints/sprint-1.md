# Sprint 1: WebRTC Signaling & Room Management

## Objetivo
Implementar la infraestructura de señalización WebRTC (Pion) y la gestión dinámica de salas (Room lifecycle) en el backend, permitiendo que múltiples dispositivos se conecten y establezcan conexiones WebRTC Peer-to-Peer estables con el servidor.

## Enfoque
- Implementar los adaptadores de señalización WebSocket (`internal/adapters/signaling/`) y WebRTC (`internal/adapters/webrtc/`).
- Desarrollar la lógica de negocio para la gestión de salas en `internal/domain/room/` (unión, abandono, control de estado).
- Seguir estrictamente TDD para cada caso de uso y verificar límites con golangci-lint.
