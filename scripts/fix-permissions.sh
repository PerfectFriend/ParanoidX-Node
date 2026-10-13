#!/bin/bash
ISLE_HOME="${ISLE_HOME:-$HOME}"  # переносимый дом проекта: переопредели env-ом, дефолт $HOME
set -euo pipefail
echo "=== Fixing simplex-node permissions ==="
chown -R tomas:tomas $ISLE_HOME/.local/share/simplex-node
chown -R tomas:tomas $ISLE_HOME/ParanoidX/docker
for d in $ISLE_HOME/.local/share/simplex-node-A*; do
  [ -d "$d" ] && chown -R tomas:tomas "$d"
done
echo "Done. Now run: bash $ISLE_HOME/ParanoidX/scripts/launch-node.sh"
