package ui

import (
	"os"
	"os/exec"
	"runtime"
)

// OpenEditor opens the given file in the user's default editor (vim, nano, or $EDITOR)
func OpenEditor(filename string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vim"
		}
	}

	cmd := exec.Command(editor, filename)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// OpenFileWithDefault opens a file with the system default application (open on macOS, xdg-open on Linux)
func OpenFileWithDefault(filename string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", filename)
	case "linux":
		cmd = exec.Command("xdg-open", filename)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", filename)
	default:
		return nil
	}
	return cmd.Start()
}
