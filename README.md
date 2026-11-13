# Phone Remote

Control your Windows PC's **mouse, keyboard and volume** from your Android phone over Wi-Fi.

- **Trackpad**: drag one finger to move, tap to left-click, tap with two fingers to right-click, drag with two fingers (or along the right edge) to scroll. Left / Hold (for dragging) / Right buttons, adjustable speed.
- **Keyboard**: types through your phone's keyboard, with Esc, Tab, Enter, Backspace, arrows, and Ctrl / Alt / Win that stay on for the next key (double-tap Win for the Start menu).
- **Volume**: slider and mute, kept in sync with the PC. Also previous / play-pause / next.

## Setup

1. **PC:** run `PhoneRemote-PC.exe`. It shows your PC's IP address and a 4-digit PIN.
   If Windows Firewall asks, choose **Allow access** (Private networks).
2. **Phone:** install `PhoneRemote.apk` (Android 7+), open it, and enter the IP and PIN.
3. Phone and PC must be on the same Wi-Fi network.

No app? Open `http://<PC-IP>:8765` in your phone's browser; it's the same remote.

## Security

- Only devices on your local network can connect, and only with the PIN.
- The PIN is created at random on first run and saved next to the .exe in `phoneremote-pin.txt`. Edit that file to change it.
- Traffic is **not encrypted**. Use it on your home Wi-Fi, not public networks.

## Building from source

| Part | Requirements | Command | Output |
|------|--------------|---------|--------|
| Android app | JDK 11+, python3, zip, curl | `./phone/build.sh` | `dist/PhoneRemote.apk` |
| PC server (Windows x64) | Go 1.22+ | `./pc/build.sh` | `dist/PhoneRemote-PC.exe` |

The Android build doesn't need the Android SDK. It downloads three small tools from Maven Central
(dx, apksig, android stubs) and writes the binary manifest itself (`phone/make_manifest.py`).
A signing key (`phone/release.p12`) is created on the first build. It is git-ignored, so keep it if you want future builds to install as updates.

## How it works

- `phone/assets/index.html`: the whole remote UI. The app is a WebView that loads it.
- `pc/main.go`: a WebSocket server on port 8765 that checks the PIN and receives small JSON commands.
- `pc/input_windows.go`: sends the input through Windows `SendInput` and sets the volume through Core Audio.
