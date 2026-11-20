package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/othonhugo/devscope/internal/config"
	"github.com/othonhugo/devscope/internal/env"
	"github.com/othonhugo/devscope/internal/filter"
	"github.com/othonhugo/devscope/internal/jail"
	"github.com/othonhugo/devscope/internal/pty"
)

// StartInteractiveSession prepara o ambiente limpo, o PATH sintético, os hooks do shell
// e inicia o subshell dentro do Mount Namespace acoplado ao PTY.
func StartInteractiveSession(global *config.GlobalConfig, proj *config.ProjectConfig, inlineCmd string) (int, error) {
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

	var targetBin string
	var targetArgs []string
	var cleanupShell func()

	if inlineCmd != "" {
		targetBin = proj.Shell
		targetArgs = []string{proj.Shell, "-c", inlineCmd}
		cleanupShell = func() {}
	} else {
		var extraEnv []string
		var shellArgs []string
		targetBin, shellArgs, extraEnv, cleanupShell, err = jail.PrepareShellInit(proj)
		if err != nil {
			return 1, err
		}
		targetArgs = append([]string{targetBin}, shellArgs...)
		cleanEnv = append(cleanEnv, extraEnv...)
	}
	defer cleanupShell()

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

	if inlineCmd != "" {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode(), nil
			}
			return 1, err
		}
		return 0, nil
	}

	return pty.RunInteractive(cmd)
}
