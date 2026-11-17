package filter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/devscope/devscope/internal/config"
)

// CoreBinaries são mantidos mesmo em perfis allow-list estritos para não quebrar os hooks do shell
var CoreBinaries = []string{
	"sh", "bash", "zsh", "env", "pwd", "realpath", "ls", "cat", "rm", "mkdir", "ln", "tput",
}

// BuildSyntheticPath cria uma symlink farm temporária contendo apenas os binários
// autorizados pelo perfil (allow-list / block-list).
func BuildSyntheticPath(basePath string, prof *config.ProfileConfig) (string, func(), error) {
	runDir := os.Getenv("XDG_RUNTIME_DIR")
	if runDir == "" {
		runDir = os.TempDir()
	}

	tmpRoot, err := os.MkdirTemp(runDir, "devscope-bin-*")
	if err != nil {
		return "", nil, fmt.Errorf("falha ao criar diretório de PATH sintético: %w", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpRoot)
	}

	allowSet := make(map[string]bool)
	for _, b := range prof.Allow {
		allowSet[b] = true
	}
	if len(allowSet) > 0 {
		for _, core := range CoreBinaries {
			allowSet[core] = true
		}
	}

	blockSet := make(map[string]bool)
	for _, b := range prof.Block {
		blockSet[b] = true
	}

	dirs := strings.Split(basePath, ":")
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()

			if blockSet[name] {
				continue
			}
			if len(allowSet) > 0 && !allowSet[name] {
				continue
			}

			destLink := filepath.Join(tmpRoot, name)
			if _, err := os.Lstat(destLink); err == nil {
				continue // Mantém a precedência da primeira ocorrência no PATH
			}

			srcBin := filepath.Join(dir, name)
			_ = os.Symlink(srcBin, destLink)
		}
	}

	return tmpRoot, cleanup, nil
}
