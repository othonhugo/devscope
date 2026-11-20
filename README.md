# devscope

Subshell isolado por projeto em Go para Linux com:
- **Ambiente Limpo (`env -i`)**: Mantém apenas variáveis essenciais (`passthrough_env`) e carrega as variáveis do projeto.
- **Isolamento via Linux User & Mount Namespaces**: Oculta o restante do diretório `/home` do usuário sobrepondo um `tmpfs` em memória e mantendo apenas a raiz do projeto acessível. Zero uso de `os.RemoveAll` em pontos de montagem (risco zero de perda de arquivos).
- **Trava Física de PWD no Shell**: Hooks `readonly` em Bash (`PROMPT_COMMAND` + `cd -P`) e Zsh (`chpwd_functions`) que impedem navegação abaixo da raiz do projeto.
- **Suporte Nativo a Shebang (`#!/usr/bin/devscope`)**: Descoberta reversa automática do projeto (*Longest Prefix Match*) e suporte a múltiplos interpretadores via comentário (`# devscope: python3`, `// devscope: node`).
- **Filtragem de Comandos por Perfil**: Constrói uma *symlink farm* efêmera para o `PATH` aplicando regras de `allow` e `block`.

## Compilação e Instalação

```bash
go mod tidy
go build -o devscope ./cmd/devscope
sudo install -m 0755 devscope /usr/bin/devscope
```

## Configuração Rápida

Por padrão, `sudo` não é incluído no `PATH` da sessão. Para permitir o comando, defina `allow_sudo = true` em `[defaults]` no `config.toml`; perfis com `allow` também precisam incluir `sudo`, se usarem allow-list.

```bash
mkdir -p ~/.config/devscope/{projects,profiles} ~/workspace/demo-app
cp examples/config/config.toml ~/.config/devscope/
cp examples/config/projects/demo-app.toml ~/.config/devscope/projects/
cp examples/config/profiles/backend-safe.toml ~/.config/devscope/profiles/
cp -r examples/scripts ~/workspace/demo-app/
```

## Uso

```bash
# Listar projetos
devscope list

# Entrar no subshell isolado do projeto
devscope demo-app

# Executar comando único no escopo
devscope demo-app -c 'env && pwd'

# Executar scripts diretamente via Shebang (Bash ou Python)
~/workspace/demo-app/scripts/migrate.sh
~/workspace/demo-app/scripts/seed.py --env dev
```
