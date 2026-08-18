#!/bin/bash
# install.sh für Alpaka

echo "🦙 Installing Alpaka..."

# 1. System und Architektur erkennen
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "arm64" ] || [ "$ARCH" = "aarch64" ]; then
    ARCH="arm64"
fi

# 2. GitHub URL zusammenbauen
REPO="Wheely20/alpaka"
FILENAME="alpaka-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${FILENAME}"

echo "⬇️  Downloading $FILENAME..."

# 3. Datei herunterladen und nach /usr/local/bin verschieben
sudo curl -sSL "$URL" -o /usr/local/bin/alpaka

# 4. Datei ausführbar machen
sudo chmod +x /usr/local/bin/alpaka

echo "✅ Alpaka was successfully installed!"
echo "Type 'alpaka', to start."