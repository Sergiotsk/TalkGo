# Sprint 4: Polish y Alpha

## Objetivo
Instrumentar el sistema con métricas de performance, validar el comportamiento en redes reales (4G, WiFi pública), ejecutar una alpha interna con usuarios reales, y cerrar el MVP con la documentación necesaria para el equipo.

## Enfoque
- Agregar instrumentación de timestamps en el pipeline Go para medir latencia end-to-end (p50, p90).
- Testing en condiciones de red reales: 4G, WiFi pública, NAT simétrico.
- Alpha interna: sesiones reales con usuarios, recolección de feedback.
- Bug fixes y ajustes de latencia basados en datos reales.
- Documentación de onboarding para el equipo.

## Criterios de Aceptación

Según el PRD (sección 13 — Métricas de Éxito):

| ID | Criterio |
|----|---------|
| CA-01 | Latencia end-to-end p50 < 1.0s medida con timestamps en el pipeline Go |
| CA-02 | Latencia end-to-end p90 < 1.5s medida con timestamps en el pipeline Go |
| CA-03 | Tasa de conexión exitosa > 95% en condiciones normales |
| CA-04 | Tasa de reconexión exitosa > 80% ante desconexiones transitorias |
| CA-05 | Errores de pipeline < 2% del total de chunks procesados por sesión |
| CA-06 | Duración promedio de sesión en alpha > 5 minutos (indica utilidad real) |
| CA-07 | El sistema funciona correctamente en redes 4G y WiFi pública |
| CA-08 | El sistema funciona detrás de NAT simétrico vía TURN embebido |

## Entregables
- Instrumentación de latencia en el pipeline Go (timestamps por chunk, logs estructurados)
- Reporte de testing en redes reales (4G, WiFi pública, NAT simétrico)
- Reporte de alpha interna: sesiones realizadas, issues encontrados, latencia observada
- Bug fixes priorizados post-alpha
- README de onboarding para el equipo (setup, comandos, arquitectura básica)

## Definición de MVP Completo

El MVP está listo cuando:
1. Dos personas con idiomas diferentes pueden mantener una conversación fluida usando solo sus teléfonos
2. La latencia end-to-end es < 1.5s en el p90 en condiciones de red normales
3. La app funciona en background en iOS y Android
4. El sistema es estable durante al menos 30 minutos continuos sin degradación
