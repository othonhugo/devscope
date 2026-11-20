package config

// GlobalConfig representa ~/.config/devscope/config.toml
type GlobalConfig struct {
	Defaults DefaultsConfig `toml:"defaults"`
}

type DefaultsConfig struct {
	Shell           string   `toml:"shell"`
	PassthroughEnv  []string `toml:"passthrough_env"`
	BasePath        string   `toml:"base_path"`
	StrictNamespace bool     `toml:"strict_namespace"`
	AllowSudo       bool     `toml:"allow_sudo"`
}

// ProjectConfig representa ~/.config/devscope/projects/<project>.toml
type ProjectConfig struct {
	Name        string            `toml:"-"`
	Path        string            `toml:"path"`
	Shell       string            `toml:"shell"`
	Profile     string            `toml:"profile"`
	EnvFile     string            `toml:"env_file"`
	ExtraPaths  []string          `toml:"extra_paths"`
	ExtraMounts []string          `toml:"extra_mounts"`
	Env         map[string]string `toml:"env"`
}

// ProfileConfig representa ~/.config/devscope/profiles/<profile>.toml
type ProfileConfig struct {
	Name  string   `toml:"-"`
	Allow []string `toml:"allow"`
	Block []string `toml:"block"`
}
