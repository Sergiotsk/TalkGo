# Runes y Unicode en Go

> Surgió mientras trabajábamos en el filtro de idioma del pipeline de traducción (ADR-0005).
> El problema real: `len("ñoño")` da 6 en vez de 4, y eso rompía el umbral de detección.

---

## El problema base: computadoras y texto

Las computadoras solo entienden números. Para representar texto existe una tabla que
mapea número → carácter. La más básica es **ASCII** (1963):

```
65 → 'A'    97 → 'a'    110 → 'n'
```

ASCII usa **1 byte** por carácter y cubre 128 símbolos. Perfecto para inglés.
Pero no existe la `ñ`, ni la `你`, ni el `😀`.

---

## Unicode — la tabla universal

**Unicode** asigna un número único a cada carácter de todos los idiomas:

```
U+0041 → 'A'
U+00F1 → 'ñ'
U+4F60 → '你'  (chino: "tú")
U+1F600 → '😀'
```

Ese número se llama **code point**. En Go, un `rune` es exactamente eso:
un code point Unicode representado como `int32`.

```go
var r rune = 'ñ'    // r == 241  (U+00F1)
var r2 rune = '你'  // r2 == 20320 (U+4F60)
```

---

## UTF-8: cómo se guardan en memoria

Unicode define los números, pero no dice cómo almacenarlos. **UTF-8** es la
codificación más usada — guarda cada code point usando **1 a 4 bytes**:

```
'A'  → 1 byte   (ASCII puro)
'ñ'  → 2 bytes
'你' → 3 bytes
'😀' → 4 bytes
```

En Go, los `string` son **secuencias de bytes UTF-8**. Ahí está la trampa:

```go
s := "ñoño"

len(s)           // → 6  bytes  (ñ=2, o=1, ñ=2, o=1)
len([]rune(s))   // → 4  runes  (caracteres reales)
```

---

## Por qué importa en TalkGo

En el filtro de idioma (`isExpectedLanguage`), necesitamos contar caracteres
reales — no bytes. Si usáramos bytes:

```go
// Con texto árabe o chino, len() da el triple de caracteres visibles
text := "你好世界"        // 4 caracteres chinos
len(text)          // → 12 bytes  ← INCORRECTO para contar
len([]rune(text))  // → 4 runes   ← lo que el humano cuenta
```

El umbral de 8 que usamos en el filtro se refiere a 8 **caracteres visibles**,
no a 8 bytes. Sin `[]rune()`, frases en árabe o chino pasarían el filtro
aunque fueran largas.

---

## Iteración: bytes vs runes

```go
s := "ñoño"

// MAL — itera bytes, parte caracteres multibyte
for i := 0; i < len(s); i++ {
    fmt.Println(s[i]) // 195, 177, 111, 195, 177, 111 (bytes crudos)
}

// BIEN — range itera runes automáticamente
for i, r := range s {
    fmt.Printf("pos %d → %c\n", i, r)
}
// pos 0 → ñ   (ocupa bytes 0 y 1)
// pos 2 → o   (ocupa byte 2)
// pos 3 → ñ   (ocupa bytes 3 y 4)
// pos 5 → o   (ocupa byte 5)
```

> `range` sobre un string en Go itera runes. Los índices son posiciones en **bytes**,
> por eso salta de 0 a 2 — la `ñ` ocupa 2 bytes.

---

## Resumen

| Concepto | Qué es |
|----------|--------|
| **Byte** | Unidad de memoria (8 bits, valor 0-255) |
| **Code point** | Número único que Unicode asigna a cada carácter |
| **Rune** | Como Go llama al code point — es un `int32` |
| **UTF-8** | Codificación que guarda cada rune usando 1-4 bytes |
| `len(s)` | Cuenta **bytes** |
| `len([]rune(s))` | Cuenta **caracteres reales** |

> Para texto multilingüe: siempre contás runes, nunca bytes.

---

## Dónde aparece esto en el código

```
internal/adapters/translator/pipeline_translator.go
  → isExpectedLanguage(): len([]rune(text)) < 8
  → isPromptEcho(): strings.TrimSpace()
```

Contexto: ADR-0005 — validación de idioma para evitar alucinaciones de Whisper.
