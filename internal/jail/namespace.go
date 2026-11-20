package jail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/othonhugo/devscope/internal/config"
)

const InternalBootstrapArg = "__devscope_ns_init"
const PayloadEnvKey = "DEVSCOPE_NS_PAYLOAD"

// NamespacePayload contém os parâmetros para o bootstrap do processo filho dentro do Mount Namespace
type NamespacePayload struct {
	ProjectRoot string   `json:"project_root"`
	ExtraMounts []string `json:"extra_mounts"`
	TargetBin   string   `json:"target_bin"`
	TargetArgs  []string `json:"target_args"`
	CleanEnv    []string `json:"clean_env"`
	StrictNS    bool     `json:"strict_ns"`
}

// ConfigureSysProcAttr prepara os atributos de criação de User + Mount Namespace
// mapeando o UID/GID real do usuário 1:1 (evitando que ferramentas achem que rodam como root).
func ConfigureSysProcAttr(strict bool) *syscall.SysProcAttr {
	if !strict {
		return &syscall.SysProcAttr{}
	}
	uid := os.Getuid()
	gid := os.Getgid()

	return &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: uid, HostID: uid, Size: 1},
		},
		GidMappingsEnableSetgroups: false,
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: gid, HostID: gid, Size: 1},
		},
	}
}

// RunNamespaceBootstrap é executado quando o binário detecta InternalBootstrapArg em os.Args[1].
// Ele já está rodando dentro do novo User e Mount Namespace.
// IMPORTANTE: Nenhum os.RemoveAll é usado; todo o isolamento usa tmpfs em memória que desaparece sozinho.
func RunNamespaceBootstrap() error {
	rawPayload := os.Getenv(PayloadEnvKey)
	if rawPayload == "" {
		return fmt.Errorf("DEVSCOPE_NS_PAYLOAD ausente no bootstrap")
	}

	var payload NamespacePayload
	if err := json.Unmarshal([]byte(rawPayload), &payload); err != nil {
		return fmt.Errorf("payload inválido no bootstrap: %w", err)
	}

	if payload.StrictNS {
		if err := setupIsolatedMounts(&payload); err != nil {
			// Caso o ambiente (ex: container Docker restrito) bloqueie mount, avisa e segue com chdir
			fmt.Fprintf(os.Stderr, "devscope [aviso]: mount namespace parcial (%v); mantendo trava de shell.\n", err)
		}
	}

	if err := os.Chdir(payload.ProjectRoot); err != nil {
		return fmt.Errorf("falha ao entrar no diretório do projeto (%s): %w", payload.ProjectRoot, err)
	}

	return syscall.Exec(payload.TargetBin, payload.TargetArgs, payload.CleanEnv)
}

func setupIsolatedMounts(p *NamespacePayload) error {
	// 1. Torna toda a árvore de montagens privada para jamais propagar nada ao host
	if err := syscall.Mount("", "/", "", syscall.MS_REC|syscall.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("MS_PRIVATE na raiz falhou: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || !config.IsSubpath(homeDir, p.ProjectRoot) {
		// Se o projeto não está em HOME, apenas garante o bind-mount dele mesmo
		return nil
	}

	// 2. Cria um staging em tmpfs para segurar a referência viva ao diretório do projeto e extra_mounts
	stagingBase := "/tmp"
	if err := syscall.Mount("tmpfs", stagingBase, "tmpfs", 0, "mode=0700,size=16m"); err != nil {
		return fmt.Errorf("tmpfs em /tmp falhou: %w", err)
	}

	projHold := filepath.Join(stagingBase, "project_root")
	if err := os.MkdirAll(projHold, 0700); err != nil {
		return err
	}

	// Faz bind-mount recursivo do projeto para o staging antes de ocultar o /home
	if err := syscall.Mount(p.ProjectRoot, projHold, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("bind-mount do projeto para staging falhou: %w", err)
	}

	// Prepara extra_mounts (ex: ~/.cache, ~/.cargo) se existirem dentro de HOME
	type savedMount struct {
		holdPath string
		origPath string
	}
	var savedExtras []savedMount
	for i, extra := range p.ExtraMounts {
		expanded := config.ExpandTilde(extra)
		realExtra, err := filepath.EvalSymlinks(expanded)
		if err != nil || !config.IsSubpath(homeDir, realExtra) {
			continue
		}
		hold := filepath.Join(stagingBase, fmt.Sprintf("extra_%d", i))
		_ = os.MkdirAll(hold, 0700)
		if err := syscall.Mount(realExtra, hold, "", syscall.MS_BIND|syscall.MS_REC, ""); err == nil {
			savedExtras = append(savedExtras, savedMount{holdPath: hold, origPath: realExtra})
		}
	}

	// 3. Oculta todo o diretório HOME do usuário com um tmpfs limpo em memória!
	// Isso faz com que qualquer 'cd ..' ou 'ls /home/user' veja apenas a árvore vazia até o projeto.
	if err := syscall.Mount("tmpfs", homeDir, "tmpfs", 0, "mode=0755,size=32m"); err != nil {
		return fmt.Errorf("ocultação de HOME com tmpfs falhou: %w", err)
	}

	// 4. Recria o caminho esqueleto dentro do tmpfs de HOME e devolve o bind-mount do projeto
	if err := os.MkdirAll(p.ProjectRoot, 0755); err != nil {
		return fmt.Errorf("recriação do caminho do projeto no tmpfs falhou: %w", err)
	}
	if err := syscall.Mount(projHold, p.ProjectRoot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("restauração do bind-mount do projeto falhou: %w", err)
	}

	// Restaura os extra_mounts autorizados
	for _, sm := range savedExtras {
		_ = os.MkdirAll(sm.origPath, 0755)
		_ = syscall.Mount(sm.holdPath, sm.origPath, "", syscall.MS_BIND|syscall.MS_REC, "")
	}

	// 5. Desmonta os pontos de staging com MNT_DETACH e limpa /tmp com um novo tmpfs vazio
	_ = syscall.Unmount(projHold, syscall.MNT_DETACH)
	for _, sm := range savedExtras {
		_ = syscall.Unmount(sm.holdPath, syscall.MNT_DETACH)
	}
	_ = syscall.Mount("tmpfs", stagingBase, "tmpfs", 0, "mode=1777,size=64m")

	return nil
}
