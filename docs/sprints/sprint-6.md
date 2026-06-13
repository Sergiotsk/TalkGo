# Sprint 6 — Expo Migration & Distribution

**Objetivo:** Migrar el cliente mobile de React Native CLI puro a Expo SDK para habilitar distribución via Expo Go sin compilación nativa.

**Estado:** Planificado

---

## Contexto

El cliente mobile fue generado con `react-native init` (RN 0.76.9). No tiene Expo como dependencia y no puede levantarse con `expo start`. Esto bloquea la distribución a testers via Expo Go definida en el Sprint 5.

---

## Requerimientos

### REQ-MOB-EXP-01 — Migración a Expo SDK
El proyecto mobile debe poder iniciarse con `npx expo start` y ser escaneado con Expo Go.

**Criterio de aceptación:**
- `npx expo start` levanta el bundler sin errores
- Un tester con Expo Go instalado puede escanear el QR y ver la app

### REQ-MOB-EXP-02 — Preservar lógica existente
Toda la lógica de `src/` (hooks, screens, services, types) debe funcionar sin cambios de comportamiento.

**Criterio de aceptación:**
- Todos los tests existentes siguen pasando
- `ConversationScreen`, `useSignaling`, `useWebRTC`, `api.ts` sin modificaciones funcionales

### REQ-MOB-EXP-03 — Eliminar artefactos nativos
Los directorios `android/` e `ios/` y el `Gemfile` deben eliminarse del repo — Expo los genera en build time.

**Criterio de aceptación:**
- El repo no contiene `android/`, `ios/`, ni `Gemfile`
- El `.gitignore` excluye los builds nativos generados por Expo

### REQ-MOB-EXP-04 — Compatibilidad con react-native-webrtc
La dependencia `react-native-webrtc` debe ser compatible con la versión de Expo SDK elegida.

**Criterio de aceptación:**
- `react-native-webrtc` funciona en Expo Go o se usa el plugin de config plugin de Expo

---

## Decisiones de diseño

| Decisión | Elección | Razón |
|----------|----------|-------|
| Expo SDK version | 51 o 52 (latest stable) | Compatibilidad con RN 0.73/0.74 y react-native-webrtc |
| Workflow | Expo Go (managed) | Distribución sin compilación — objetivo del sprint |
| package manager | npm (default Expo) | pnpm tiene quirks con Expo en Windows |

---

## Tareas (para SDD)

- [ ] TASK-EXP-01: Eliminar `android/`, `ios/`, `Gemfile` del repo
- [ ] TASK-EXP-02: Reemplazar `package.json` con dependencias Expo SDK
- [ ] TASK-EXP-03: Adaptar `babel.config.js` a `babel-preset-expo`
- [ ] TASK-EXP-04: Adaptar `metro.config.js` a `@expo/metro-config`
- [ ] TASK-EXP-05: Adaptar `app.json` con campos Expo requeridos
- [ ] TASK-EXP-06: Verificar compatibilidad de `react-native-webrtc` con Expo Go
- [ ] TASK-EXP-07: Levantar con `npx expo start` y verificar QR en Expo Go
- [ ] TASK-EXP-08: Correr tests y verificar que pasan

---

## Riesgos

| Riesgo | Probabilidad | Mitigación |
|--------|-------------|-----------|
| `react-native-webrtc` no compatible con Expo Go managed | Alta | Usar Expo Dev Client o reemplazar por `expo-av` + WebSocket manual |
| Diferencias en APIs de módulos nativos | Media | Revisar cada import de RN y reemplazar por equivalente Expo |
