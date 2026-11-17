#!/usr/bin/devscope
set -euo pipefail

echo "=== Script Shell no Escopo ==="
echo "Projeto : ${DEVSCOPE_PROJECT}"
echo "Raiz    : ${DEVSCOPE_ROOT}"
echo "PWD     : $(pwd)"
echo "DB URL  : ${DATABASE_URL:-nao_definida}"
