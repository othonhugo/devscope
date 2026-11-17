package runner

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devscope/devscope/internal/config"
	"github.com/devscope/devscope/internal/env"
	"github.com/devscope/devscope/internal/filter"
	"github.com/devscope/devscope/internal/jail"
)

// RunShebangScript executa um script invocado via #!/usr/bin/devscope.
// 1. Descobre a qual projeto o script pertence via Longest Prefix Match;
// 2. Inspeciona as primeiras 5 linhas em busca de diretivas (# devscope: python3, // devscope: node);
// 3. Valida se o interpretador está autorizado no PATH sintético do perfil;
// 4. Executa sem PTY para preservar pipes (stdout/stderr separados) dentro do Mount Namespace.
func RunShebangScript(global *config.GlobalConfig, scriptPath string, scriptArgs []string) (int, error) {
	absScript, err := filepath.Abs(scriptPath)
	if err != nil {
		return 1, err
	}
	realScript, err := filepath.EvalSymlinks(absScript)
	if err != nil {
		return 1, fmt.Errorf("script não encontrado (%s): %w", scriptPath, err)
	}

	proj, err := config.FindProjectByPath(realScript, global)
	if err != nil {
		return 1, fmt.Errorf("bloqueio de escopo no shebang: %w", err)
	}

	prof, err := config.LoadProfile(proj.Profile)
	if err != nil {
		return 1, err
	}

	synthBin, cleanupBin, err := filter.BuildSyntheticPath(global.Defaults.BasePath, prof)
	if err != nil {
		return 1, err
	}
	defer cleanupBin()

	cleanEnv, err := env.BuildCleanEnv(global, proj, synthBin)
	if err != nil {
		return 1, err
	}

	interpCmd, err := detectInterpreter(realScript, proj, synthBin)
	if err != nil {
		return 1, err
	}

	targetBin := interpCmd[0]
	targetArgs := append(interpCmd, realScript)
	targetArgs = append(targetArgs, scriptArgs...)

	payload := jail.NamespacePayload{
		ProjectRoot: proj.Path,
		ExtraMounts: proj.ExtraMounts,
		TargetBin:   targetBin,
		TargetArgs:  targetArgs,
		CleanEnv:    cleanEnv,
		StrictNS:    global.Defaults.StrictNamespace,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return 1, err
	}

	selfExe, err := os.Executable()
	if err != nil {
		selfExe = "/proc/self/exe"
	}

	cmd := exec.Command(selfExe, jail.InternalBootstrapArg)
	cmd.Dir = proj.Path
	cmd.Env = append(cleanEnv, fmt.Sprintf("%s=%s", jail.PayloadEnvKey, string(payloadBytes)))
	cmd.SysProcAttr = jail.ConfigureSysProcAttr(global.Defaults.StrictNamespace)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

// detectInterpreter lê até as 5 primeiras linhas do script procurando diretivas como:
//   # devscope: python3 -u
//   // devscope: node
//   -- devscope: lua
func detectInterpreter(scriptPath string, proj *config.ProjectConfig, synthBin string) ([]string, error) {
	f, err := os.Open(scriptPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineCount := 0
	var directive string

	for scanner.Scan() && lineCount < 5 {
		lineCount++
		line := strings.TrimSpace(scanner.Text())
		if lineCount == 1 && strings.HasPrefix(line, "#!") {
			continue
		}

		for _, prefix := range []string{"#", "//", "--"} {
			if strings.HasPrefix(line, prefix) {
				body := strings.TrimSpace(strings.TrimPrefix(line, prefix))
				if strings.HasPrefix(body, "devscope:") {
					directive = strings.TrimSpace(strings.TrimPrefix(body, "devscope:"))
					break
				}
			}
		}
		if directive != "" {
			break
		}
	}

	if directive == "" {
		return []string{proj.Shell}, nil
	}

	parts := strings.Fields(directive)
	if len(parts) == 0 {
		return []string{proj.Shell}, nil
	}

	binName := parts[0]
	resolvedBin, err := resolveInScopedPath(binName, proj, synthBin)
	if err != nil {
		return nil, fmt.Errorf("interpretador %q bloqueado pelo perfil ou inexistente no PATH do projeto %q", binName, proj.Name)
	}

	result := append([]string{resolvedBin}, parts[1:]...)
	return result, nil
}

func resolveInScopedPath(bin string, proj *config.ProjectConfig, synthBin string) (string, error) {
	if filepath.IsAbs(bin) {
		// Caminhos absolutos só são aceitos se estiverem dentro do próprio projeto ou se o nome constar na symlink farm
		if config.IsSubpath(proj.Path, bin) {
			if st, err := os.Stat(bin); err == nil && !st.IsDir() {
				return bin, nil
			}
		}
		bin = filepath.Base(bin)
	}

	// 1. Procura em extra_paths do projeto
	for _, extra := range proj.ExtraPaths {
		dir := extra
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(proj.Path, dir)
		}
		candidate := filepath.Join(dir, bin)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
	}

	// 2. Procura na symlink farm filtrada pela allow/block-list
	candidate := filepath.Join(synthBin, bin)
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		return candidate, nil
	}

	return "", os.ErrNotExist
}
