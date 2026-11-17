package env

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devscope/devscope/internal/config"
)

// BuildCleanEnv cria um ambiente do zero (estilo env -i), preservando apenas
// passthrough_env e injetando as variáveis do projeto e o PATH controlado.
func BuildCleanEnv(global *config.GlobalConfig, proj *config.ProjectConfig, syntheticBinDir string) ([]string, error) {
	envMap := make(map[string]string)

	// 1. Copia apenas as variáveis essenciais autorizadas da sessão hospedeira
	for _, key := range global.Defaults.PassthroughEnv {
		if val, ok := os.LookupEnv(key); ok {
			envMap[key] = val
		}
	}

	// Garante valores mínimos de fallback
	if _, ok := envMap["TERM"]; !ok {
		envMap["TERM"] = "xterm-256color"
	}

	// 2. Carrega arquivo .env local do projeto (se especificado)
	if proj.EnvFile != "" {
		envFilePath := proj.EnvFile
		if !filepath.IsAbs(envFilePath) {
			envFilePath = filepath.Join(proj.Path, envFilePath)
		}
		fileVars, err := parseDotEnv(envFilePath)
		if err != nil {
			return nil, fmt.Errorf("erro ao ler env_file (%s): %w", envFilePath, err)
		}
		for k, v := range fileVars {
			envMap[k] = v
		}
	}

	// 3. Aplica variáveis explícitas declaradas no manifesto [env]
	for k, v := range proj.Env {
		envMap[k] = v
	}

	// 4. Constrói PATH rígido: diretórios internos do projeto + symlink farm filtrada
	var pathParts []string
	for _, extra := range proj.ExtraPaths {
		var resolved string
		if filepath.IsAbs(extra) {
			resolved = filepath.Clean(extra)
		} else {
			resolved = filepath.Join(proj.Path, extra)
		}
		// Apenas caminhos dentro da árvore do projeto são aceitos em extra_paths
		if config.IsSubpath(proj.Path, resolved) {
			pathParts = append(pathParts, resolved)
		}
	}
	if syntheticBinDir != "" {
		pathParts = append(pathParts, syntheticBinDir)
	} else {
		pathParts = append(pathParts, global.Defaults.BasePath)
	}
	envMap["PATH"] = strings.Join(pathParts, ":")

	// 5. Metadados de escopo do devscope
	envMap["DEVSCOPE_ACTIVE"] = "1"
	envMap["DEVSCOPE_PROJECT"] = proj.Name
	envMap["DEVSCOPE_ROOT"] = proj.Path
	envMap["PWD"] = proj.Path

	result := make([]string, 0, len(envMap))
	for k, v := range envMap {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result, nil
}

func parseDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)
		vars[key] = val
	}
	return vars, scanner.Err()
}
