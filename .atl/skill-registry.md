# TalkGo AI Skill Registry

Este archivo registra las habilidades y convenciones automatizadas del proyecto TalkGo para guiar a los agentes de IA.

## Project Standards

### Go Code Rules
- Usar Arquitectura Hexagonal. El dominio (`internal/domain/`) NUNCA debe importar nada de `internal/adapters/` o infraestructura externa (WebRTC, OpenAI).
- Usar table-driven tests con `t.Run()`.
- Documentar todas las funciones exportadas en inglés empezando con su nombre.
- Manejo de errores con wrapping: `fmt.Errorf("context: %w", err)`.

### Strict TDD Mode
- Modo TDD Estricto habilitado. Escribir primero el test unitario en el mismo directorio (ej. `foo_test.go`), verificar su falla, luego implementar la lógica mínima para pasarlo, y finalmente refactorizar.

---

## Active Skills

| Skill | Triggers | Description |
|---|---|---|
| `go-testing` | `**/*_test.go` | Go testing patterns, table-driven tests, mocks. |
| `sdd-apply` | `sdd-apply` / change tasks | Executor for implementing tasks following SDD specs. |
