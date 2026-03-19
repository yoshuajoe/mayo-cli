package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"mayo-cli/internal/ui"
)

// HandleTelegramCommand processes subcommands for /telegram
func HandleTelegramCommand(parts []string) {
	if len(parts) < 2 {
		ui.PrintInfo("Usage:\n  /telegram prepare — setup Caddyfile for HTTPS\n  /telegram exec    — run Caddy server")
		return
	}

	sub := parts[1]
	switch sub {
	case "prepare":
		handleTelegramPrepare()
	case "exec":
		handleTelegramExec()
	default:
		ui.PrintInfo("Usage: /telegram [prepare|exec]")
	}
}

func handleTelegramPrepare() {
	var hasDomain bool
	survey.AskOne(&survey.Confirm{
		Message: "Do you have a domain for this server (required for public HTTPS/Telegram Webhooks)?",
		Default: true,
	}, &hasDomain)

	var domain string
	var port string = "8080" // Default internal port for Mayo Master API

	if hasDomain {
		survey.AskOne(&survey.Input{
			Message: "Enter your domain (e.g., bot.example.com):",
		}, &domain)

		if domain == "" {
			ui.PrintError("Domain is required for automatic HTTPS.")
			return
		}
	} else {
		ui.RenderStep("🔍", "Detecting public IP...")
		// Try to fetch public IP
		resp, err := exec.Command("curl", "-s", "ifconfig.me").Output()
		publicIP := "YOUR_SERVER_IP"
		if err == nil {
			publicIP = strings.TrimSpace(string(resp))
		}
		
		ui.PrintInfo(fmt.Sprintf("Using server IP: %s", publicIP))
		ui.PrintInfo("Note: Telegram Webhooks REQUIRE valid HTTPS (usually via a public domain).")
		domain = ":80" // Bind to port 80 if no domain provided
	}

	survey.AskOne(&survey.Input{
		Message: "Enter target application port (where Mayo is running, default 8080):",
		Default: port,
	}, &port)

	caddyfileContent := ""
	if hasDomain {
		caddyfileContent = fmt.Sprintf("%s {\n\treverse_proxy localhost:%s\n}\n", domain, port)
	} else {
		caddyfileContent = fmt.Sprintf(":80 {\n\treverse_proxy localhost:%s\n}\n", port)
	}

	err := os.WriteFile("Caddyfile", []byte(caddyfileContent), 0644)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Failed to write Caddyfile: %v", err))
		return
	}

	ui.PrintSuccess("Caddyfile generated successfully!")
	
	if hasDomain {
		ui.PrintInfo("Steps to finish your Telegram / HTTPS setup:")
		ui.RenderStep("1️⃣", fmt.Sprintf("Point your domain '%s' (A Record) to this server's public IP.", domain))
		ui.RenderStep("2️⃣", "Ensure ports 80 and 443 are open in your firewall (e.g. AWS Security Group, ufw).")
		ui.RenderStep("3️⃣", "Make sure 'caddy' is installed on your server.")
		ui.RenderStep("4️⃣", "Run '/telegram exec' to start the proxy and obtain SSL certificates automatically.")
		ui.PrintInfo(fmt.Sprintf("Your endpoint will be: https://%s/v1/[session_id]/query", domain))
	} else {
		ui.PrintInfo("Steps to finish setup:")
		ui.RenderStep("1️⃣", "Ensure port 80 is open in your firewall.")
		ui.RenderStep("2️⃣", "Make sure 'caddy' is installed.")
		ui.RenderStep("3️⃣", "Run '/telegram exec' to start.")
		ui.PrintInfo("Note: Endpoint will be accessed via http://[YOUR_SERVER_IP]. Telegram may not work without HTTPS.")
	}
}

func handleTelegramExec() {
	if _, err := os.Stat("Caddyfile"); os.IsNotExist(err) {
		ui.PrintError("Caddyfile not found. Run '/telegram prepare' first.")
		return
	}

	// Check if caddy is in PATH
	_, err := exec.LookPath("caddy")
	if err != nil {
		ui.PrintError("Caddy is not installed or not in PATH.")
		ui.PrintInfo("Please install Caddy first: https://caddyserver.com/docs/install")
		return
	}

	ui.RenderStep("🚀", "Starting Caddy server in the background...")
	
	// 'caddy start' runs it in the background as a daemon
	cmd := exec.Command("caddy", "start", "--config", "Caddyfile", "--adapter", "caddyfile")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		ui.PrintError(fmt.Sprintf("Failed to start Caddy: %v", err))
		return
	}

	ui.PrintSuccess("Caddy is running and proxying traffic!")
	ui.PrintInfo("Check Caddy logs if you encounter issues with SSL certificates.")
}
