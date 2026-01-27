#!/bin/bash
# Development helper script
# Workaround for wails dev hanging on some systems

set -e

echo "🔨 Building application..."
wails build -clean

echo ""
echo "✅ Build complete!"
echo ""
echo "🚀 Running application..."
echo "   Desktop UI:  ./build/bin/goop2"
echo "   CLI Peer:    ./build/bin/goop2 peer ./peers/peerA"
echo "   Rendezvous:  ./build/bin/goop2 peer ./peers/peerB"
echo ""

# Run the app
./build/bin/goop2
