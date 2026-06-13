# Sprint 7 — Navegación, Onboarding y Prueba Real

**Objetivo:** Eliminar todos los props hardcodeados del cliente mobile y habilitar una prueba end-to-end real entre dos dispositivos en la misma red.

**Estado:** Planificado

---

## Contexto

El cliente mobile tiene `ConversationScreen` hardcodeada en `App.tsx` (roomId="test-room-id"). Para una prueba real se necesita navegación completa: onboarding de primer uso y home screen con flujos Crear/Unirse sala.

---

## Requerimientos

### REQ-NAV-01 — Guard de onboarding
Primer uso → OnboardingScreen. Usos siguientes → HomeScreen directamente.

### REQ-NAV-02 — OnboardingScreen
Pantalla de bienvenida con 3 bullets informativos, selector de idioma (ES/PT/EN/FR), campo nombre, botón Continuar deshabilitado si nombre vacío.

### REQ-NAV-03 — HomeScreen: Crear sala
Botón "Crear sala" → POST /rooms → muestra shortCode de 6 chars → navega a ConversationScreen.

### REQ-NAV-04 — HomeScreen: Unirse a sala
Input de 6 chars (mayúsculas) → GET /rooms/code/{code} → navega a ConversationScreen. Errores 404/410 con mensaje claro.

### REQ-NAV-05 — ConversationScreen vía navegación
Recibe roomId, localLang, peerLang, userId desde route.params. Sin props hardcodeadas.

### REQ-NAV-06 — Persistencia de identidad
Nombre e idioma persisten entre sesiones via AsyncStorage. Hidratación al montar App.

### REQ-NAV-07 — Prueba real entre dos dispositivos
Dos dispositivos en la misma red pueden iniciar una conversación de traducción real.

---

## Stack técnico

| Dependencia | Uso |
|-------------|-----|
| `@react-navigation/native` + `@react-navigation/native-stack` | Stack navigator |
| `react-native-screens` + `react-native-safe-area-context` | Peers de navigation |
| `@react-native-async-storage/async-storage` | Persistencia nombre/idioma |
| `zustand` (ya instalado) | `useUserStore` — identidad runtime |

---

## Arquitectura de navegación

```
App.tsx
└── NavigationContainer
    └── RootNavigator (Stack)
        ├── OnboardingScreen  ← solo primer uso
        ├── HomeScreen        ← pantalla principal
        └── ConversationScreen ← recibe params reales
```

---

## Tasks (21 tasks, 8 fases)

### FASE 1 — Dependencias
- [ ] TASK-NAV-01: Instalar React Navigation + peers
- [ ] TASK-NAV-02: Instalar AsyncStorage
- [ ] TASK-NAV-03: Mocks de AsyncStorage y react-native-screens en jest
- [ ] TASK-NAV-04: Mock de @react-navigation para tests

### FASE 2 — useUserStore
- [ ] TASK-NAV-05: [TEST] Tests de useUserStore
- [ ] TASK-NAV-06: Implementar useUserStore (Zustand + AsyncStorage)

### FASE 3 — OnboardingScreen
- [ ] TASK-NAV-07: Tipos de navegación (RootStackParamList)
- [ ] TASK-NAV-08: [TEST] Tests de OnboardingScreen
- [ ] TASK-NAV-09: Implementar OnboardingScreen

### FASE 4 — HomeScreen: Crear sala
- [ ] TASK-NAV-10: [TEST] Tests flujo Crear Sala
- [ ] TASK-NAV-11: Implementar flujo Crear Sala

### FASE 5 — HomeScreen: Unirse
- [ ] TASK-NAV-12: [TEST] Tests flujo Unirse
- [ ] TASK-NAV-13: Implementar flujo Unirse

### FASE 6 — Navigator + guard
- [ ] TASK-NAV-14: [TEST] Tests guard onboarding en App.tsx
- [ ] TASK-NAV-15: Crear RootNavigator
- [ ] TASK-NAV-16: Refactorizar App.tsx

### FASE 7 — ConversationScreen
- [ ] TASK-NAV-17: [TEST] Tests ConversationScreen con navigation params
- [ ] TASK-NAV-18: Refactorizar ConversationScreen

### FASE 8 — Verificación
- [ ] TASK-NAV-19: Suite completa con coverage
- [ ] TASK-NAV-20: TypeScript sin errores
- [ ] TASK-NAV-21: Smoke test en dispositivo

---

## Riesgos

| Riesgo | Probabilidad | Mitigación |
|--------|-------------|-----------|
| Peer deps de React Navigation conflictuando | Media | Usar `npx expo install` para resolución automática |
| ConversationScreen refactor rompe tests existentes | Media | TASK-NAV-17 primero, refactor después |
| peerLang desconocido en flujo guest | Baja | Backend lo resuelve; si no, preguntar al usuario antes de navegar |
