package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// ConfigDir retorna o diretório base ~/.config/devscope
func ConfigDir() (string, error) {
	if custom := os.Getenv("DEVSCOPE_CONFIG_DIR"); custom != "" {
		return custom, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("não foi possível determinar HOME: %w", err)
	}
	return filepath.Join(home, ".config", "devscope"), nil
}

// LoadGlobal carrega ~/.config/devscope/config.toml ou retorna padrões seguros
func LoadGlobal() (*GlobalConfig, error) {
	cfg := &GlobalConfig{
		Defaults: DefaultsConfig{
			Shell: "/bin/bash",
			PassthroughEnv: []string{
				"TERM", "USER", "LOGNAME", "HOME", "LANG", "LC_ALL",
				"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "SSH_AUTH_SOCK",
			},
			BasePath:        "/usr/local/bin:/usr/bin:/bin",
			StrictNamespace: true,
		},
	}

	dir, err := ConfigDir()
	if err != nil {
		return cfg, nil
	}

	cfgPath := filepath.Join(dir, "config.toml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("erro ao ler %s: %w", cfgPath, err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("erro de sintaxe em %s: %w", cfgPath, err)
	}

	if cfg.Defaults.Shell == "" {
		cfg.Defaults.Shell = "/bin/bash"
	}
	if cfg.Defaults.BasePath == "" {
		cfg.Defaults.BasePath = "/usr/local/bin:/usr/bin:/bin"
	}
	return cfg, nil
}

// LoadProject carrega ~/.config/devscope/projects/<name>.toml e resolve caminhos canônicos
func LoadProject(name string, global *GlobalConfig) (*ProjectConfig, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	projPath := filepath.Join(dir, "projects", name+".toml")
	data, err := os.ReadFile(projPath)
	if err != nil {
		return nil, fmt.Errorf("projeto %q não encontrado em %s: %w", name, projPath, err)
	}

	var proj ProjectConfig
	if err := toml.Unmarshal(data, &proj); err != nil {
		return nil, fmt.Errorf("erro ao decodificar manifesto %s: %w", projPath, err)
	}

	proj.Name = name
	if proj.Path == "" {
		return nil, fmt.Errorf("o projeto %q não possui o atributo obrigatório 'path'", name)
	}

	expandedPath := ExpandTilde(proj.Path)
	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("caminho inválido no projeto %q: %w", name, err)
	}

	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, fmt.Errorf("diretório do projeto %q (%s) não existe: %w", name, absPath, err)
	}

	info, err := os.Stat(realPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("caminho do projeto %q (%s) não é um diretório válido", name, realPath)
	}

	proj.Path = realPath
	if proj.Shell == "" {
		proj.Shell = global.Defaults.Shell
	}
	if proj.Env == nil {
		proj.Env = make(map[string]string)
	}

	return &proj, nil
}

// ListProjects lista todos os projetos registrados em ~/.config/devscope/projects/*.toml
func ListProjects(global *GlobalConfig) ([]*ProjectConfig, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(filepath.Join(dir, "projects"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var projects []*ProjectConfig
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".toml")
		proj, err := LoadProject(name, global)
		if err == nil {
			projects = append(projects, proj)
		}
	}
	return projects, nil
}

// FindProjectByPath realiza a descoberta reversa (Longest Prefix Match) para scripts Shebang
func FindProjectByPath(targetPath string, global *GlobalConfig) (*ProjectConfig, error) {
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, err
	}
	realTarget, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		return nil, err
	}

	projects, err := ListProjects(global)
	if err != nil {
		return nil, err
	}

	var bestMatch *ProjectConfig
	bestLen := -1

	for _, proj := range projects {
		if IsSubpath(proj.Path, realTarget) {
			if len(proj.Path) > bestLen {
				bestMatch = proj
				bestLen = len(proj.Path)
			}
		}
	}

	if bestMatch == nil {
		return nil, fmt.Errorf("nenhum projeto registrado contém o caminho %q", realTarget)
	}
	return bestMatch, nil
}

// LoadProfile carrega ~/.config/devscope/profiles/<profile>.toml
func LoadProfile(name string) (*ProfileConfig, error) {
	if name == "" {
		return &ProfileConfig{Name: "default"}, nil
	}

	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}

	profPath := filepath.Join(dir, "profiles", name+".toml")
	data, err := os.ReadFile(profPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &ProfileConfig{Name: name}, nil
		}
		return nil, fmt.Errorf("erro ao carregar perfil %q: %w", name, err)
	}

	var prof ProfileConfig
	if err := toml.Unmarshal(data, &prof); err != nil {
		return nil, fmt.Errorf("erro de sintaxe no perfil %q: %w", profPath, err)
	}
	prof.Name = name
	return &prof, nil
}

// IsSubpath verifica com segurança se child está dentro da árvore de base
func IsSubpath(base, child string) bool {
	base = filepath.Clean(base)
	child = filepath.Clean(child)
	if base == child {
		return true
	}
	prefix := base + string(filepath.Separator)
	return strings.HasPrefix(child, prefix)
}

// ExpandTilde substitui ~/ pelo diretório HOME do usuário atual
func ExpandTilde(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
