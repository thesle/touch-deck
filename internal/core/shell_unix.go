//go:build !windows

package core

import (
	"context"
	"os/exec"
)

// newShellCmd runs command through `bash -c`, the behavior TouchDeck has always
// had on Linux and other Unix systems.
func newShellCmd(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, "bash", "-c", command)
}

// hideConsoleWindow is a no-op outside Windows: there is no console window to
// suppress.
func hideConsoleWindow(cmd *exec.Cmd) {}

// platformDefaultCommands returns the first-run sample button commands for
// Linux/Unix desktops.
func platformDefaultCommands() defaultCommands {
	return defaultCommands{
		Hello:      "notify-send 'TouchDeck' 'Hello! Your TouchDeck is working beautifully.'",
		OpenGitHub: "xdg-open 'https://github.com'",
		Date:       "notify-send 'Current Date' \"$(date)\"",
	}
}
