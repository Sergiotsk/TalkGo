# ADR-0003: Audio Mono por Dispositivo (Eliminación de Canales L/R Estéreo)

## Estado
Accepted

## Contexto
En la concepción inicial del proyecto (Bifocal), se planteó la necesidad de separar el audio en dos canales (Izquierdo/Derecho) dentro de un único stream estéreo para permitir que dos personas compartieran un par de auriculares, escuchando cada una su respectiva traducción.
Sin embargo, esta solución técnica introduce una enorme fricción de hardware (compartir auriculares físicos) y complejidades de software (mezcla estéreo manual por WebRTC y soporte dispar en navegadores y dispositivos móviles).

Dado que el modelo de TalkGo está pensado para que **cada participante use su propio dispositivo móvil individual** (celular, tablet), cada persona cuenta con su propio canal de salida de audio completo.

## Decisión
Decidimos eliminar la separación estéreo de canales L/R en los streams de audio:
- Cada participante se conecta a través de su propio dispositivo usando su conexión WebRTC independiente.
- Los streams de audio de entrada y salida se procesarán nativamente como **señales mono completas**.
- El puerto `AudioMixer` y las lógicas asociadas de mezcla de audio se re-enfocan para combinar pistas de voz + traducción o ambiente en el dispositivo del usuario de ser necesario, pero operando en mono.

## Consecuencias
### Positivas
- **Fácil de usar**: Elimina la incomodidad de compartir auriculares. Cada usuario usa sus auriculares normales (Bluetooth o cable) conectados a su propio teléfono.
- **Simplicidad técnica**: No se requiere manipulación de canales PCM estéreo en el servidor para WebRTC, lo que reduce la carga de CPU y la latencia del servidor.
- **Portabilidad**: Hace que el sistema sea 100% integrable y embebible en plataformas como Zoom, Teams o Google Meet, que operan nativamente con streams de audio individuales/mono.

### Negativas
- **Consumo de Red**: Requiere múltiples conexiones de red por sala en lugar de un único flujo compartido, aunque para 2-3 personas el consumo de ancho de banda adicional es insignificante.
