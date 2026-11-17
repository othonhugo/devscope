package pty

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// RunInteractive inicia o comando acoplado a um Pseudoterminal (PTY), coloca o terminal
// hospedeiro em Raw Mode (garantindo restauração via defer) e propaga mudanças de janela (SIGWINCH).
func RunInteractive(cmd *exec.Cmd) (int, error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return 1, err
	}
	defer func() { _ = ptmx.Close() }()

	// Sincroniza o tamanho da janela e monitora SIGWINCH
	winchCh := make(chan os.Signal, 1)
	signal.Notify(winchCh, syscall.SIGWINCH)
	go func() {
		for range winchCh {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	winchCh <- syscall.SIGWINCH // Dispara ajuste inicial imediato
	defer func() {
		signal.Stop(winchCh)
		close(winchCh)
	}()

	// Coloca o terminal hospedeiro em Raw Mode se os.Stdin for um TTY real
	stdinFd := int(os.Stdin.Fd())
	if term.IsTerminal(stdinFd) {
		oldState, err := term.MakeRaw(stdinFd)
		if err == nil {
			defer func() { _ = term.Restore(stdinFd, oldState) }()
		}
	}

	// Proxy bidirecional entre o terminal hospedeiro e o mestre PTY
	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
	}()
	_, _ = io.Copy(os.Stdout, ptmx)

	// Aguarda encerramento do processo filho e extrai código de saída real
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}
