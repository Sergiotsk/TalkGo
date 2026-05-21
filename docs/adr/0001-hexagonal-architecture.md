# ADR-0001: Usar Arquitectura Hexagonal (Ports & Adapters)

## Estado
Accepted

## Contexto
El proyecto TalkGo/Bifocal necesita soportar traducción simultánea por voz en tiempo real. 
Dado que el stack tecnológico involucra múltiples capas complejas (WebRTC con Pion, APIs de IA como OpenAI Realtime o Whisper/GPT/ElevenLabs, y mezcla de audio), acoplar el dominio de negocio a estas tecnologías externas generaría un sistema sumamente rígido, difícil de testear y propenso a errores al intentar migrar o expandir capacidades.

## Decisión
Decidimos implementar **Arquitectura Hexagonal (Ports & Adapters)**:
- El dominio (`internal/domain/`) permanecerá libre de dependencias de infraestructura y de frameworks.
- Toda interacción externa se definirá mediante interfaces en la capa de límites o puertos (`internal/ports/`).
- Las implementaciones técnicas concretas se realizarán en la capa de adaptadores (`internal/adapters/`).
- La capa de aplicación (`internal/app/`) orquestará los flujos de negocio.

Para asegurar que los agentes de IA (Cursor/Antigravity) no violen estos límites, configuramos `depguard` en `golangci-lint` para fallar el build inmediatamente si se importan adaptadores o librerías de infraestructura en el dominio.

## Consecuencias
### Positivas
- **Mantenibilidad**: Es extremadamente fácil cambiar de adaptadores (por ejemplo, reemplazar Pion WebRTC o agregar una integración nativa de Teams sin tocar la lógica del dominio).
- **Testabilidad**: Toda la lógica del dominio se puede testear de forma aislada y ultra rápida usando mocks de los puertos.
- **Seguridad con IA**: El arnés de IA tiene reglas sumamente claras y automatizadas para guiar el desarrollo.

### Negativas
- **Boilerplate**: Requiere crear múltiples interfaces y archivos que sirvan de puertos antes de escribir la implementación.
- **Curva de Aprendizaje**: Exige rigurosidad extrema para no contaminar las capas internas con conceptos de infraestructura.
