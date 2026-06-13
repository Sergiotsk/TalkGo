# TalkGo — Tester Guide (Expo Go)

This guide is for people who want to test TalkGo on their phone.
You do NOT need to install Node.js, npm, or any developer tools.

---

## What you need

- A smartphone — iPhone (iOS 13+) or Android (8.0+)
- The **room code** from the person organizing the session (a 6-character code like `ABC123`)
- A working Wi-Fi or mobile data connection

---

## Step 1: Install Expo Go

Expo Go is a free app that lets you run TalkGo on your phone without a full app install.

**On iPhone (iOS):**
1. Open the App Store
2. Search for **Expo Go**
3. Install the app by Expo (developer: Expo Project, Inc.)
4. App Store link: [https://apps.apple.com/app/expo-go/id982107779](https://apps.apple.com/app/expo-go/id982107779)

**On Android:**
1. Open the Play Store
2. Search for **Expo Go**
3. Install the app by Expo
4. Play Store link: [https://play.google.com/store/apps/details?id=host.exp.exponent](https://play.google.com/store/apps/details?id=host.exp.exponent)

---

## Step 2: Grant microphone permission

TalkGo needs access to your microphone to work. You will be asked to grant this when the app opens for the first time.

**On iPhone:**
1. When the app asks "Expo Go Would Like to Access the Microphone", tap **OK**
2. If you missed the prompt: go to **Settings → Privacy & Security → Microphone**, find **Expo Go**, and turn the toggle ON

**On Android:**
1. When the app asks to allow microphone access, tap **Allow**
2. If you missed the prompt: go to **Settings → Apps → Expo Go → Permissions → Microphone** and set it to **Allow**

> Without microphone permission, you will hear the other person but they won't hear you.

---

## Step 3: Open the app

The organizer will share either a **QR code** or a **URL**. Use whichever they provide.

**Option A — Scan the QR code:**
1. Open Expo Go
2. Tap the **Scan QR Code** button on the home screen
3. Point your camera at the QR code the organizer shared
4. The TalkGo interface will open automatically

**Option B — Enter the URL manually:**
1. Open Expo Go
2. Tap **Enter URL manually**
3. Type or paste the URL the organizer sent you
   - It looks like: `exp://45-123-45-67.sslip.io/--/`
4. Tap **Go** or press Enter

---

## Step 4: Join a room

Once TalkGo opens inside Expo Go:

1. You will see a field asking for a **room code**
2. Enter the 6-character code the organizer gave you (e.g., `ABC123`)
3. Tap **Join Room**
4. Wait a moment — when you see "Connected", the session is live

---

## Step 5: Start talking

Once connected, speak normally into your phone. TalkGo will translate your voice in real time.

**Tips for best audio quality:**
- Use the phone in a quiet environment — background noise affects translation accuracy
- Speak at a normal pace — rushing or mumbling reduces quality
- Hold the phone about 20–30 cm from your face, or use earphones with a built-in microphone
- Avoid speakerphone in noisy rooms — the echo degrades the audio

---

## Troubleshooting

**"Can't connect" or the app hangs on "Connecting..."**

- Check your internet connection — try opening a website in your browser
- If on Wi-Fi, try switching to mobile data (or vice versa)
- Ask the organizer to confirm the server is running
- Close Expo Go completely and reopen it, then try again

**"No audio" — you can't hear the other person**

- Check your phone volume is not set to zero or silent
- Verify microphone permission is granted (see Step 2)
- If using Bluetooth earphones, make sure they are connected and selected as audio output

**"Microphone not working" — the other person can't hear you**

- Go back to Step 2 and confirm microphone permission is set to Allow
- Restart Expo Go and join the room again

**App crashes or freezes**

- Force-close Expo Go from your phone's app switcher
- Uninstall and reinstall Expo Go from the App Store or Play Store
- Rejoin using the same room code (it stays valid for the session duration)

**Still having issues?**

Contact the session organizer with:
- Your phone model and OS version
- A description of what you see on screen when the problem happens
