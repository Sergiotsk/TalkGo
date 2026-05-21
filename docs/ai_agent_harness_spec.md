# AI Agent Harness — Especificación y Guía

**Proyecto:** TalkGo  
**Documento:** Configuración del entorno de trabajo seguro con agentes de IA  
**Estado:** Especificación lista para implementación (Sprint 0)  
**Fecha:** 2026-05-21

---

## 1. ¿Qué es un AI Agent Harness y por qué lo necesitás?

### La Analogía

Pensá en un equipo de construcción. Vos sos el arquitecto y los agentes de IA (Cursor, Antigravity, Copilot) son los obreros. Son increíblemente rápidos, pueden trabajar en paralelo, y no se cansan. Pero:

- **No entienden el plano completo** — solo ven el pedazo que les mostrás
- **No tienen criterio arquitectónico** — si les pedís un muro, lo ponen donde les digas, aunque bloquee una puerta
- **No recuerdan contexto entre sesiones** — cada vez empiezan de cero (salvo que uses memoria persistente)
- **Pueden generar código que compila pero viola principios** — importar dependencias en el dominio, mezclar capas, hardcodear configs

El **harness** es el conjunto de barreras físicas, reglas automatizadas y documentación que evita que estos "obreros rápidos" produzcan daño. Es tu red de seguridad.

### Los 3 Niveles de Protección

```
Nivel 1: PREVENCIÓN (antes de que el agente escriba)
    → Cursor Rules, SDD Specs, Architecture docs
    → Le decís al agente QUÉ hacer y CÓMO

Nivel 2: DETECCIÓN (mientras el agente escribe)
    → Linting (depguard), Type checking (go vet)
    → El IDE detecta violaciones en tiempo real

Nivel 3: BLOQUEO (después de que el agente escriba)
    → CI/CD (GitHub Actions), Tests (go test)
    → Si algo pasó los niveles 1 y 2, el CI lo frena antes del merge
```

**Los tres niveles son necesarios.** Ninguno es suficiente por sí solo.

---

## 2. Componentes del Harness

### Mapa Completo

```
AI Agent Harness
├── 📋 Nivel 1: PREVENCIÓN (configuración del agente)
│   ├── .cursor/rules/talkgo.mdc        ← Reglas para Cursor AI
│   ├── .gemini/settings.json            ← Config para Antigravity/Gemini
│   ├── .atl/skill-registry.md           ← Registry de skills disponibles
│   ├── docs/adr/                        ← Architecture Decision Records
│   └── SDD (Spec-Driven Development)    ← Specs que guían al agente
│
├── 🔍 Nivel 2: DETECCIÓN (análisis estático)
│   ├── .golangci.yml                    ← Configuración de linters
│   │   ├── depguard                     ← Enforcement de boundaries
│   │   ├── govet                        ← Análisis estático Go
│   │   ├── staticcheck                  ← Bugs comunes
│   │   └── revive                       ← Style + patterns
│   └── go vet ./...                     ← Type checking nativo
│
├── 🚫 Nivel 3: BLOQUEO (CI/CD + tests)
│   ├── .github/workflows/ci.yaml        ← GitHub Actions
│   ├── Makefile                          ← Comandos estándar
│   ├── go test -race ./...              ← Tests con race detector
│   └── Branch protection rules           ← GitHub branch rules
│
└── 📚 Documentación como Guardrail
    ├── README.md                         ← Onboarding para humanos Y agentes
    ├── docs/adr/                         ← Decisiones arquitectónicas
    └── docs/conventions.md              ← Convenciones del proyecto
```

---

## 3. Nivel 1: PREVENCIÓN — Configuración del Agente

### 3.1 Cursor Rules (`.cursor/rules/talkgo.mdc`)

**¿Qué es?** Un archivo que Cursor carga automáticamente cuando trabajás en el proyecto. Le dice al agente las reglas del proyecto ANTES de que escriba una sola línea de código.

**¿Por qué `.mdc`?** Cursor usa el formato MDC (Markdown with Context) que permite especificar globs (qué archivos aplican las reglas) y metadata.

**¿Cómo funciona?** Cuando abrís un archivo `.go` en el proyecto, Cursor lee automáticamente las reglas que matchean el glob `**/*.go` y las inyecta en el contexto del agente. El agente las "lee" antes de generar código.

```markdown
---
description: TalkGo project conventions for AI agents
globs: ["**/*.go"]
---

## Architecture: Hexagonal (Ports & Adapters)

This project uses strict Hexagonal Architecture. The dependency rule is:

    adapters → ports ← domain
    (outer)   (boundary)  (inner)

### Dependency Rules (CRITICAL — violations fail CI)

- `internal/domain/` MUST NOT import anything from `internal/adapters/`
- `internal/domain/` MUST NOT import infrastructure packages (net/http, database/*, pion/*)
- `internal/ports/` defines INTERFACES ONLY — no implementations
- `internal/adapters/` implements interfaces from `internal/ports/`
- `internal/app/` orchestrates domain + ports — it's the "wiring" layer

### Why This Matters

If you (the AI agent) import `pion/webrtc` inside `internal/domain/room/`,
the domain becomes coupled to a specific WebRTC library. When we need to
replace pion or add a new transport (Zoom RTMS, Teams Bot), we'd have to
modify the domain — which should NEVER change for infrastructure reasons.

## Go Conventions

### Error Handling
- Always wrap errors with context: `fmt.Errorf("creating room: %w", err)`
- Never ignore errors silently: `_ = someFunc()` is forbidden
- Use sentinel errors for domain errors: `var ErrRoomFull = errors.New("room is full")`

### Constructor Pattern
- Every type with dependencies uses a constructor: `NewXxx(deps...) *Xxx`
- Constructors validate inputs and return errors when appropriate
- No global state, no init() functions, no package-level vars (except sentinels)

### Context
- All I/O operations take `context.Context` as first parameter
- Domain logic does NOT use context (it's a domain concept, not infra)

### Documentation
- All exported functions MUST have doc comments
- Doc comments start with the function name: `// NewRoom creates a new Room...`

## Testing

### Strict TDD Mode is ENABLED
- Write the test FIRST, then the implementation
- Every interface in `ports/` MUST have a mock (manual or generated)
- Use table-driven tests with `t.Run()` for multiple cases
- Test file location: colocated (`foo.go` → `foo_test.go`)

### Coverage Targets
- `internal/domain/`: 80% minimum
- `internal/adapters/`: 60% minimum
- `internal/app/`: 70% minimum

## What NOT to Do
- Do NOT add dependencies without documenting in `docs/adr/`
- Do NOT write business logic in adapters — adapters are thin wrappers
- Do NOT use `init()` functions
- Do NOT commit API keys or secrets (use environment variables)
- Do NOT create files outside the established directory structure
```

#### Concepto Clave: Context Window

> Los agentes de IA tienen una "ventana de contexto" limitada — la cantidad de información que pueden "recordar" mientras generan código. Las Cursor Rules se inyectan en esa ventana, ocupando espacio pero asegurando que las reglas están presentes. **Por eso deben ser concisas pero completas** — no novelas, pero tampoco tan breves que se pierdan matices.

---

### 3.2 Configuración Gemini/Antigravity (`.gemini/settings.json`)

**¿Qué es?** Configuración a nivel de proyecto para cuando trabajés con Antigravity (este agente). A diferencia de Cursor que trabaja inline, Antigravity puede ejecutar comandos, crear archivos, y gestionar workflows complejos.

```json
{
  "project": "talkgo",
  "language": "go",
  "architecture": "hexagonal",
  "strict_tdd": true,
  "conventions": {
    "commits": "conventional-commits",
    "branches": "feature/{issue-number}-{short-description}"
  }
}
```

---

### 3.3 Spec-Driven Development (SDD)

**¿Qué es?** Una metodología de desarrollo donde las **especificaciones escritas** son la fuente de verdad, no el código. El agente de IA lee las specs antes de escribir código, implementa según las specs, y luego se verifica contra las specs.

**¿Por qué es crucial para agentes de IA?**

Sin SDD:
```
Vos: "Haceme un servicio de salas"
Agente: *inventa una estructura, API y comportamiento basándose en training data*
Resultado: Código que funciona pero no se alinea con tu visión
```

Con SDD:
```
Vos: *Escribís spec con requisitos, scenarios, y design*
Agente: *lee la spec, implementa exactamente lo especificado*
Resultado: Código que cumple los requisitos documentados
Verificación: *se compara implementación contra spec*
```

**Flujo SDD:**

```
/sdd-explore → Investigar el problema, entender el codebase
    ↓
/sdd-propose → Crear propuesta de cambio (qué y por qué)
    ↓
/sdd-spec → Escribir especificación (requisitos + escenarios Given/When/Then)
    ↓
/sdd-design → Diseño técnico (cómo implementar)
    ↓
/sdd-tasks → Desglosar en tareas implementables
    ↓
/sdd-apply → Implementar (el agente escribe código siguiendo spec+design)
    ↓
/sdd-verify → Verificar (comparar implementación contra spec)
    ↓
/sdd-archive → Archivar el cambio completado
```

**Para TalkGo usamos `engram` como backend de persistencia** (almacena specs en memoria persistente, no en archivos). Esto es ideal para desarrollo solo. Cuando sumemos equipo, migramos a `openspec` (file-based, committable).

#### Inicialización SDD para TalkGo

El comando `/sdd-init` detecta automáticamente:
- Stack: Go 1.22+
- Test runner: `go test` (built-in)
- Linter: `golangci-lint`
- Formatter: `gofmt` + `goimports`
- Strict TDD: enabled (configurado en user rules)

Y persiste esta información en engram para que todas las fases SDD la conozcan.

---

### 3.4 Architecture Decision Records (ADRs)

**¿Qué son?** Documentos cortos que registran decisiones arquitectónicas importantes con su contexto y consecuencias. Son cruciales para agentes de IA porque:

1. **Previenen que el agente revierta una decisión ya tomada** — si hay un ADR que dice "usamos OpenAI Realtime en vez del pipeline de 3 pasos", el agente no va a proponer lo contrario
2. **Dan contexto para decisiones futuras** — el agente puede leer ADRs previos para entender por qué se hicieron ciertas elecciones

**Template:**

```markdown
# ADR-NNNN: [Título de la Decisión]

## Estado
Proposed | Accepted | Deprecated | Superseded by ADR-XXXX

## Contexto
¿Cuál es el problema o la situación que requiere una decisión?

## Decisión
¿Qué decidimos hacer y por qué?

## Consecuencias
### Positivas
- ...
### Negativas
- ...
### Riesgos
- ...
```

**ADRs planificados para Sprint 0:**

| ADR | Decisión |
|-----|----------|
| ADR-0001 | Usar Arquitectura Hexagonal (Ports & Adapters) |
| ADR-0002 | OpenAI Realtime Translate vs Pipeline de 3 pasos (Proposed — pendiente validación) |
| ADR-0003 | Audio mono por dispositivo (no estéreo) |
| ADR-0004 | Engram como persistence backend para SDD (migrar a openspec con equipo) |

---

## 4. Nivel 2: DETECCIÓN — Análisis Estático

### 4.1 golangci-lint (`.golangci.yml`)

**¿Qué es?** Un meta-linter que ejecuta múltiples herramientas de análisis estático en Go de forma unificada. Es la primera línea de defensa automática contra código que viola reglas.

**¿Por qué importa para agentes de IA?** Los agentes frecuentemente:
- Importan paquetes que no deberían (violando boundaries)
- Generan código que compila pero tiene bugs sutiles
- No siguen convenciones de formato
- Ignoran errores con `_ =`

El linter los atrapa ANTES de que el código llegue a un commit.

```yaml
# .golangci.yml

run:
  timeout: 5m
  modules-download-mode: readonly

linters:
  enable:
    # Correctness — bugs reales
    - errcheck          # Detecta errores no chequeados
    - govet             # Análisis estático oficial de Go
    - staticcheck       # Suite de análisis avanzado (SA*, S*, ST*, QF*)
    - typecheck         # Errores de tipo
    - ineffassign       # Asignaciones que no se usan
    - unused            # Código muerto

    # Style — consistencia
    - gofmt             # Formato estándar Go
    - goimports         # Imports ordenados + formato
    - revive            # Linter extensible de estilo (reemplaza golint)
    - misspell          # Errores de ortografía en comments/strings

    # Architecture — boundaries
    - depguard          # CLAVE: restringe imports por paquete
    - gocritic          # Sugerencias de mejora avanzadas

linters-settings:
  # ============================================================
  # DEPGUARD — El guardian de la arquitectura hexagonal
  # ============================================================
  # Esta es la pieza MÁS IMPORTANTE del harness.
  #
  # ¿Qué hace? Impide que ciertos paquetes importen a otros.
  # ¿Por qué importa? Si un agente de IA importa `pion/webrtc`
  # dentro de `internal/domain/`, depguard falla el build.
  #
  # Esto es enforcement AUTOMÁTICO de la regla de dependencias
  # de la arquitectura hexagonal: el dominio NO conoce la infra.
  # ============================================================
  depguard:
    rules:
      # El dominio NO puede importar infraestructura
      domain-isolation:
        list-mode: denylist
        files:
          - "**/internal/domain/**"
        deny:
          - pkg: "github.com/pion/*"
            desc: "Domain MUST NOT import WebRTC infrastructure"
          - pkg: "net/http"
            desc: "Domain MUST NOT import HTTP — use ports for I/O"
          - pkg: "database/*"
            desc: "Domain MUST NOT import database packages"
          - pkg: "github.com/gorilla/*"
            desc: "Domain MUST NOT import HTTP libraries"
          - pkg: "github.com/sashabaranov/go-openai"
            desc: "Domain MUST NOT import OpenAI SDK — use Translator port"
          - pkg: "encoding/json"
            desc: "Domain SHOULD NOT handle serialization — that's adapter concern"

      # Los ports NO pueden importar adapters
      ports-isolation:
        list-mode: denylist
        files:
          - "**/internal/ports/**"
        deny:
          - pkg: "**/internal/adapters/**"
            desc: "Ports define interfaces — they MUST NOT import implementations"

      # Nadie importa paquetes de test en código de producción
      no-test-in-prod:
        list-mode: denylist
        files:
          - "!**/*_test.go"
        deny:
          - pkg: "testing"
            desc: "Package testing should only be imported in test files"

  revive:
    rules:
      - name: exported
        severity: warning
        arguments:
          - "checkPrivateReceivers"
      - name: unexported-return
        severity: warning
      - name: error-return
        severity: warning
      - name: error-naming
        severity: warning

  gocritic:
    enabled-tags:
      - diagnostic
      - style
      - performance

issues:
  # No ignorar ningún issue — queremos ver TODO
  max-issues-per-linter: 0
  max-same-issues: 0

  exclude-rules:
    # Permitir dot imports en tests (para testify, etc.)
    - path: _test\.go
      linters:
        - revive
      text: "dot-imports"
```

#### Concepto Clave: depguard como Guardian Arquitectónico

```
Sin depguard:
    Agente escribe: import "github.com/pion/webrtc/v4"
    En archivo: internal/domain/room/room.go
    Resultado: Compila ✅, pero el dominio ahora depende de infra ❌
    Consecuencia: Cuando quieras integrar con Zoom, tenés que tocar el dominio

Con depguard:
    Agente escribe: import "github.com/pion/webrtc/v4"
    En archivo: internal/domain/room/room.go
    Resultado: golangci-lint FALLA ❌
    Mensaje: "Domain MUST NOT import WebRTC infrastructure"
    Consecuencia: El agente (o vos) reciben feedback inmediato de que eso viola la arquitectura
```

**Es como poner una reja en una obra. No importa qué tan apurado esté el obrero — la reja no lo deja pasar.**

---

### 4.2 go vet

**¿Qué es?** La herramienta de análisis estático oficial de Go. Detecta errores que compilan pero son bugs:

```go
// Esto compila pero go vet lo detecta como bug:
fmt.Printf("%d", "hello")  // tipo incorrecto para %d
```

Se ejecuta automáticamente como parte de `golangci-lint`.

---

## 5. Nivel 3: BLOQUEO — CI/CD y Tests

### 5.1 GitHub Actions (`.github/workflows/ci.yaml`)

**¿Qué es?** Un workflow de CI que corre automáticamente en cada push y pull request. Es la última línea de defensa — si el código llega acá con problemas, el CI lo frena.

```yaml
name: CI

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  check:
    name: Build, Lint & Test
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Download dependencies
        run: go mod download

      - name: Build
        run: go build ./...

      - name: Lint (architecture boundaries + style)
        uses: golangci/golangci-lint-action@v7
        with:
          version: latest
          args: --timeout=5m

      - name: Test (with race detector)
        run: go test -race -cover -coverprofile=coverage.out ./...

      - name: Check coverage threshold
        run: |
          COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%')
          echo "Total coverage: ${COVERAGE}%"
          # Fail if coverage drops below 50% (MVP threshold, increase over time)
          if (( $(echo "$COVERAGE < 50" | bc -l) )); then
            echo "❌ Coverage ${COVERAGE}% is below threshold of 50%"
            exit 1
          fi
```

#### El Flujo Completo de un Cambio

```
1. Vos (o un agente) escribe código
    ↓
2. IDE (Cursor) muestra errores de lint en tiempo real [Nivel 2]
    ↓
3. git add + git commit
    ↓
4. git push → GitHub Actions se activa [Nivel 3]
    ↓
5. CI ejecuta: build → lint → test → coverage
    ↓
6. Si CUALQUIER paso falla → ❌ PR no se puede mergear
    ↓
7. Si todo pasa → ✅ PR listo para review
```

---

### 5.2 Makefile

**¿Qué es?** Comandos estándar que unifican cómo se ejecutan las tareas del proyecto. Importantes para agentes porque les das UN comando consistente en vez de que inventen el suyo.

```makefile
.PHONY: build test lint fmt check setup clean

# ============================================
# Core commands (los que usa CI)
# ============================================

build:                          ## Compila todo el proyecto
	go build ./...

test:                           ## Corre tests con race detector y coverage
	go test -race -cover ./...

test-verbose:                   ## Tests con output detallado
	go test -race -v -cover ./...

lint:                           ## Corre todos los linters (incluye depguard)
	golangci-lint run ./...

fmt:                            ## Formatea todo el código
	gofmt -w .
	goimports -w .

check: fmt lint test            ## Corre TODO — es lo que ejecuta CI

# ============================================
# Development helpers
# ============================================

setup:                          ## Primera vez: instala herramientas
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	@echo "✅ Setup complete. Run 'make check' to verify."

clean:                          ## Limpia artefactos de build
	go clean -testcache
	rm -f coverage.out

cover:                          ## Genera reporte de coverage en HTML
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "📊 Coverage report: coverage.html"

# ============================================
# SDD helpers (Spec-Driven Development)
# ============================================

verify: lint test               ## Verificación rápida (lint + test)
	@echo "✅ Verification passed"

help:                           ## Muestra esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
```

#### Concepto Clave: Makefile como Interfaz para Agentes

Cuando un agente de IA necesita verificar que su código es correcto, le decís:

```
"Corré `make check`"
```

No `go test -race -cover ./... && golangci-lint run ./... && gofmt -l .`. El Makefile abstrae la complejidad y asegura que el agente ejecuta TODOS los checks, no solo algunos.

---

### 5.3 Testing Infrastructure

#### Strict TDD con Agentes de IA

**¿Cómo funciona TDD con un agente?**

```
1. VOS escribís la spec (o le pedís al agente que la escriba con /sdd-spec)
    ↓
2. Le pedís al agente: "Implementá la tarea 1.1 del change X"
    ↓
3. El agente (en Strict TDD mode) PRIMERO escribe el test:

    func TestNewRoom_ValidLanguages(t *testing.T) {
        room, err := NewRoom("es", "en")
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if room.SourceLang != "es" {
            t.Errorf("got %q, want %q", room.SourceLang, "es")
        }
    }

4. Verifica que el test FALLA (Red):
    $ go test ./internal/domain/room/
    --- FAIL: TestNewRoom_ValidLanguages

5. Implementa el mínimo código para que pase (Green):

    func NewRoom(source, target string) (*Room, error) {
        return &Room{SourceLang: source, TargetLang: target}, nil
    }

6. Verifica que el test PASA:
    $ go test ./internal/domain/room/
    --- PASS: TestNewRoom_ValidLanguages

7. Refactoriza si necesario (Refactor)
    ↓
8. Siguiente test → repite el ciclo
```

#### Mocks para Ports

```go
// internal/ports/driven/translator.go — La interface REAL
type Translator interface {
    TranslateStream(ctx context.Context, sourceLang, targetLang string,
        audioIn <-chan []byte) (<-chan []byte, error)
}

// internal/mocks/translator_mock.go — El mock para tests
type MockTranslator struct {
    TranslateStreamFunc func(ctx context.Context, sourceLang, targetLang string,
        audioIn <-chan []byte) (<-chan []byte, error)
    Calls []TranslateStreamCall
}

type TranslateStreamCall struct {
    SourceLang string
    TargetLang string
}

func (m *MockTranslator) TranslateStream(ctx context.Context, 
    sourceLang, targetLang string,
    audioIn <-chan []byte) (<-chan []byte, error) {
    m.Calls = append(m.Calls, TranslateStreamCall{sourceLang, targetLang})
    if m.TranslateStreamFunc != nil {
        return m.TranslateStreamFunc(ctx, sourceLang, targetLang, audioIn)
    }
    out := make(chan []byte)
    close(out)
    return out, nil
}
```

**¿Por qué mocks manuales y no generados?** Para el MVP, mocks manuales son más legibles y dan más control. Cuando escale, se puede usar `mockgen` o `moq`.

---

## 6. Skill Registry (`.atl/skill-registry.md`)

**¿Qué es?** Un índice de skills (habilidades especializadas) que los agentes de IA pueden cargar según el contexto. Cuando un agente trabaja en un archivo Go de testing, automáticamente carga las convenciones de testing del proyecto.

**¿Para qué sirve?** Evita que el agente use patrones genéricos cuando el proyecto tiene convenciones específicas. Por ejemplo, si TalkGo usa table-driven tests con un patrón específico, el skill registry le dice al agente "usá este patrón, no el que vos conocés".

Este archivo se genera automáticamente con `/sdd-init`. Se actualiza cuando se agregan skills nuevos al proyecto.

---

## 7. `.gitignore` — Qué NO commitear

```gitignore
# =====================
# Go artifacts
# =====================
/bin/
/vendor/
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
go.work
go.work.sum
coverage.out
coverage.html

# =====================
# IDE
# =====================
.idea/
.vscode/
*.swp
*.swo
*~

# =====================
# OS
# =====================
.DS_Store
Thumbs.db
Desktop.ini

# =====================
# Secrets (NUNCA commitear)
# =====================
.env
.env.local
.env.*.local
*.pem
*.key

# =====================
# AI Agent artifacts
# Estos son archivos de trabajo interno
# de los agentes, NO van al repo
# =====================
.gemini/antigravity/

# =====================
# Build/Deploy
# =====================
dist/
tmp/
```

---

## 8. Checklist de Implementación — Sprint 0

### Orden de Ejecución

Cada item tiene su justificación de por qué va en ese orden:

```
[ ] 1. go mod init                    ← Sin esto nada compila
[ ] 2. Estructura de directorios      ← El esqueleto antes de la carne
[ ] 3. Domain types (structs)         ← Lo más puro, sin deps
[ ] 4. Port interfaces                ← Contratos antes de implementaciones
[ ] 5. Domain tests                   ← TDD: test antes del código (pero los types ya existen)
[ ] 6. .golangci.yml                  ← Activar guardians ANTES de escribir más código
[ ] 7. Makefile                       ← Unificar comandos
[ ] 8. make check                     ← Verificar que todo pasa ANTES de seguir
[ ] 9. .cursor/rules/talkgo.mdc       ← Ahora Cursor sabe las reglas para código futuro
[ ] 10. .github/workflows/ci.yaml     ← CI listo para el primer push
[ ] 11. .gitignore                    ← Antes del primer commit
[ ] 12. ADRs iniciales                ← Documentar decisiones ya tomadas
[ ] 13. README.md                     ← Onboarding
[ ] 14. /sdd-init                     ← Inicializar SDD con engram
[ ] 15. git init + push               ← Primer commit al repo
[ ] 16. make check (en CI)            ← Verificar que CI pasa en GitHub
```

---

## 9. Principios del Harness

### 9.1 — Fail Fast, Fail Loud

Si algo viola una regla, debe fallar **inmediato** y con un **mensaje claro**. No warnings silenciosos. No "suggestion". Error. Stop. Fix.

### 9.2 — Zero Trust en el Agente

No asumás que el agente "sabe" las reglas porque se las dijiste una vez. Las reglas deben estar:
- En el context del agente (Cursor Rules) → para que las lea
- En el linter (depguard) → para que se enforzen aunque no las lea
- En CI (GitHub Actions) → para que se bloquee aunque el linter no corra localmente

**Redundancia intencional.**

### 9.3 — Especificación > Instrucción

"Implementá un servicio de salas" es una instrucción.
"Implementá la tarea 1.1 del change room-management según la spec" es una referencia a una especificación.

La instrucción deja margen de interpretación. La spec no.

### 9.4 — Mocks en los Boundaries

Siempre mockeá en los ports, nunca en las dependencias internas del dominio. Esto asegura que los tests del dominio son verdaderamente independientes de la infraestructura.

### 9.5 — El Humano Siempre Lidera

El agente propone, vos decidís. El agente implementa, vos verificás. El agente nunca debería:
- Elegir una dependencia nueva sin tu OK
- Cambiar la arquitectura sin un ADR
- Mergear a main sin CI verde

---

## 10. Anti-Patterns — Qué Evitar

| Anti-Pattern | Por qué es malo | Qué hacer en cambio |
|-------------|-----------------|---------------------|
| **"Haceme toda la feature"** | El agente inventa estructura, API, tests — todo sin spec | Usar SDD: spec → design → tasks → apply |
| **No correr lint antes de commit** | El código llega a CI con errores que podías atrapar local | `make check` antes de CADA commit |
| **Ignorar warnings del linter** | "Es solo un warning" → se acumulan → se normalizan → deuda | Zero warnings policy. Si es un false positive, suprimir explícitamente con comment |
| **Dejar que el agente elija dependencias** | El agente agrega `go-kit`, `wire`, `fx` sin necesidad | Toda dependencia nueva requiere ADR o aprobación explícita |
| **No revisar el código del agente** | "Si el test pasa, está bien" — no, puede pasar por motivos incorrectos | Code review del output del agente, especialmente la primera vez que toca una área |
| **Commitear archivos de agente** | `.gemini/antigravity/` en el repo contamina y confunde | `.gitignore` bien configurado |
| **Pedir cambios sin contexto** | "Arreglá el bug" sin decir cuál, dónde, ni cómo reproducir | Siempre dar: archivo, línea, comportamiento esperado vs actual |

---

## 11. Evolución del Harness

### Fase MVP (Sprint 0 — Ahora)
- Cursor Rules básicas
- golangci-lint con depguard
- GitHub Actions CI
- Makefile
- SDD con engram
- ADRs manuales

### Fase Equipo (Cuando sumen devs)
- Migrar SDD a openspec (file-based, committable)
- Pre-commit hooks (husky equivalent for Go)
- PR templates con checklist
- CODEOWNERS para review automático
- Coverage badges en README
- Branch protection: require 1 review + CI green

### Fase Enterprise
- Signed commits obligatorios
- SAST (Static Application Security Testing)
- Dependency scanning (govulncheck)
- License compliance checking
- Audit logging de cambios

---

*Este documento es la referencia principal para entender y mantener el AI agent harness de TalkGo. Se actualiza cuando se agregan o modifican componentes del harness.*
