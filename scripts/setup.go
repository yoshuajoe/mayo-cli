package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	fmt.Println("🐶 Mayo - Cross-Platform Setup Utility")
	fmt.Println("---------------------------------------")

	// 1. Check Go
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Println("❌ Error: Go binary not found. Please install Go first.")
		os.Exit(1)
	}

	// 2. Build
	fmt.Println("🔨 Building Mayo...")
	binName := "mayo"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	buildCmd := exec.Command("go", "build", "-o", filepath.Join("bin", binName), "main.go")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Printf("❌ Build failed: %v\n", err)
		os.Exit(1)
	}

	// 3. Get GOPATH/bin
	goEnvCmd := exec.Command("go", "env", "GOPATH")
	out, err := goEnvCmd.Output()
	if err != nil {
		fmt.Printf("❌ Could not determine GOPATH: %v\n", err)
		os.Exit(1)
	}
	goPath := strings.TrimSpace(string(out))
	goBin := filepath.Join(goPath, "bin")

	// 4. Install
	fmt.Printf("🚀 Installing to: %s\n", goBin)
	os.MkdirAll(goBin, 0755)

	src := filepath.Join("bin", binName)
	dst := filepath.Join(goBin, binName)

	input, _ := os.ReadFile(src)
	err = os.WriteFile(dst, input, 0755)
	if err != nil {
		fmt.Printf("❌ Permission error or failed to copy: %v\n", err)
		fmt.Println("Tip: Try running with sudo (Linux/Mac) or as Administrator (Windows).")
		os.Exit(1)
	}

	// 5. Config Dirs
	home, _ := os.UserHomeDir()
	configRoot := filepath.Join(home, ".mayo-cli")
	os.MkdirAll(filepath.Join(configRoot, "sessions"), 0755)
	os.MkdirAll(filepath.Join(configRoot, "data"), 0755)

	fmt.Println("✅ Installation complete!")

	// 6. Automated PATH Setup
	fmt.Println("\n🛠️  Setting up system PATH...")
	if runtime.GOOS == "windows" {
		psCmd := fmt.Sprintf(`$CurrentPath = [Environment]::GetEnvironmentVariable("Path", "User"); if ($CurrentPath -notlike "*%s*") { [Environment]::SetEnvironmentVariable("Path", $CurrentPath + ";%s", "User"); Write-Host "✅ Added to Windows PATH." } else { Write-Host "ℹ️ Already in PATH." }`, goBin, goBin)
		exec.Command("powershell", "-Command", psCmd).Run()
	} else {
		// Unix PATH Setup
		shell := os.Getenv("SHELL")
		profile := ""
		if strings.Contains(shell, "zsh") {
			profile = filepath.Join(home, ".zshrc")
		} else if strings.Contains(shell, "bash") {
			profile = filepath.Join(home, ".bashrc")
			if runtime.GOOS == "darwin" {
				profile = filepath.Join(home, ".bash_profile")
			}
		}

		if profile != "" {
			content, err := os.ReadFile(profile)
			pathExport := fmt.Sprintf("\n# Added by Mayo\nexport PATH=\"$PATH:%s\"\n", goBin)
			if err == nil && !strings.Contains(string(content), goBin) {
				f, _ := os.OpenFile(profile, os.O_APPEND|os.O_WRONLY, 0644)
				f.WriteString(pathExport)
				f.Close()
				fmt.Printf("✅ Added to %s\n", profile)
				fmt.Printf("👉 Please run: source %s\n", profile)
			} else {
				fmt.Println("ℹ️ PATH already configured or profile not found.")
			}
		}
	}

	fmt.Println("\n--- FINAL STEP ---")
	if runtime.GOOS == "windows" {
		fmt.Println("Please RESTART your terminal/PowerShell to use 'mayo'.")
	} else {
		fmt.Println("Please RESTART your terminal or 'source' your profile to use 'mayo'.")
	}
}
