package cmd

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"mayo-cli/internal/api"
	"mayo-cli/internal/config"
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
		cfg, _ := config.LoadConfig()
		if cfg != nil && len(cfg.KnownDomains) > 0 {
			options := []string{}
			for _, d := range cfg.KnownDomains {
				options = append(options, fmt.Sprintf("(current: %s)", d))
			}
			options = append(options, "Set up new domain")

			var selection string
			survey.AskOne(&survey.Select{
				Message: "Select domain for Telegram:",
				Options: options,
			}, &selection)

			if selection == "Set up new domain" {
				survey.AskOne(&survey.Input{Message: "Enter your domain (e.g., bot.example.com):"}, &domain)
			} else {
				domain = strings.TrimPrefix(selection, "(current: ")
				domain = strings.TrimSuffix(domain, ")")
			}
		} else {
			survey.AskOne(&survey.Input{
				Message: "Enter your domain (e.g., bot.example.com):",
			}, &domain)
		}

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

	portInt, _ := strconv.Atoi(port)
	caddyMgr := api.NewCaddyManager()
	
	// Check if already installed
	if !caddyMgr.IsInstalled() {
		var confirm bool
		survey.AskOne(&survey.Confirm{
			Message: "Caddy is not installed. Would you like to install it now?",
			Default: true,
		}, &confirm)
		if confirm {
			if err := caddyMgr.Install(); err != nil {
				ui.PrintError(err.Error())
				return
			}
		} else {
			ui.PrintError("Caddy installation required for this command.")
			return
		}
	}

	err := caddyMgr.Setup(domain, portInt)
	if err != nil {
		ui.PrintError(fmt.Sprintf("Failed to setup Caddy: %v", err))
		return
	}

	// Save domain to known domains if successful
	cfg, _ := config.LoadConfig()
	if domain != "" && !strings.HasPrefix(domain, ":") && cfg != nil {
		found := false
		for _, kd := range cfg.KnownDomains {
			if kd == domain {
				found = true
				break
			}
		}
		if !found {
			cfg.KnownDomains = append(cfg.KnownDomains, domain)
			config.SaveConfig(cfg)
		}
	}

	ui.PrintSuccess("Caddyfile generated successfully!")
	
	if hasDomain {
		ui.PrintInfo("Steps to finish your Telegram / HTTPS setup:")
		ui.RenderStep("1️⃣", fmt.Sprintf("Point your domain '%s' (A Record) to this server's public IP.", domain))
		ui.RenderStep("2️⃣", "Ensure ports 80 and 443 are open in your firewall (e.g. AWS Security Group, ufw).")
		ui.RenderStep("3️⃣", "Run '/telegram exec' to start the proxy and obtain SSL certificates automatically.")
		ui.PrintInfo(fmt.Sprintf("Your endpoint will be: https://%s/v1/[session_id]/query", domain))
	} else {
		ui.PrintInfo("Steps to finish setup:")
		ui.RenderStep("1️⃣", "Ensure port 80 is open in your firewall.")
		ui.RenderStep("2️⃣", "Run '/telegram exec' to start.")
		ui.PrintInfo("Note: Endpoint will be accessed via http://[YOUR_SERVER_IP]. Telegram may not work without HTTPS.")
	}
}

func handleTelegramExec() {
	caddyMgr := api.NewCaddyManager()
	if !caddyMgr.IsInstalled() {
		ui.PrintError("Caddy is not installed.")
		return
	}

	if err := caddyMgr.Start(); err != nil {
		ui.PrintError(fmt.Sprintf("Failed to start Caddy: %v", err))
		return
	}

	ui.PrintSuccess("Caddy is running and proxying traffic!")
	ui.PrintInfo("Check Caddy logs if you encounter issues with SSL certificates.")
}
