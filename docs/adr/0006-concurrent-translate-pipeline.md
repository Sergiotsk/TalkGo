# ADR-0006 — Pipeline de traducción concurrente con salida ordenada

**Estado:** Aceptado  
**Fecha:** 2026-06-16  
**Sprint:** 9

---

## Contexto

El pipeline original en `PipelineTranslator.TranslateStream` procesaba frases
de forma estrictamente secuencial:

```
frase A: Translate (~1-2s) → TTS (~2.5s) → [frase B recién empieza]
frase B: Translate (~1-2s) → TTS (~2.5s) → [frase C recién empieza]
```

Con un hablante rápido (3 frases en 6 segundos), se formaba un backlog:

```
22:12:33  whisper "She was ruthless..."
22:12:34  whisper "Feared."                     ← entra 1s después
22:12:38  translated "She was ruthless" (5.3s de latencia percibida)
22:12:42  translated "Feared."           (8s de latencia percibida)
```

La frase "Feared." tardó 8 segundos en aparecer aunque la traducción en sí toma <1s.
El 87% del tiempo era cola de espera, no procesamiento real.

## Decisión

Separar la etapa de traducción (I/O bound, paralelizable) de la etapa de TTS
(debe ser secuencial para preservar el orden del audio).

### Diseño: fan-out de traducción + drainer ordenado

```
sttCh ──┬──► [translate goroutine, seq=0] ──┐
        ├──► [translate goroutine, seq=1] ──┤──► resultCh ──► drainer ──► TTS ──► audioCh
        └──► [translate goroutine, seq=2] ──┘                  (ordena)
```

1. **Fan-out**: por cada frase del STT, se asigna un número de secuencia y se lanza
   una goroutine de traducción independiente. Pueden correr N en paralelo.
2. **`resultCh`**: canal buffereado que recibe resultados en el orden que terminen
   (posiblemente desordenado).
3. **Drainer ordenado**: usa un `map[int]translationResult` como buffer. Solo emite
   al canal de salida cuando tiene el siguiente número de secuencia esperado.
4. **TTS secuencial**: dentro del drainer, TTS corre una frase a la vez para evitar
   superposición de audio en el auricular del receptor.

### Resultado esperado

```
frase A: Translate ──┐
frase B: Translate ──┤ (paralelo)    Translate termina ≈ t+1.5s para todas
frase C: Translate ──┘
                       drainer: A→TTS(2.5s)→audio | B→TTS(2.5s)→audio | ...
```

Latencia percibida de frase B: ~1.5s (translate) + espera TTS de A (~2.5s) = ~4s
vs ~8s anterior. Reducción del ~50% en backlog de frases rápidas.

## Manejo de fallos

Si la traducción de seq=N falla, se emite un `translationResult{skipped: true}` para
que el drainer avance el contador sin bloquear frases posteriores.

## Alternativas descartadas

| Alternativa | Razón de descarte |
|-------------|-------------------|
| Worker pool fijo (N goroutines) | Complejidad sin beneficio claro vs goroutine por frase |
| TTS también en paralelo | Audio se superpone en el auricular — inaceptable |
| Drop de frases con backlog alto | Pierde información; mejor ordenar que descartar |
| Canal con capacidad 1 (drop oldest) | Ya existe en capa superior (drainOldest en pipeline.go) |

## Consecuencias

**Positivas:**
- Latencia percibida cae ~50% cuando el hablante produce frases rápidamente
- El orden de audio y transcripts queda preservado
- Sin cambios en la interfaz `driven.Translator` — solo cambia la implementación interna

**Negativas:**
- Picos de N llamadas simultáneas a GPT-4o en conversaciones rápidas
- Mayor consumo de tokens en rafagas (sin throttling — aceptable en MVP)
- Código más complejo: WaitGroup + map de pendientes + secuenciamiento

## Impacto en el código

| Archivo | Cambio |
|---------|--------|
| `internal/adapters/translator/pipeline_translator.go` | Refactor completo del orquestador — fan-out + drainer ordenado |
