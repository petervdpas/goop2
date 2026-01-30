#!/bin/bash
set -e

echo "🔨 Building application..."
wails build -clean -platform linux/arm64 -tags webkit2_41

echo ""
echo "✅ Build complete!"
echo ""
echo "🚀 Running application..."
echo "   Desktop UI:  ./build/bin/goop2"
echo "   CLI Peer:    ./build/bin/goop2 peer ./peers/peerA"
echo "   Rendezvous:  ./build/bin/goop2 peer ./peers/peerB"
echo ""

./build/bin/goop2
