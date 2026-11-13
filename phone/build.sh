#!/bin/bash
# Builds dist/PhoneRemote.apk using only tools from Maven Central (no Android SDK needed).
# Requires: JDK 11+, python3, zip, curl.
set -e
cd "$(dirname "$0")"
T=../tools
M=https://repo1.maven.org/maven2
mkdir -p $T ../dist
[ -f $T/dalvik-dx-16.0.1.jar ]  || curl -sfL -o $T/dalvik-dx-16.0.1.jar  $M/com/jakewharton/android/repackaged/dalvik-dx/16.0.1/dalvik-dx-16.0.1.jar
[ -f $T/apksig-2.3.0.jar ]      || curl -sfL -o $T/apksig-2.3.0.jar      $M/com/android/tools/build/apksig/2.3.0/apksig-2.3.0.jar
[ -f $T/android-4.1.1.4.jar ]   || curl -sfL -o $T/android-4.1.1.4.jar   $M/com/google/android/android/4.1.1.4/android-4.1.1.4.jar

rm -rf build && mkdir -p build/classes build/apk/assets
javac --release 8 -nowarn -cp $T/android-4.1.1.4.jar -d build/classes src/com/phoneremote/app/*.java
java -cp $T/dalvik-dx-16.0.1.jar com.android.dx.command.Main --dex --min-sdk-version=24 --output=build/apk/classes.dex build/classes
python3 make_manifest.py build/apk/AndroidManifest.xml
cp assets/index.html build/apk/assets/
(cd build/apk && zip -q -X -r ../unsigned.apk AndroidManifest.xml classes.dex assets)

# Signing key: created locally on first build and never committed (see .gitignore).
KEYPASS="${KEYPASS:-phoneremote}"
[ -f release.p12 ] || keytool -genkeypair -keystore release.p12 -storetype PKCS12 -storepass "$KEYPASS" -keypass "$KEYPASS" \
   -alias phoneremote -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Phone Remote" >/dev/null
javac -nowarn -cp $T/apksig-2.3.0.jar -d build Sign.java
java --add-exports java.base/sun.security.x509=ALL-UNNAMED --add-exports java.base/sun.security.pkcs=ALL-UNNAMED \
     --add-exports java.base/sun.security.util=ALL-UNNAMED \
     -cp $T/apksig-2.3.0.jar:build Sign release.p12 "$KEYPASS" build/unsigned.apk ../dist/PhoneRemote.apk
