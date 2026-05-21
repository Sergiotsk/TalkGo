# TalkGo — Visión Enterprise Integration

**Documento:** Estrategia de integración con plataformas de videoconferencia empresarial  
**Estado:** Investigación completada — Ejecución futura (Fase 5 del roadmap)  
**Fecha:** 2026-05-21  
**Objetivo:** Documentar los caminos técnicos para embeber el motor de traducción de TalkGo en plataformas como Zoom, Teams y Google Meet, ofreciéndolo como servicio a empresas.

---

## 1. La Oportunidad

### El Problema Empresarial

Las empresas globales gastan millones en interpretación simultánea para reuniones internacionales:

- **Intérpretes humanos profesionales:** $50–150 USD/hora por par de idiomas
- **Agencias de interpretación:** contratos anuales de $100K+ para empresas medianas
- **Limitación de idiomas:** un intérprete cubre 1 par de idiomas, reuniones multilingües requieren múltiples intérpretes
- **Disponibilidad:** necesitan reservarse con anticipación, no están disponibles on-demand
- **Fatiga del intérprete:** rotación cada 30 minutos para mantener calidad

### El Gap de Mercado

| Solución | Problema |
|----------|----------|
| Interpretación humana | Cara, requiere planificación, no escala |
| Zoom Translated Captions | Solo subtítulos, no audio traducido en tiempo real |
| Teams Copilot Translation | Atado al ecosistema Microsoft, calidad variable |
| Google Meet Translation | Limitado a subtítulos, pocas opciones de idioma |

**TalkGo puede llenar este gap:** traducción de audio en tiempo real, contextual (LLM), integrada directamente en la plataforma de video que la empresa ya usa.

### Timing

> **Microsoft está retirando su feature "Converse" (multi-device translation) el 30 de junio de 2026.** Esto crea un gap directo en el mercado de traducción para consumidores Y señala que el espacio enterprise está en transición.

---

## 2. Productos que Ya Hacen Esto (Competidores Enterprise)

### KUDO

| Aspecto | Detalle |
|---------|---------|
| **Modelo** | Cloud SaaS sobre AWS (multi-tenant) |
| **Teams** | v2.0 — "KUDO Translator" se une como participante al meeting |
| **Zoom** | Browser-based; pega el link de Zoom en la plataforma KUDO → genera link de acceso. También tiene app en Zoom Marketplace |
| **Idiomas** | IA: 60+ idiomas. Intérpretes humanos: 200+ idiomas incluyendo lengua de señas |
| **Compliance** | ISO 27001:2022, SOC 2 Type 2, WAF, DDoS mitigation |
| **Ancho de banda** | Optimizado para 10-20 Kb/sec |
| **Diferenciador** | Modelo híbrido: IA + intérpretes humanos en la misma sesión |

### Interprefy

| Aspecto | Detalle |
|---------|---------|
| **Modelo** | "Inject" method para Zoom/Teams |
| **Cómo funciona** | Captura audio del meeting → cloud de Interprefy → intérpretes/IA procesan → inyecta audio traducido de vuelta en los canales de interpretación nativos de la plataforma |
| **UX** | Los participantes usan el selector de idioma nativo de la plataforma (ícono de globo en Zoom, prompt de interpretación en Teams) |
| **Alternativa** | "Interprefy Agent" — se envía una invitación por email, se une automáticamente, postea link/QR de traducción |
| **Web** | Widget iframe con API `postMessage` |

### Wordly.ai

| Aspecto | Detalle |
|---------|---------|
| **Modelo** | Cloud SaaS — browser-based |
| **Zoom** | Via Zoom Marketplace app + feature de live streaming para acceso a audio |
| **Teams** | Vista side-by-side en browser (meeting + interfaz Wordly) |
| **Output** | Subtítulos o audio sintetizado en el idioma elegido |
| **Compliance** | SOC 2 Type II, ISO 27001 |
| **Features** | Glosarios custom, identificación de hablante |

### Recall.ai (Capa de Abstracción)

| Aspecto | Detalle |
|---------|---------|
| **Qué es** | API unificada para bots de meeting en TODAS las plataformas |
| **Plataformas** | Zoom, Teams, Google Meet, Webex, Slack Huddles |
| **Cómo funciona** | Provisiona participantes virtuales headless ("bots") que se unen a meetings |
| **Output** | Audio raw, video (PNG/H.264), transcripciones en tiempo real vía WebSocket/Webhooks |
| **Bidireccional** | Puede reproducir audio, mostrar video, enviar chat de vuelta al meeting |
| **Compliance** | SOC 2, GDPR |
| **Valor para TalkGo** | Elimina la necesidad de mantener infraestructura de bots separada por plataforma |

### Análisis: Qué Hace Diferente a TalkGo

| Diferenciador | KUDO/Interprefy/Wordly | TalkGo |
|--------------|------------------------|--------|
| **Motor de traducción** | ASR + MT tradicional (o intérpretes humanos) | LLM contextual (GPT-4o) — traducciones idiomáticas, no literales |
| **Latencia** | 2-5s típico | <1s target (OpenAI Realtime: 300-800ms) |
| **Modelo de negocio** | Licencia enterprise + intérpretes humanos | Software puro — escala sin costos humanos |
| **Stack** | Cerrado, propietario | Arquitectura hexagonal extensible |
| **Standalone app** | No — solo funcionan dentro de plataformas | Sí — app móvil independiente + integración enterprise |

---

## 3. Caminos Técnicos de Integración por Plataforma

### 3.1 Zoom — RTMS (Realtime Media Streams) ⭐ Recomendado como primera integración

| Aspecto | Detalle |
|---------|---------|
| **API** | Zoom RTMS (GA desde julio 2025) |
| **Protocolo** | WebSocket seguro (canales de señalización + media) |
| **Acceso a audio raw** | ✅ Sí — streams per-participant disponibles |
| **Modelo de deployment** | Cloud-native — el backend de TalkGo recibe streams WebSocket |
| **Scopes requeridos** | `meeting:rtms:read`, `meeting:read:meeting_audio` |
| **SDKs oficiales** | Node.js y Python |
| **Trigger** | Auto-start setting, REST API, o `startRTMS()` vía Zoom Apps JS SDK |

#### Flujo de Integración

```
1. Configurar webhook subscriptions para "RTMS started" / "RTMS stopped"
2. Cuando se activa, el backend establece WebSocket de señalización (auth/negociación)
3. Zoom provee endpoint de WebSocket de media para streams de audio/video/transcripción
4. TalkGo recibe audio raw → pipeline de traducción → audio traducido
5. Audio traducido se inyecta de vuelta al meeting
```

#### Ventajas

- **No requiere bot infrastructure** — los streams llegan directamente al servidor
- **Cloud-native** — no necesita instancias de cliente Zoom corriendo en servers
- **Mejor DX** — SDKs oficiales, documentación clara

#### Requisitos para Marketplace

- Registrar "General App" en Zoom Marketplace
- Habilitar RTMS en la cuenta
- Zoom Marketplace listing requerido para distribución

#### Documentación

- https://developers.zoom.us/docs/rtms
- https://github.com/zoom (SDKs y samples oficiales)

---

### 3.2 Microsoft Teams — Real-Time Media Bot

| Aspecto | Detalle |
|---------|---------|
| **SDK** | `Microsoft.Graph.Communications.Calls.Media` (NuGet) |
| **Lenguaje** | C# / .NET **únicamente** — no hay soporte para Node.js, Python, Go u otros |
| **Acceso a audio raw** | ✅ Sí — 50 frames/segundo, cada frame = 20ms de audio PCM |
| **Formato de audio** | PCM 16KHz, 16-bit, 320 samples por frame de 20ms (640 bytes/frame). Codecs: SILK, G.722 |
| **Latencia** | Frame-level (granularidad de 20ms) |
| **Modelo de deployment** | Bot en la nube sobre **Windows Server** (Azure VM fuertemente recomendado) |
| **Permisos** | Azure AD app con permiso Graph `Calls.AccessMedia.All` |
| **Infraestructura** | Windows Server, rangos de puertos específicos abiertos, networking de alto ancho de banda |

#### Flujo de Integración

```
1. Bot se une al meeting de Teams vía Microsoft Graph Communications API
2. Establece sesión de media declarando modalidades soportadas (recibir audio)
3. Real-Time Media Platform entrega frames de audio raw vía API socket-like
4. TalkGo procesa frames en tiempo real (traducción)
5. Audio traducido se envía de vuelta al meeting
```

#### Certificación y Licenciamiento

| Requisito | Detalle |
|-----------|---------|
| **Publisher Verification** | Obligatorio — requiere membresía en Microsoft AI Cloud Partner Program (MAICPP) + Partner One ID |
| **Publisher Attestation** | Auto-evaluación opcional de seguridad/manejo de datos |
| **Microsoft 365 Certification** | Auditoría completa + pen test por Microsoft (recomendado para enterprise) |
| **Store Validation** | Cientos de test cases para funcionalidad, seguridad, políticas de store |
| **Herramienta** | App Compliance Automation Tool (ACAT) disponible en Azure portal |

#### Implicación Arquitectónica para TalkGo

> **⚠️ El .NET requirement es un problem.** TalkGo está en Go. Opciones:
>
> 1. **Companion service en .NET** que maneja la integración con Teams y se comunica con el pipeline de TalkGo via gRPC → recomendado
> 2. **Reescribir el adapter en .NET** duplicando lógica → no recomendado
> 3. **Usar Recall.ai** como abstracción para evitar el .NET tax → evaluar costo/beneficio

#### Documentación

- https://learn.microsoft.com/en-us/microsoftteams/platform/bots/calls-and-meetings/real-time-media-concepts
- https://learn.microsoft.com/en-us/graph/cloud-communications-concept-overview

#### Advertencia de Microsoft

Microsoft explícitamente dice que los Real-Time Media bots **NO son recomendados para escenarios generales de agentes IA**. Recomiendan Copilot Studio agents o transcripciones post-meeting vía Graph API para la mayoría de los casos de uso. Esto puede cambiar, pero es un riesgo a monitorear.

---

### 3.3 Google Meet — Media API (Developer Preview)

| Aspecto | Detalle |
|---------|---------|
| **API** | Google Meet Media API |
| **Estado** | ⚠️ **Developer Preview** — NO está en GA |
| **Acceso a audio raw** | ✅ Sí — audio/video en tiempo real vía WebRTC |
| **OAuth Scopes** | Restringidos: `meetings.conference.media.readonly` |
| **Codecs requeridos** | AV1, VP8, VP9 (via libvpx, dav1d) |
| **Red** | Mínimo 4 Mbps de ancho de banda, reportes periódicos de métricas |
| **Modelo de deployment** | Cliente WebRTC server-side |

#### Restricciones Críticas

- **TODOS** los participantes deben estar registrados en el Workspace Developer Preview Program
- Assessment de seguridad de terceros requerido anualmente
- Verificación OAuth rigurosa — toma semanas+
- El host puede revocar acceso; encryption/watermarking corta el stream

#### Alternativa: Chrome Extension (`chrome.tabCapture`)

| Aspecto | Detalle |
|---------|---------|
| **API** | `chrome.tabCapture` (Manifest V3) |
| **Acceso a audio** | ✅ Sí — `MediaStream` del tab activo |
| **User gesture** | Requerido — el usuario debe clickear para iniciar captura |
| **Procesamiento** | Ruta: `AudioContext` → `AudioWorklet` → WebSocket al backend |
| **Limitación** | Solo browser, captura TODO el tab (no per-participant) |

#### Recomendación para TalkGo

Esperar a que Meet Media API salga de Developer Preview. Mientras tanto, la Chrome Extension es un stopgap viable pero limitado. **Google Meet NO debería ser la primera plataforma enterprise** de TalkGo.

---

### 3.4 Desktop Universal — Virtual Audio Device (Modelo Krisp.ai)

| Aspecto | Detalle |
|---------|---------|
| **Cómo funciona** | Instala dispositivos virtuales de "Micrófono" y "Speaker" a nivel de OS |
| **Intercepción** | Las apps seleccionan el dispositivo virtual; el audio real es interceptado, procesado y reenviado |
| **Procesamiento** | Todo local, on-device (sin round-trip a servidor para la intercepción en sí) |
| **Compatibilidad** | Funciona con CUALQUIER app que permita seleccionar dispositivos de audio |
| **Complejidad** | **MUY ALTA** — requiere driver de audio en kernel-mode (WDM en Windows) |
| **Driver signing** | Certificado EV Code Signing + tests HLK para Windows moderno |
| **Sample code** | Microsoft SYSVAD: `github.com/microsoft/Windows-driver-samples/tree/main/audio/sysvad` |

#### Alternativa Más Simple: WASAPI Loopback (Windows)

| Aspecto | Detalle |
|---------|---------|
| **API** | Windows Core Audio / WASAPI |
| **Flag** | `AUDCLNT_STREAMFLAGS_LOOPBACK` |
| **Acceso** | Captura TODO el audio reproduciéndose en un endpoint de rendering |
| **Complejidad** | Moderada (user-mode, sin driver) |
| **Limitación** | Captura audio del sistema mezclado (no per-app); solo shared mode |

#### Evaluación para TalkGo

El Virtual Audio Device es la **solución más elegante y agnóstica a la plataforma** — funciona con Zoom, Teams, Meet, Discord, WhatsApp, literalmente cualquier cosa. Pero el costo de desarrollo es altísimo (kernel drivers, firma de drivers, testing en múltiples versiones de Windows/macOS). **Recomendado solo si TalkGo alcanza tracción enterprise significativa que justifique la inversión.**

---

## 4. Matriz Comparativa de Approaches

| Approach | Audio Raw | Latencia | Agnóstico | Complejidad | Deployment |
|----------|-----------|----------|-----------|-------------|------------|
| **Zoom RTMS** | ✅ Per-participant | Baja (WebSocket) | ❌ Solo Zoom | Media | Cloud backend |
| **Teams Bot** | ✅ PCM 16K | ~20ms frames | ❌ Solo Teams | Alta (.NET, Windows) | Cloud VM Azure |
| **Meet Media API** | ✅ WebRTC | Baja | ❌ Solo Meet | Alta (Preview) | Server-side |
| **Chrome tabCapture** | ✅ MediaStream | Baja (local) | ⚠️ Solo meetings en browser | Baja-Media | Browser extension |
| **Virtual Audio Device** | ✅ Full control | <1ms (local) | ✅ **CUALQUIER app** | **Muy Alta** (kernel) | Desktop app |
| **WASAPI Loopback** | ✅ Audio mezclado | Baja (local) | ⚠️ Solo Windows | Media | Desktop app |
| **Recall.ai** | ✅ Raw streams | Baja | ✅ **Todas las plataformas** | **Baja** (API calls) | Cloud SaaS |

---

## 5. Por Qué la Arquitectura de TalkGo Está Preparada

### Arquitectura Hexagonal = Adapters Plug-and-Play

El pipeline de traducción de TalkGo está diseñado con **ports & adapters** (arquitectura hexagonal). El dominio (traducción) no conoce el transporte (WebRTC, WebSocket, PCM frames). Cada plataforma enterprise es simplemente un **nuevo adapter**:

```
Hoy (App Móvil):
    react-native-webrtc → WebRTC Adapter → Translator Port → Pipeline

Futuro Zoom:
    Zoom RTMS → WebSocket Adapter → Translator Port → Pipeline

Futuro Teams:
    Teams .NET Bot → gRPC Adapter → Translator Port → Pipeline

Futuro Desktop:
    Virtual Audio Device → OS Audio Adapter → Translator Port → Pipeline
```

### Prerequisito: Extracción como Microservicio

Antes de la Fase 5, en la Fase 4 del roadmap se planifica:

```
Extraer internal/app/ + internal/domain/ + internal/ports/driven/
    → Servicio independiente con API gRPC/WebSocket
    → Los adapters de Zoom/Teams/Desktop se conectan a este servicio
    → El servidor WebRTC de la app móvil también se conecta a este servicio
```

Esto permite que el mismo motor de traducción sirva a la app móvil Y a las integraciones enterprise sin duplicar código.

### Lo que NO cambia cuando agregamos enterprise

| Componente | Cambia? | Detalle |
|-----------|---------|---------|
| `internal/domain/` | ❌ | Room, Session, Translation — misma lógica |
| `internal/ports/driven/translator.go` | ❌ | Misma interface `TranslateStream()` |
| `internal/ports/driven/` (todos) | ❌ | Los contratos no cambian |
| `internal/app/` | ❌ | La orquestación es la misma |
| `internal/adapters/webrtc/` | ❌ | Sigue sirviendo a la app móvil |
| `internal/adapters/zoom/` | ✅ NUEVO | Adapter para Zoom RTMS |
| `internal/adapters/teams/` | ✅ NUEVO | Adapter para Teams (o companion .NET service) |
| `internal/adapters/meet/` | ✅ NUEVO | Adapter para Google Meet |

---

## 6. Estrategia de Go-to-Market Enterprise (Alto Nivel)

### Secuencia Recomendada

```
1. App móvil (MVP)       → Valida el producto con usuarios reales
2. API pública           → Permite que otros integren TalkGo
3. Zoom integration      → Primera plataforma enterprise (mejor DX, mayor market share)
4. Teams integration     → Segunda plataforma (mayor penetración enterprise)
5. Desktop universal     → Para empresas que usan plataformas no soportadas
6. Google Meet           → Cuando Media API salga de Preview
```

### Modelo de Negocio Enterprise (Ideas — No Definido)

| Modelo | Descripción |
|--------|-------------|
| **Per-seat/month** | $X/usuario/mes con minutos incluidos |
| **Per-minute** | $X/minuto de traducción (pass-through del costo de API + margen) |
| **Tier-based** | Free (X min/mes) → Pro → Enterprise (custom) |
| **Platform fee** | Fee base por integración + per-minute usage |

### Requisitos para Enterprise Sales

| Requisito | Estado |
|-----------|--------|
| SOC 2 Type II | ❌ Necesario antes de enterprise |
| ISO 27001 | ❌ Necesario antes de enterprise |
| GDPR compliance | ⚠️ Básico en MVP, necesita hardening |
| Data residency | ❌ Necesario para clientes EU/regulados |
| SLA (99.9%+) | ❌ Necesario para enterprise |
| SSO/SAML | ❌ Necesario para enterprise |
| Audit logs | ❌ Necesario para enterprise |
| Admin dashboard | ❌ Necesario para enterprise |

---

## 7. Riesgos Específicos de Enterprise

| Riesgo | Severidad | Mitigación |
|--------|-----------|------------|
| **Teams requiere .NET** — nuestro backend es Go | Alta | Companion service .NET minimal o usar Recall.ai como abstracción |
| **Google Meet API no está en GA** | Media | Esperar. No priorizar Meet como primera integración |
| **Certificación en Marketplaces toma meses** | Media | Iniciar proceso de certificación en paralelo con desarrollo |
| **Costos de compliance (SOC2, ISO)** | Alta | $50-100K+ para auditoría inicial. Planificar como inversión pre-revenue enterprise |
| **Virtual Audio Device = kernel drivers** | Alta | Solo si el ROI enterprise lo justifica. Evaluar Recall.ai primero |
| **Microsoft desaconseja Real-Time Media bots** | Media | Monitorear evolución de la política. Tener fallback a transcripción post-meeting |
| **Rate limits de APIs de traducción a escala enterprise** | Alta | Negociar enterprise tiers con OpenAI. Considerar modelos self-hosted como fallback |
| **Latencia en enterprise (reuniones con muchos participantes)** | Media | Pipeline por participante es O(n). Escalar horizontalmente el servicio de traducción |

---

## 8. Timeline Estimado

| Fase | Cuándo | Prerequisito |
|------|--------|-------------|
| **MVP App Móvil** | Ahora → 7 semanas | — |
| **Validación con usuarios** | Post-MVP, 1-2 meses | App funcionando |
| **API pública** | Fase 4, ~3 meses post-MVP | Microservicio extraído |
| **Zoom RTMS integration** | Fase 5, ~6 meses post-MVP | API pública lista, Zoom Marketplace registration |
| **Teams Bot integration** | Fase 5, ~9 meses post-MVP | Companion .NET service, Azure setup, certificación |
| **Enterprise sales** | ~12 meses post-MVP | SOC 2, ISO 27001, SLA, SSO |

---

## 9. Decisiones Pendientes para Enterprise

| Decisión | Opciones | Cuándo Decidir |
|----------|----------|---------------|
| **¿Recall.ai o bots propios?** | Recall.ai (rápido, vendor dependency) vs. bots custom (control total, más desarrollo) | Antes de Fase 5 |
| **¿Companion .NET service para Teams?** | .NET service + gRPC bridge vs. Recall.ai abstraction | Antes de Teams integration |
| **¿Virtual Audio Device?** | Desarrollar driver propio vs. partnear con alguien que ya lo tenga | Solo si enterprise demand lo justifica |
| **¿Modelo de pricing enterprise?** | Per-seat vs. per-minute vs. tier-based | Antes de enterprise sales |
| **¿Self-hosted option?** | Cloud-only vs. on-prem deployment para clientes regulados | Cuando lleguen requests de healthcare/gobierno |

---

*Este documento se actualizará a medida que el producto evolucione y se acerque la Fase 5 del roadmap.*
