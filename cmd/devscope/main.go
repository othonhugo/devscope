package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/othonhugo/devscope/internal/config"
	"github.com/othonhugo/devscope/internal/jail"
	"github.com/othonhugo/devscope/internal/runner"
)

func main() {
	// 1. Intercepta bootstrap interno de Mount Namespace antes de qualquer parsing de CLI
	if len(os.Args) > 1 && os.Args[1] == jail.InternalBootstrapArg {
		if err := jail.RunNamespaceBootstrap(); err != nil {
			fmt.Fprintf(os.Stderr, "devscope [erro fatal de namespace]: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		fmt.Fprintf(os.Stderr, "devscope: erro na configuração global: %v\n", err)
		os.Exit(1)
	}

	firstArg := os.Args[1]

	// 2. Comandos utilitários da CLI
	switch firstArg {
	case "-h", "--help", "help":
		printUsage()
		return
	case "-l", "--list", "list":
		listRegisteredProjects(globalCfg)
		return
	}

	// 3. Detecção Automática de Modo Shebang (#!/usr/bin/devscope)
	// Se o primeiro argumento contém '/' ou é um arquivo regular existente no disco e não um subcomando:
	if isShebangInvocation(firstArg, globalCfg) {
		exitCode, err := runner.RunShebangScript(globalCfg, firstArg, os.Args[2:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "devscope: %v\n", err)
		}
		os.Exit(exitCode)
	}

	// 4. Modo Subshell Interativo ou Comando Inline (-c)
	projectName := firstArg
	var inlineCmd string

	if len(os.Args) >= 4 && os.Args[2] == "-c" {
		inlineCmd = strings.Join(os.Args[3:], " ")
	}

	projCfg, err := config.LoadProject(projectName, globalCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "devscope: %v\n", err)
		os.Exit(1)
	}

	exitCode, err := runner.StartInteractiveSession(globalCfg, projCfg, inlineCmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "devscope: %v\n", err)
	}
	os.Exit(exitCode)
}

func isShebangInvocation(arg string, global *config.GlobalConfig) bool {
	if strings.Contains(arg, "/") {
		if st, err := os.Stat(arg); err == nil && !st.IsDir() {
			return true
		}
	}
	// Caso o script tenha sido chamado no diretório atual sem './', verifica se não conflita com nome de projeto
	if _, err := config.LoadProject(arg, global); err == nil {
		return false
	}
	if st, err := os.Stat(arg); err == nil && !st.IsDir() {
		return true
	}
	return false
}

func listRegisteredProjects(global *config.GlobalConfig) {
	projects, err := config.ListProjects(global)
	if err != nil {
		fmt.Fprintf(os.Stderr, "devscope: erro ao listar projetos: %v\n", err)
		os.Exit(1)
	}
	if len(projects) == 0 {
		dir, _ := config.ConfigDir()
		fmt.Printf("Nenhum projeto encontrado em %s/projects/*.toml\n", dir)
		return
	}
	fmt.Println("Projetos registrados no devscope:")
	for _, p := range projects {
		prof := p.Profile
		if prof == "" {
			prof = "default"
		}
		fmt.Printf("  - %-20s -> %s (shell: %s, perfil: %s)\n", p.Name, p.Path, p.Shell, prof)
	}
}

func printUsage() {
	fmt.Println(`Uso:
  devscope <projeto>                  Entra no subshell isolado do projeto (PTY + Mount Namespace)
  devscope <projeto> -c "<comando>"   Executa um comando único dentro do escopo do projeto
  devscope list                       Lista todos os projetos em ~/.config/devscope/projects/
  #!/usr/bin/devscope                 Usa no topo de scripts (suporta # devscope: python3 / node / bash)`)
}
