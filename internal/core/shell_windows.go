package core

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

// createNoWindow is the CREATE_NO_WINDOW process creation flag: the child
// console process (cmd.exe) runs without a console window.
const createNoWindow = 0x08000000

// newShellCmd runs command through cmd.exe (%ComSpec%, falling back to
// cmd.exe). The command line is set raw through SysProcAttr.CmdLine because
// Go's default argument quoting follows CommandLineToArgvW rules, which cmd.exe
// does not use (see `go doc os/exec.Command`).
func newShellCmd(ctx context.Context, command string) *exec.Cmd {
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = "cmd.exe"
	}
	cmd := exec.CommandContext(ctx, comspec)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: cmdExeCommandLine(command)}
	return cmd
}

// hideConsoleWindow stops cmd.exe from flashing a console window. It sets only
// CREATE_NO_WINDOW and deliberately not HideWindow: HideWindow passes
// STARTF_USESHOWWINDOW+SW_HIDE to the child, and cmd.exe can hand that on to a
// GUI program it launches directly, so a button running e.g. `notepad` could
// open hidden. Output pipes (RunCommandSync) still work with CREATE_NO_WINDOW.
func hideConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

// platformDefaultCommands returns the first-run sample button commands for
// Windows. cmd.exe has no message-box builtin and msg.exe is missing on Home
// editions, so the two notification buttons call PowerShell explicitly.
func platformDefaultCommands() defaultCommands {
	return defaultCommands{
		Hello:      `powershell -NoProfile -Command "Add-Type -AssemblyName PresentationFramework; [void][System.Windows.MessageBox]::Show('Hello! Your TouchDeck is working beautifully.', 'TouchDeck')"`,
		OpenGitHub: `start "" "https://github.com"`,
		Date:       `powershell -NoProfile -Command "Add-Type -AssemblyName PresentationFramework; [void][System.Windows.MessageBox]::Show((Get-Date).ToString(), 'Current Date')"`,
	}
}
