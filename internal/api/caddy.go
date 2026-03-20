package api

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"mayo-cli/internal/config"
	"mayo-cli/internal/ui"
)

type CaddyManager struct {
	ConfigDir string
}

func (c *CaddyManager) getBinPath() string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	// Store in ~/.mayo/bin/caddy
	return filepath.Join(c.ConfigDir, "bin", "caddy"+ext)
}

func (c *CaddyManager) getCmdPath() string {
	bin := c.getBinPath()
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	// Fallback to system-wide caddy if portable not found
	path, err := exec.LookPath("caddy")
	if err == nil {
		return path
	}
	return ""
}

func NewCaddyManager() *CaddyManager {
	return &CaddyManager{ConfigDir: config.GetConfigDir()}
}

func (c *CaddyManager) IsInstalled() bool {
	return c.getCmdPath() != ""
}

func (c *CaddyManager) Install() error {
	if c.IsInstalled() {
		// Even if system-wide is installed, maybe they want the portable version?
		// For now, if any caddy exists, we're good.
		return nil
	}

	binPath := c.getBinPath()
	os.MkdirAll(filepath.Dir(binPath), 0755)

	ui.RenderStep("📦", "Downloading portable Caddy server for isolation...")
	
	downloadURL := fmt.Sprintf("https://caddyserver.com/api/download?os=%s&arch=%s", runtime.GOOS, runtime.GOARCH)
	ui.PrintInfo(fmt.Sprintf("Fetching binary from: %s", downloadURL))
	
	// Use curl to download. We could use http.Get, but curl is simpler for progress/handling.
	cmd := exec.Command("curl", "-L", "-o", binPath, downloadURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to download Caddy: %v", err)
	}

	// Make executable
	if err := os.Chmod(binPath, 0755); err != nil {
		return fmt.Errorf("failed to make Caddy executable: %v", err)
	}

	ui.PrintSuccess(fmt.Sprintf("Portable Caddy installed at: %s", binPath))
	return nil
}

func (c *CaddyManager) Setup(domain string, port int) error {
	caddyfilePath := filepath.Join(c.ConfigDir, "Caddyfile")
	
	content := ""
	if domain == "" || domain == ":80" {
		content = fmt.Sprintf(":80 {\n\treverse_proxy localhost:%d\n}\n", port)
	} else {
		content = fmt.Sprintf("%s {\n\treverse_proxy localhost:%d\n}\n", domain, port)
	}

	err := os.WriteFile(caddyfilePath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write Caddyfile: %v", err)
	}

	return nil
}

func (c *CaddyManager) Start() error {
	exe := c.getCmdPath()
	if exe == "" {
		return fmt.Errorf("caddy not found. Please run install first")
	}

	caddyfilePath := filepath.Join(c.ConfigDir, "Caddyfile")
	if _, err := os.Stat(caddyfilePath); os.IsNotExist(err) {
		return fmt.Errorf("Caddyfile not found. Run setup first")
	}

	// Use 'caddy reload' if already running, otherwise 'caddy start'
	// For simplicity, we'll try 'caddy start' and if it fails because it's already running, we try 'caddy reload'
	
	ui.RenderStep("📡", "Starting/Reloading Caddy proxy...")
	ui.PrintInfo("Note: If you use a custom domain, Caddy may take a minute to verify SSL via Let's Encrypt.")
	
	// Use context with timeout to prevent hanging forever (e.g. DNS propagation)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Check if caddy is already running
	checkCmd := exec.Command("pgrep", "caddy")
	if err := checkCmd.Run(); err == nil {
		// Running, so reload
		cmd := exec.CommandContext(ctx, exe, "reload", "--config", caddyfilePath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return c.handleCaddyError(err, string(out), exe, ctx.Err() == context.DeadlineExceeded)
		}
		return nil
	}

	// Not running, so start
	cmd := exec.CommandContext(ctx, exe, "start", "--config", caddyfilePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return c.handleCaddyError(err, string(out), exe, ctx.Err() == context.DeadlineExceeded)
	}
	return nil
}

func (c *CaddyManager) handleCaddyError(err error, output string, exe string, isTimeout bool) error {
	msg := "\n------------------------------------------------------------\n"
	msg += "❌ Caddy Proxy Error\n"
	msg += "------------------------------------------------------------\n\n"

	if isTimeout {
		msg += "Status: TIMEOUT (60s)\n"
		msg += "Reason: Caddy is taking too long to start. Usually due to SSL/DNS verification.\n"
	} else {
		msg += fmt.Sprintf("Status: FAILED (%v)\n", err)
	}

	if output != "" {
		msg += fmt.Sprintf("\nCaptured Output:\n%s\n", ui.StyleMuted.Render(output))
	}

	msg += "\n💡 TROUBLESHOOTING GUIDE:\n"
	msg += "1. DNS Propagation: Verify your domain points directly to this server IP.\n"
	msg += "2. Cloudflare Proxy: If using Cloudflare, turn OFF 'Proxy' (Orange Cloud) until Caddy is up.\n"
	
	if runtime.GOOS == "linux" && strings.Contains(output, "permission denied") {
		msg += "3. Linux Port Restrictions: You must allow this binary to bind to port 80/443:\n"
		msg += fmt.Sprintf("   Run: sudo setcap cap_net_bind_service=+ep %s\n", exe)
	} else if runtime.GOOS == "linux" {
		msg += "3. Firewall: Ensure ports 80 and 443 are open (e.g., sudo ufw allow 80,443/tcp).\n"
	} else {
		msg += "3. Firewall: Ensure port 80 and 443 are open in your server's security group (AWS, GCP, etc).\n"
	}

	msg += "\n------------------------------------------------------------\n"
	return fmt.Errorf(msg)
}

func (c *CaddyManager) Stop() error {
	exe := c.getCmdPath()
	if exe == "" {
		return nil
	}
	cmd := exec.Command(exe, "stop")
	cmd.CombinedOutput() // Usually fine to ignore output of stop
	return nil
}
