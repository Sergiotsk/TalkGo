# Checklist de Tareas — Usuario (Sprint 5 Alpha)

Estas son las tareas que **solo vos podés hacer** para que el alpha funcione.
El código del servidor lo implementa el equipo de desarrollo. Tu parte es la infraestructura y la distribución.

---

## Antes de arrancar el deploy

### ✅ TAREA-U00 — Crear tu clave SSH (hacerlo UNA sola vez, en tu máquina local)

Una clave SSH te permite conectarte al VPS de forma segura, sin contraseña.

**Qué hacer (en PowerShell local):**

```powershell
# 1. Crear la clave (formato ed25519, la más moderna y segura)
ssh-keygen -t ed25519 -C "talkgo-hetzner"
# Presioná Enter en todo (ubicación default y sin passphrase)

# 2. Ver la clave PÚBLICA (esta se sube a Hetzner)
cat $env:USERPROFILE\.ssh\id_ed25519.pub
```

**Dónde quedan los archivos:**
| Archivo | Ubicación | Qué es |
|---------|-----------|--------|
| `id_ed25519` | `C:\Users\Serjito\.ssh\id_ed25519` | Clave PRIVADA — nunca la compartas |
| `id_ed25519.pub` | `C:\Users\Serjito\.ssh\id_ed25519.pub` | Clave PÚBLICA — esta va en Hetzner |

**Cómo conectarte al VPS una vez creado:**
```powershell
ssh root@<IP-del-VPS>
# Ejemplo: ssh root@45.123.45.67
```

**Deshabilitar acceso por contraseña (más seguro):**
Una vez conectado al VPS, ejecutá:
```bash
sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config
systemctl restart sshd
```

**Por qué:** Sin clave SSH no podés gestionar el servidor. Es el paso cero de cualquier deploy.

---

### ✅ TAREA-U01 — Crear cuenta en Hetzner y provisionar el VPS

**Qué hacer:**
1. Entrá a [hetzner.com](https://www.hetzner.com) y creá una cuenta
2. En la consola, creá un nuevo servidor:
   - **Tipo**: CX11 (2 vCPU, 2 GB RAM) — €3.29/mes
   - **Región**: Falkenstein o Helsinki (la más cercana a São Paulo disponible) — o elegí el datacenter de Ashburn si Hetzner no tiene SA
   - **OS**: Ubuntu 24.04 LTS
   - **Sin firewall por ahora** (lo configuramos después)
3. Anotá la **IP pública** que te asigna (ej. `45.123.45.67`)

**Por qué:** El servidor Go, Coturn y Caddy van a correr en este VPS.

---

### ✅ TAREA-U02 — Construir tu dominio gratuito con sslip.io

**Qué hacer:**
1. Con la IP del paso anterior, reemplazá los puntos por guiones
2. Agregá `.sslip.io` al final

**Ejemplo:**
```
IP:     45.123.45.67
Dominio: 45-123-45-67.sslip.io
```

3. Verificá que resuelve: `ping 45-123-45-67.sslip.io` — tiene que devolver la misma IP
4. **Compartí la IP y el dominio con el equipo de desarrollo** para que finalicen el `Caddyfile` y el `coturn.conf`

**Por qué:** Caddy usa este dominio para obtener el certificado HTTPS automáticamente con Let's Encrypt.

---

### ✅ TAREA-U03 — Verificar tu OpenAI API Key con acceso a Realtime

**Qué hacer:**
1. Entrá a [platform.openai.com](https://platform.openai.com)
2. Verificá que tu API key tiene acceso al modelo `gpt-4o-realtime-preview`
3. Asegurate de tener crédito disponible (el alpha consume ~$0.06/minuto de conversación)
4. Recomendado: configurá un **Usage Limit** en OpenAI para no pasarte del presupuesto

**Por qué:** El servidor llama a la API de OpenAI Realtime en cada sesión de traducción.

---

## Durante el deploy

### ✅ TAREA-U04 — Instalar Docker en el VPS

**Qué hacer:**
1. Conectate al VPS por SSH: `ssh root@<tu-IP>`
2. Ejecutá estos comandos:
```bash
curl -fsSL https://get.docker.com | sh
systemctl enable docker
systemctl start docker
```
3. Verificá: `docker --version` tiene que mostrar algo como `Docker version 27.x`

**Por qué:** Todo el stack (Go server + Coturn + Caddy) corre en contenedores Docker.

---

### ✅ TAREA-U05 — Abrir los puertos necesarios en el firewall del VPS

**Qué hacer:**
En la consola de Hetzner, en la sección **Firewall**, habilitá:

| Puerto | Protocolo | Para qué |
|--------|-----------|---------|
| 22 | TCP | SSH (ya abierto) |
| 80 | TCP | HTTP (Caddy redirect a HTTPS) |
| 443 | TCP | HTTPS + WebSocket (Caddy) |
| 3478 | UDP | TURN (Coturn) |
| 3478 | TCP | TURN alternativo |
| 5349 | TCP | TURN sobre TLS |
| 49152-65535 | UDP | Media relay TURN |

**Por qué:** Sin estos puertos el TURN server no funciona y los usuarios detrás de NAT no se conectan.

---

### ✅ TAREA-U06 — Subir el proyecto y levantar los servicios

**Qué hacer:**
1. Copiá el proyecto al VPS (el equipo te da el comando exacto, algo como):
```bash
scp -r ./talkgo root@<tu-IP>:/opt/talkgo
```
2. Conectate por SSH y entrá a la carpeta:
```bash
ssh root@<tu-IP>
cd /opt/talkgo
```
3. Creá el archivo `.env` con tus credenciales (el equipo te da la plantilla):
```bash
cp .env.example .env
nano .env   # Editá OPENAI_API_KEY y TURN_PASSWORD
```
4. Levantá todo:
```bash
docker compose up -d
```
5. Verificá que los 3 servicios están corriendo:
```bash
docker compose ps
```
6. Verificá HTTPS: `curl https://<tu-dominio>.sslip.io/health`

**Por qué:** Un solo comando levanta el servidor Go, Coturn y Caddy juntos.

---

## Distribución a testers

### ✅ TAREA-U07 — Configurar el cliente React Native con la URL de producción

**Qué hacer:**
1. En el repo del cliente React Native, buscá el archivo de configuración donde está la URL del servidor (ej. `config.ts`, `constants.ts`, o similar)
2. Cambiá la URL de `ws://localhost:8080` a `wss://<tu-dominio>.sslip.io`
3. Consultá al equipo si no encontrás el archivo exacto

**Por qué:** El cliente tiene que apuntar al servidor de producción, no a localhost.

---

### ✅ TAREA-U08 — Preparar Expo Go para los testers

**Qué hacer:**
1. Instalá **Expo Go** en tu celular:
   - iOS: [App Store — Expo Go](https://apps.apple.com/app/expo-go/id982107779)
   - Android: [Play Store — Expo Go](https://play.google.com/store/apps/details?id=host.exp.exponent)
2. En tu máquina de desarrollo, en la carpeta del cliente RN, ejecutá:
```bash
npx expo start
```
3. Escaneá el QR con la cámara (iOS) o desde la app Expo Go (Android)
4. Pediles a los testers que hagan lo mismo: instalar Expo Go y escanear el QR

**Por qué:** Expo Go permite distribuir la app sin pasar por TestFlight ni APK firmado. Es la forma más rápida para un alpha de 5 usuarios.

---

## Resumen — Orden de ejecución

```
0. TAREA-U00 — Crear clave SSH en tu máquina local (hacerlo UNA sola vez)
1. TAREA-U01 — Crear VPS en Hetzner (cargar la clave pública al crearlo)
2. TAREA-U02 — Construir dominio sslip.io + compartir IP con el equipo
3. TAREA-U03 — Verificar OpenAI API Key
4. TAREA-U04 — Instalar Docker en el VPS
5. TAREA-U05 — Abrir puertos en el firewall
6. TAREA-U06 — Subir proyecto y levantar servicios (después de que el equipo entregue el código)
7. TAREA-U07 — Configurar URL del cliente RN
8. TAREA-U08 — Distribuir Expo Go a testers
```

**Las tareas U01-U05 las podés hacer YA**, en paralelo al desarrollo del Sprint 5.
**Las tareas U06-U08** van después de que el equipo entregue el código.

---

## Contacto con el equipo

Si en algún paso necesitás ayuda, consultá con el equipo proporcionando:
- La IP pública del VPS (necesaria para U02)
- Si algún comando falla, el mensaje de error completo
