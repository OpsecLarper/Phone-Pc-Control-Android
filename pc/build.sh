#!/bin/bash
# Builds dist/PhoneRemote-PC.exe (Windows 64-bit). Requires Go 1.22+. Works from Linux, Mac or Windows (Git Bash).
set -e
cd "$(dirname "$0")"
mkdir -p ../dist
cp ../phone/assets/index.html index.html   # same UI is also served at http://PC-IP:8765
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ../dist/PhoneRemote-PC.exe .
