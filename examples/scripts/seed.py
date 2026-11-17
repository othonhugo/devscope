#!/usr/bin/devscope
# devscope: python3 -u

import os
import sys

print("=== Script Python3 Isolado via devscope ===")
print(f"Projeto Ativo : {os.environ.get('DEVSCOPE_PROJECT')}")
print(f"Diretório CWD : {os.getcwd()}")
print(f"DATABASE_URL  : {os.environ.get('DATABASE_URL')}")
print(f"Argumentos    : {sys.argv[1:]}")
