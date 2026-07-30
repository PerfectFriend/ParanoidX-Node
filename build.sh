#!/bin/bash
# Build the simplex-node Go binary to the usual location

set -e
mkdir -p "$HOME/bin"
go build -o "$HOME/bin/simplex-node" ./cmd/simplex-node
echo "Built → $HOME/bin/simplex-node"
echo "Run with: $HOME/bin/simplex-node -listen 0.0.0.0:8080 -data ~/.local/share/simplex-node"
