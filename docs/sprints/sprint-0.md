# Sprint 0: Scaffolding, Arquitectura y AI Harness

## Objetivo
Configurar el entorno de desarrollo seguro para trabajar con agentes de IA mediante Spec-Driven Development, estructurar el proyecto base usando Arquitectura Hexagonal y preparar el repositorio en GitHub.

## Criterios de Aceptación
1. El proyecto en Go compila correctamente (`make build` exitoso).
2. Los tests unitarios del dominio corren y pasan (`make test` exitoso).
3. `golangci-lint` linta todo el código sin warnings ni errores (`make lint` exitoso).
4. El arnés de IA (`.cursor/rules/talkgo.mdc` y `depguard`) bloquea código que rompa las reglas arquitectónicas.
5. El flujo de CI de GitHub Actions valida el build, lint y tests en cada push.
6. La documentación inicial del proyecto (ADRs, README, especificaciones) está completa y cargada en el repositorio.

## Entregables
- Módulo Go inicializado y configurado.
- Estructura de carpetas Hexagonal limpia con tipos del dominio básicos (`Room`, `Session`, `Chunk`) e interfaces de puertos vacías.
- Arnés de linter, Makefile, gitignore y GitHub workflow configurados.
- Repositorio privado en GitHub listo con la primera versión de la rama `main`.
