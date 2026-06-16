# ADR-0007 — Cambio de modelo de traducción: gpt-4o → gpt-4o-mini

**Estado:** Aceptado  
**Fecha:** 2026-06-16  
**Sprint:** 9

---

## Contexto

La etapa de traducción de texto usaba `gpt-4o` como modelo por defecto.
Con el nuevo pipeline concurrente (ADR-0006), múltiples llamadas pueden correr
en paralelo, multiplicando el costo por frase.

Evaluando la tarea concreta: traducir frases conversacionales cortas (5-20 palabras)
de un idioma a otro. No requiere razonamiento complejo, ni contexto largo, ni
generación creativa. Es una tarea de mapeo lingüístico directo.

## Comparativa

| Modelo | Input | Output | Latencia típica | Calidad traducción corta |
|--------|-------|--------|-----------------|--------------------------|
| gpt-4o | $2.50/1M | $10/1M | ~1-2s | Excelente |
| gpt-4o-mini | $0.15/1M | $0.60/1M | ~0.5-1s | Muy buena |

- Reducción de costo: **94%** (16x más barato)
- Latencia: ~40% menor (modelo más chico, respuesta más rápida)
- Calidad: para frases conversacionales cortas la diferencia es imperceptible

## Decisión

Cambiar el modelo por defecto de traducción de `gpt-4o` a `gpt-4o-mini`.

El campo `Model` en `TextTranslatorConfig` se mantiene configurable — si en el
futuro se necesita mayor calidad para un idioma específico, se puede override
sin cambiar código.

## Alternativas descartadas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| DeepL API | Excelente calidad pero dependencia nueva; evaluar en Sprint posterior |
| Modelo local (OPUS-MT) | Latencia de inferencia local inaceptable sin GPU dedicada |
| Mantener gpt-4o | Costo 16x mayor sin mejora medible en traducción conversacional |

## Consecuencias

**Positivas:**
- Costo de traducción baja de ~$0.01/min a ~$0.001/min por sala
- Latencia de la etapa translate baja ~40%
- Sin cambios en la interfaz ni en el comportamiento observable

**Negativas:**
- Posible degradación en pares de idiomas poco comunes o frases técnicas complejas
- Si la calidad resulta insuficiente, el rollback es cambiar una constante

## Impacto en el código

| Archivo | Cambio |
|---------|--------|
| `internal/adapters/translator/openai_translate.go` | `defaultTranslateModel = "gpt-4o-mini"` |
