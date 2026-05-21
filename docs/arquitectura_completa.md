# Especificación de Requerimientos de Producto (PRD) & Arquitectura Técnica

**Proyecto:** Traductor Simultáneo Inmersivo de Alta Fluidez (Bifocal)
**Archivo:** `arquitectura_completa.md`
**Estado:** Core Unificado (Captura Dual-Device + Pipeline de IA + Salida Estéreo)

---

## 1. Objetivo del Producto
El objetivo principal es desarrollar una plataforma de traducción bilateral en tiempo real que emule la fluidez de una conversación natural cara a cara. A diferencia de las herramientas de traducción tradicionales —que fuerzan a los usuarios a hablar por turnos rígidos y pausados ("hablar-esperar-escuchar")—, esta aplicación permite una **conversación simultánea (bifocal)**. Esto se logra mediante la separación estricta de canales de audio desde la captura, el procesamiento concurrente en el backend y una salida de audio espacializada.

---

## 2. Visión del Flujo Conversacional
1. **Naturalidad:** Los dos usuarios pueden hablar al mismo tiempo o interrumpirse de manera orgánica, tal como ocurre en una charla cotidiana.
2. **Baja Latencia:** El tiempo transcurrido entre la emisión de la voz en el idioma de origen y la recepción del audio traducido en el idioma de destino debe ser inferior al umbral de percepción disruptiva (< 1 segundo en total).
3. **Inmersión:** El usuario escucha a su interlocutor traducido con una espacialidad definida, facilitando la identificación instintiva de quién está hablando sin confundirse con su propio retorno.

---

## 3. Arquitectura del Sistema (End-to-End)

El flujo de datos se divide en tres capas fundamentales: **Captura e Ingesta Concurrente**, **Core de Procesamiento (Pipeline de IA)** y **Salida de Audio Espacial**.

### Capa 1: Captura e Ingesta Concurrente (Dual-Device)
* **El Problema Físico del Hardware:** Los sistemas operativos móviles (iOS y Android) restringen el uso simultáneo del micrófono integrado del teléfono y el micrófono de un auricular Bluetooth para separar dos pistas de audio independientes en un mismo dispositivo.
* **La Solución (Cooperación de Dispositivos):** Emparejamiento de dos smartphones en una misma sesión en vivo.
    * **Dispositivo A (Usuario A):** Captura el audio limpio de la *Voz A* utilizando su propio teléfono (o su auricular vinculado).
    * **Dispositivo B (Usuario B):** Captura el audio limpio de la *Voz B* utilizando su respectivo teléfono.
* **Protocolo de Conectividad (WebRTC):** * Se utiliza para el streaming de audio puro y continuo con latencia imperceptible.
    * Un **Servidor de Señalización** intermedio gestiona el intercambio de metadatos de red para conectar ambos dispositivos en una sesión única.
    * Se implementan servidores **STUN/TURN** para garantizar el bypass de firewalls y NATs en redes móviles o Wi-Fi públicas.

### Capa 2: Backend en Go & Procesamiento Concurrente (El Motor)
El servidor central que recibe las señales está desarrollado en **Go (Golang)** para aprovechar su rendimiento en bajo nivel y su modelo nativo de concurrencia.
* **Goroutines de Escucha:** El backend levanta una `goroutine` independiente y dedicada exclusivamente para escuchar cada canal de audio entrante de forma asíncrona y no bloqueante (una para el Dispositivo A y otra para el Dispositivo B).
* **Go Channels:** Se emplean para transmitir (pipear) los buffers de audio capturados de manera segura y eficiente hacia el pipeline de Inteligencia Artificial, evitando colisiones de memoria o bloqueos en el flujo de datos.

### Capa 3: Pipeline de Procesamiento de IA (Core)
Una vez que las Goroutines estabilizan los paquetes de audio, el pipeline procesa la información de manera asíncrona para cada canal mediante el siguiente flujo secuencial:

```
[Audio Raw (WebRTC)] ➔ (Faster Whisper) ➔ [Texto] ➔ (GPT-4o) ➔ [Traducción] ➔ (ElevenLabs) ➔ [Audio Traducido]
```

1. **Transcripción Ultra-Rápida (STT - Speech to Text):**
    * **Tecnología:** **Faster Whisper** (procesamiento por chunks/bloques de streaming de audio).
    * **Objetivo:** Convertir el flujo de voz a texto en milisegundos, manteniendo marcas de tiempo y tolerando interrupciones.
2. **Traducción Contextual Dinámica (LLM):**
    * **Tecnología:** API de **OpenAI (GPT-4o / GPT-4o-realtime)**.
    * **Objetivo:** Evitar la traducción literal palabra por palabra. El modelo analiza el contexto semántico, modismos e intención de la oración para generar una traducción fluida, natural y coloquial en el idioma de destino.
3. **Síntesis de Voz de Alta Fidelidad (TTS - Text to Speech):**
    * **Tecnología:** API de **ElevenLabs** (Modelos optimizados Turbo/Multilingual).
    * **Objetivo:** Generar el flujo de audio de la traducción con voces de aspecto humano, conservando la naturalidad y los matices emocionales de la interacción.

### Capa 4: Renderizado y Salida Estéreo Inmersiva
* El backend recibe los flujos de audio sintetizados por ElevenLabs y los empaqueta asignándoles un canal de salida de audio específico para mantener la espacialidad en los auriculares del oyente.
* **Mapeo de Canales:**
    * **Canal Izquierdo (L):** Voz traducida del Interlocutor A.
    * **Canal Derecho (R):** Voz traducida del Interlocutor B.
* El flujo de audio estéreo final se transmite de vuelta a los dispositivos a través de WebRTC para su reproducción inmediata.

---

## 4. Stack Tecnológico Seleccionado

| Componente | Tecnología | Justificación Técnica |
| :--- | :--- | :--- |
| **Frontend Móvil** | React Native | Multiplataforma nativo (iOS/Android) con excelente soporte para librerías WebRTC nativas y manejo de flujos de audio. |
| **Backend de Ingesta** | Go (Golang) + `pion/webrtc` | Concurrencia nativa ultraligera (Goroutines) ideal para procesamiento de streams de audio de alta densidad sin degradación de performance. |
| **Motor STT** | Faster Whisper | Reimplementación optimizada de Whisper en C++ que reduce drásticamente la latencia y el consumo de memoria en entornos de producción. |
| **Capa de Traducción** | OpenAI API (GPT-4o) | Capacidad avanzada de comprensión contextual, manejo de modismos y baja latencia de respuesta estructurada. |
| **Motor TTS** | ElevenLabs API | Calidad de síntesis de voz líder en la industria, soporte multilingual avanzado y modelos Turbo de ultra-baja latencia. |

---

## 5. Matriz de Control de Flujo y Casos de Borde

* **Latencia Acumulada:** Si la red fluctúa, el backend debe priorizar el descarte de frames de audio antiguos en favor del tiempo real (Jitter Buffer adaptativo en WebRTC).
* **Interrupciones:** Si el Usuario B interrumpe al Usuario A, la `goroutine` del Canal A debe continuar procesando hasta un punto de pausa lógico, mientras la `goroutine` del Canal B inicia instantáneamente su pipeline de traducción sin bloquear al canal opuesto.
* **Pérdida de Conexión de Señalización:** Si un dispositivo pierde la conexión, el servidor de Go debe sostener la sesión durante un período de gracia (reconexión rápida) antes de dar por finalizada la sala.

---

## 6. Hoja de Ruta de Desarrollo (Roadmap)

### Fase 1: MVP de Conectividad e Ingesta (Go + WebRTC)
* [ ] Configurar el servidor de señalización WebRTC básico en Go.
* [ ] Desarrollar la interfaz mínima en React Native para emparejar dos dispositivos (ej. mediante un código de sala o QR).
* [ ] Validar que el servidor de Go reciba ambos streams de audio en tiempo real y los asigne correctamente a dos Goroutines independientes sin pérdida de paquetes.

### Fase 2: Integración del Pipeline de Entrada (STT Local/Cloud)
* [ ] Conectar la salida de los canales (Go Channels) con una instancia de Faster Whisper.
* [ ] Validar la salida de texto en la consola del servidor en tiempo real a medida que se habla por los micrófonos de los dispositivos móviles.

### Fase 3: Integración del Pipeline de Salida (LLM + TTS + Estéreo)
* [ ] Pipear el texto de Faster Whisper hacia la API de GPT-4o.
* [ ] Enviar la respuesta traducida a la API de ElevenLabs.
* [ ] Programar la lógica en Go para inyectar el paneo estéreo (Izquierda/Derecha) en los canales correspondientes.
* [ ] Transmitir el audio final de regreso a los dispositivos y verificar la correcta escucha espacial en auriculares.
