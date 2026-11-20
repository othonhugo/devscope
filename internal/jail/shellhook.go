package jail

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/othonhugo/devscope/internal/config"
)

// PrepareShellInit gera arquivos de inicialização efêmeros para Bash ou Zsh que:
// 1. Auto-removem o arquivo temporário logo no carregamento;
// 2. Configuram o prompt indicando o escopo ativo;
// 3. Travam a navegação física (cd -P / chpwd / PROMPT_COMMAND) na raiz do projeto;
// 4. Tornam DEVSCOPE_ROOT e PATH somente-leitura (readonly).
func PrepareShellInit(proj *config.ProjectConfig) (shellBin string, args []string, extraEnv []string, cleanup func(), err error) {
	shellName := filepath.Base(proj.Shell)
	runDir := os.Getenv("XDG_RUNTIME_DIR")
	if runDir == "" {
		runDir = os.TempDir()
	}

	switch shellName {
	case "zsh":
		zdotdir, err := os.MkdirTemp(runDir, "devscope-zsh-*")
		if err != nil {
			return "", nil, nil, nil, err
		}
		cleanup = func() { _ = os.RemoveAll(zdotdir) }

		zshrcPath := filepath.Join(zdotdir, ".zshrc")
		content := buildZshRC(proj, zdotdir)
		if err := os.WriteFile(zshrcPath, []byte(content), 0600); err != nil {
			cleanup()
			return "", nil, nil, nil, err
		}

		// -d: NO_GLOBAL_RCS (evita que /etc/zshenv ou /etc/zshrc sujem o ambiente limpo)
		return proj.Shell, []string{"-d", "-i"}, []string{"ZDOTDIR=" + zdotdir}, cleanup, nil

	default: // bash ou sh
		rcFile, err := os.CreateTemp(runDir, "devscope-bashrc-*")
		if err != nil {
			return "", nil, nil, nil, err
		}
		rcPath := rcFile.Name()
		cleanup = func() { _ = os.Remove(rcPath) }

		content := buildBashRC(proj, rcPath)
		if _, err := rcFile.WriteString(content); err != nil {
			rcFile.Close()
			cleanup()
			return "", nil, nil, nil, err
		}
		rcFile.Close()

		return proj.Shell, []string{"--noprofile", "--rcfile", rcPath, "-i"}, nil, cleanup, nil
	}
}

func buildBashRC(proj *config.ProjectConfig, selfPath string) string {
	return fmt.Sprintf(`# Gerado automaticamente pelo devscope
rm -f %q 2>/dev/null

export DEVSCOPE_ROOT=%q
export DEVSCOPE_PROJECT=%q
export HISTFILE="$DEVSCOPE_ROOT/.devscope_history"
shopt -u autocd 2>/dev/null || true

__devscope_guard_pwd() {
    local phys_pwd
    phys_pwd="$(builtin pwd -P 2>/dev/null)"
    if [[ "$phys_pwd" != "$DEVSCOPE_ROOT" && "$phys_pwd" != "$DEVSCOPE_ROOT/"* ]]; then
        echo "devscope: navegação bloqueada fora do escopo do projeto ($DEVSCOPE_ROOT)" >&2
        builtin cd -P "$DEVSCOPE_ROOT"
    fi
}

cd() {
    local target="${1:-$DEVSCOPE_ROOT}"
    if ! builtin cd -P "$target" 2>/dev/null; then
        echo "devscope: diretório inválido: $target" >&2
        return 1
    fi
    __devscope_guard_pwd
}

pushd() {
    local target="${1:-$DEVSCOPE_ROOT}"
    if ! builtin pushd "$target" >/dev/null 2>&1; then
        return 1
    fi
    __devscope_guard_pwd
}

popd() {
    builtin popd >/dev/null 2>&1 || return 1
    __devscope_guard_pwd
}

PROMPT_COMMAND="__devscope_guard_pwd"
PS1="\[\033[1;36m\](devscope:${DEVSCOPE_PROJECT})\[\033[0m\] \[\033[1;32m\]\w\[\033[0m\] \$ "

readonly DEVSCOPE_ROOT DEVSCOPE_PROJECT PATH PROMPT_COMMAND
readonly -f __devscope_guard_pwd cd pushd popd
builtin cd -P "$DEVSCOPE_ROOT"
`, selfPath, proj.Path, proj.Name)
}

func buildZshRC(proj *config.ProjectConfig, zdotdir string) string {
	return fmt.Sprintf(`# Gerado automaticamente pelo devscope
rm -rf %q 2>/dev/null

export DEVSCOPE_ROOT=%q
export DEVSCOPE_PROJECT=%q
export HISTFILE="$DEVSCOPE_ROOT/.devscope_history"
export HISTSIZE=5000
export SAVEHIST=5000
unsetopt AUTO_CD 2>/dev/null || true

__devscope_chpwd() {
    local phys_pwd="${PWD:A}"
    if [[ "$phys_pwd" != "$DEVSCOPE_ROOT" && "$phys_pwd" != "$DEVSCOPE_ROOT/"* ]]; then
        print -u2 "devscope: navegação bloqueada fora do escopo do projeto ($DEVSCOPE_ROOT)"
        builtin cd -P "$DEVSCOPE_ROOT"
    fi
}

chpwd_functions=(__devscope_chpwd)

cd() {
    local target="${1:-$DEVSCOPE_ROOT}"
    builtin cd -P "$target" || return 1
    __devscope_chpwd
}

PROMPT='%%F{cyan}%%B(devscope:'"$DEVSCOPE_PROJECT"')%%b%%f %%F{green}%%~%%f %%# '
typeset -r DEVSCOPE_ROOT DEVSCOPE_PROJECT PATH chpwd_functions
functions -r __devscope_chpwd cd 2>/dev/null || true
builtin cd -P "$DEVSCOPE_ROOT"
`, strings.ReplaceAll(zdotdir, `"`, `\"`), proj.Path, proj.Name)
}
