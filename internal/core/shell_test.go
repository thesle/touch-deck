package core

import "testing"

// TestCmdExeCommandLine checks the raw cmd.exe command line used on Windows.
// It is the only check of the Windows invocation that runs on Linux.
func TestCmdExeCommandLine(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: `echo hi`, want: `cmd.exe /d /s /c "echo hi"`},
		{in: `start "" "https://github.com"`, want: `cmd.exe /d /s /c "start "" "https://github.com""`},
	}
	for _, tc := range tests {
		if got := cmdExeCommandLine(tc.in); got != tc.want {
			t.Errorf("cmdExeCommandLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
