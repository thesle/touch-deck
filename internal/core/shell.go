package core

import (
	"context"
	"os/exec"
)

// This file holds the platform-neutral half of the shell helper used by the
// Command_Runner. The platform-specific halves live in shell_unix.go
// (bash -c, unchanged Linux behavior) and shell_windows.go (cmd.exe, no
// console window).

// defaultCommands are the commands of the three sample buttons written to a
// fresh config on first run. They differ per platform because the Linux
// defaults rely on notify-send, xdg-open and bash command substitution.
type defaultCommands struct {
	Hello      string
	OpenGitHub string
	Date       string
}

// shellCommand builds an *exec.Cmd that runs command through the platform
// shell (`bash -c` on Linux/Unix, `cmd.exe /d /s /c` on Windows) and, on
// Windows, stops the shell from opening a console window. The command is not
// started.
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := newShellCmd(ctx, command)
	hideConsoleWindow(cmd)
	return cmd
}

// cmdExeCommandLine returns the raw Windows command line used to run command
// through cmd.exe. With /s and the outer quotes, cmd.exe strips exactly that
// outer pair and runs the rest verbatim, so quotes inside command are kept as
// written. /d skips the registry AutoRun scripts. It is a pure function kept
// free of build tags so it can be unit-tested on any platform.
func cmdExeCommandLine(command string) string {
	return `cmd.exe /d /s /c "` + command + `"`
}
