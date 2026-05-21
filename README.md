# TalkGo — Traducción Simultánea por Dispositivo

TalkGo es una plataforma de traducción simultánea por voz en tiempo real pensada para conversaciones cara a cara multidisciplinarias e internacionales utilizando dispositivos móviles individuales.

## Arquitectura

El backend del proyecto está estructurado con **Arquitectura Hexagonal (Ports & Adapters)** estricta en Go. Esto permite que el dominio y las reglas de negocio permanezcan completamente aislados de la infraestructura de WebRTC (Pion) y de los servicios de traducción externa (OpenAI Realtime).

### Mapa de Dependencias

```
adapters (Infraestructura) → ports (Límites/Interfaces) ← domain (Core Puro)
```

- **Domain (`internal/domain/`)**: Contiene tipos puros y lógica de negocio. Cero importaciones externas de infraestructura.
- **Ports (`internal/ports/`)**: Define las interfaces de entrada (driving) y salida (driven).
- **Adapters (`internal/adapters/`)**: Implementa la infraestructura real (WebRTC, OpenAI APIs, Audio Mixing).
- **App (`internal/app/`)**: Orquestación y cableado de casos de uso.

---

## AI Agent Harness (Seguridad en el Desarrollo con IA)

Para garantizar un desarrollo limpio, consistente y seguro, el proyecto implementa un **Harness para Agentes de IA** con 3 niveles de protección:

1. **Prevención (Cursor Rules)**: `.cursor/rules/talkgo.mdc` previene errores de diseño antes de escribir código.
2. **Detección (Linter Estricto)**: `.golangci.yml` incluye `depguard` que bloquea importaciones que rompan la arquitectura (ej. importar pion/webrtc en el dominio).
3. **Bloqueo (CI/CD)**: GitHub Actions corre `make check` antes de permitir fusiones a `main`.

---

## Comandos Rápidos

El proyecto utiliza un `Makefile` estándar para unificar las tareas de desarrollo:

*   `make setup`: Instala linters e instrumental inicial.
*   `make build`: Compila todo el proyecto backend.
*   `make test`: Ejecuta los tests unitarios con el race detector habilitado.
*   `make lint`: Ejecuta el linter estricto de Go (`golangci-lint`).
*   `make check`: Formatea, linta y corre tests (ejecutado automáticamente en CI).

---

## Cómo Contribuir (Gente y Agentes)

Tanto si eres humano como un agente de IA, debes seguir estas reglas a rajatabla:
1. **Strict TDD**: Escribir los tests antes de la implementación.
2. **Hexagonal isolation**: Nunca cruzar las fronteras de los paquetes. Si necesitas algo de afuera, define un puerto (Port) primero.
3. **Zero warnings**: No se permiten warnings de compilación o de linter acumulados.

---

## Estructura del Proyecto (Monorepo)

*   `cmd/server/`: Punto de entrada de la aplicación servidor backend.
*   `internal/`: Código privado del backend en Go (domain, ports, adapters, app).
*   `mobile/`: Reservado para la aplicación cliente React Native en sprints futuros.
*   `docs/`: Documentación del proyecto (ADRs, PRD, especificaciones de sprints).
