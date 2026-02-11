#!/usr/bin/env bash
GOOP2_DATA="${XDG_DATA_HOME:-$HOME/.local/share}/goop2"
mkdir -p "$GOOP2_DATA"
cd "$GOOP2_DATA" || exit 1
exec /app/bin/goop2-bin "$@"
