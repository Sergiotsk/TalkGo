# Sprint 3: UX y Edge Cases

## Objetivo
Completar la experiencia de usuario de la pantalla de conversación activa y hacer el sistema robusto ante escenarios reales: desconexiones, timeouts, errores de API, y background mode en iOS/Android.

## Enfoque
- Implementar la UI de conversación activa en React Native (VU meters, estado de conexión, mute).
- Agregar reconexión automática con backoff exponencial (ICE restart).
- Manejar errores del pipeline de traducción con fallbacks y feedback visual.
- Configurar background mode en iOS (UIBackgroundModes: audio) y Android (Foreground Service).
- Testing exhaustivo de edge cases con dispositivos reales.

## Criterios de Aceptación

Según el PRD (RF-07, RNF-02 y sección 12 — Edge Cases):

| ID | Criterio |
|----|---------|
| CA-01 | La pantalla se mantiene activa (no se apaga) durante una sesión |
| CA-02 | Los VU meters responden en tiempo real al audio entrante y saliente |
| CA-03 | El botón "Finalizar" requiere confirmación ("¿Terminar conversación?") |
| CA-04 | Al finalizar, ambos dispositivos reciben notificación de fin de sesión |
| CA-05 | Reconexión automática hasta 3 intentos con backoff exponencial (1s, 2s, 4s) |
| CA-06 | Período de gracia de 30s antes de cerrar la sala por desconexión |
| CA-07 | Si la API de traducción falla persistentemente, se muestra fallback de texto en pantalla |
| CA-08 | Si el Bluetooth se desconecta mid-sesión, fallback automático al micrófono integrado |
| CA-09 | En iOS, la sesión continúa en background con UIBackgroundModes: audio |
| CA-10 | En Android, Foreground Service mantiene la sesión activa con notificación persistente |
| CA-11 | Un tercer usuario que intenta unirse recibe error claro: "Esta sala ya tiene 2 participantes" |
| CA-12 | Una sala expirada devuelve error: "Esta sala expiró. Creá una nueva." |

## Entregables
- Pantalla de conversación activa (React Native): VU meters, indicador de estado, mute, timer, botón finalizar
- Lógica de reconexión automática con ICE restart
- Manejo de errores del pipeline con feedback visual
- Background mode configurado en iOS y Android
- Suite de tests de edge cases documentados y ejecutados en dispositivos reales
