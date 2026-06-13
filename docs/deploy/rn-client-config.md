# React Native Client — Production Configuration

The TalkGo mobile client lives in the `mobile/` directory of this repository.
This document explains how to point it at a production server and how to switch between dev and prod.

---

## Files to configure

There are two files that control where the client connects:

### 1. `mobile/src/services/api.ts` — HTTP base URL

This file handles REST calls (create room, find room by code, delete room).

The relevant line:
```ts
const BASE_URL = 'http://localhost:8080';
```

For production this should read from an environment variable. See the env var pattern below.

### 2. `mobile/src/hooks/useSignaling.ts` — WebSocket URL

The `useSignaling` hook receives `serverUrl` as a prop from the screen that calls it.
The screen (`ConversationScreen`) receives `serverUrl` from the navigation params or app config.

Trace back to wherever `serverUrl` is set in your app entry point or navigation setup — that is the value to change for production.

---

## Environment variable pattern

React Native CLI projects support environment variables via `.env` files using
[`react-native-dotenv`](https://github.com/goatandsheep/react-native-dotenv) (add it if not already installed)
or via the built-in `@env` module after Babel configuration.

For Expo-managed or Expo Go distribution, use the `EXPO_PUBLIC_` prefix — these variables are
automatically inlined at build time by the Expo CLI:

```env
# mobile/.env
EXPO_PUBLIC_WS_URL=wss://45-123-45-67.sslip.io/ws/
EXPO_PUBLIC_API_URL=https://45-123-45-67.sslip.io
```

Then in `mobile/src/services/api.ts`:
```ts
const BASE_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';
```

And wherever `serverUrl` is passed to `useSignaling`:
```ts
const serverUrl = process.env.EXPO_PUBLIC_WS_URL ?? 'ws://localhost:8080';
```

> The `EXPO_PUBLIC_` prefix works with `npx expo start`. For bare React Native CLI (`npx react-native start`),
> use `react-native-dotenv` and the plain `WS_URL` / `API_URL` names instead.

---

## Dev vs production switch

| Environment | `.env` values | Start command |
|-------------|---------------|---------------|
| **Dev** (local backend) | `EXPO_PUBLIC_WS_URL=ws://localhost:8080/ws/` | `npx expo start` |
| **Prod** (VPS) | `EXPO_PUBLIC_WS_URL=wss://45-123-45-67.sslip.io/ws/` | `npx expo start --no-dev` |

### Dev (local backend)

```bash
cd mobile

# .env (dev)
echo "EXPO_PUBLIC_WS_URL=ws://localhost:8080/ws/" > .env
echo "EXPO_PUBLIC_API_URL=http://localhost:8080" >> .env

npx expo start
```

- Connects to a local Go server running on port 8080
- Hot reload is active
- On a physical device: the phone must be on the same Wi-Fi as the dev machine.
  Use the machine's local IP instead of `localhost` (e.g., `ws://192.168.1.100:8080/ws/`)

### Prod (VPS via sslip.io)

```bash
cd mobile

# .env (prod)
echo "EXPO_PUBLIC_WS_URL=wss://45-123-45-67.sslip.io/ws/" > .env
echo "EXPO_PUBLIC_API_URL=https://45-123-45-67.sslip.io" >> .env

npx expo start --no-dev
```

- `--no-dev` disables the dev menu and hot reload — closer to production behavior
- Uses WSS (WebSocket Secure over TLS) — required when the backend is HTTPS-only
- The VPS must be running (`docker compose up -d`) before testers connect

---

## Verify the connection

After starting the client with prod config, open the app and check the connection flow:

1. Create or join a room — this hits `POST /rooms` or `GET /rooms/code/{code}`
2. Enter a conversation — the WebSocket connects to `wss://<domain>/ws/{roomID}`
3. The `/health` endpoint confirms server state:
   ```bash
   curl https://45-123-45-67.sslip.io/health
   # Expected: {"status":"ok","turn_configured":true,"api_key_present":true,"codec_mode":"opus"}
   ```

---

## Important: never commit production secrets

Do NOT commit `.env` files with real API keys or passwords to the repository.
The `mobile/.gitignore` should include:
```
.env
.env.local
.env.production
```

Use `.env.example` (no real values) to document the required variables for other developers.
