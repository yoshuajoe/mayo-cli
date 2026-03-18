package cmd

import (
	"fmt"
	"mayo-cli/internal/config"
	"mayo-cli/internal/ui"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

func RunSetup() {
	cfg, _ := config.LoadConfig()
	if cfg == nil {
		cfg = &config.Config{}
	}

	// 1.B: Keyring Setup
	survey.AskOne(&survey.Confirm{
		Message: "Store credentials in system keyring?",
		Default: true,
	}, &cfg.UseKeyring)

	// 1.C: Default Limit Setup
	var useLimit bool
	survey.AskOne(&survey.Confirm{
		Message: "Enable default SQL LIMIT per query (prevent large loads)?",
		Default: true,
	}, &useLimit)

	if useLimit {
		var limitVal int
		survey.AskOne(&survey.Input{
			Message: "Max rows per query:",
			Default: "1000",
		}, &limitVal)
		cfg.DefaultLimit = limitVal
	} else {
		cfg.DefaultLimit = 0 // No limit by default
	}

	// 2.B: Interactive Mode Setup
	survey.AskOne(&survey.Confirm{
		Message: "Enable Interactive Mode (Confirm/Edit SQL before execution)?",
		Default: true,
	}, &cfg.Interactive)

	// Analyst Insight Setup
	survey.AskOne(&survey.Confirm{
		Message: "Enable Analyst Insight (Automatic AI analysis of query results)?",
		Default: true,
	}, &cfg.AnalystEnabled)

	// Teleskop.id Setup
	var setupTeleskop bool
	survey.AskOne(&survey.Confirm{
		Message: "Configure Teleskop.id Scraper?",
		Default: true,
	}, &setupTeleskop)

	if setupTeleskop {
		var teleskopKey string
		survey.AskOne(&survey.Password{
			Message: "Enter Teleskop.id API Key:",
		}, &teleskopKey)
		if teleskopKey != "" {
			cfg.SetTeleskopAPIKey(teleskopKey)
		}
	}

	var profileName string
	err := survey.AskOne(&survey.Input{
		Message: "Enter profile name (e.g., 'work', 'personal', 'gemini-pro'):",
		Default: "default",
	}, &profileName)
	if err != nil {
		ui.PrintInfo("Setup cancelled.")
		return
	}

	var provider string
	err = survey.AskOne(&survey.Select{
		Message: "Choose LLM Provider:",
		Options: config.GetProviderList(),
	}, &provider)
	if err != nil {
		ui.PrintInfo("Setup cancelled.")
		return
	}

	var apiKey string
	err = survey.AskOne(&survey.Password{
		Message: fmt.Sprintf("Enter %s API Key:", provider),
	}, &apiKey)
	if err != nil {
		ui.PrintInfo("Setup cancelled.")
		return
	}

	models := config.GetModelList(provider)
	var selectedModel string
	if len(models) > 0 {
		err = survey.AskOne(&survey.Select{
			Message: "Select Default Model:",
			Options: models,
		}, &selectedModel)
	} else {
		// Fallback defaults if list is empty for some reason
		if provider == "openai" {
			selectedModel = "gpt-4o"
		} else if provider == "groq" {
			selectedModel = "llama-3.3-70b-versatile"
		} else if provider == "anthropic" {
			selectedModel = "claude-3-5-sonnet"
		} else {
			selectedModel = "gemini-2.5-flash"
		}
	}

	if selectedModel == "" {
		ui.PrintInfo("Setup incomplete: model not selected.")
		return
	}
	defaultModel := selectedModel

	// Update or Add profile
	found := false
	for i, p := range cfg.AIProfiles {
		if p.Name == profileName {
			// If provider changed, we might need to update the model
			// but if it's the same, we keep what they had.
			if cfg.AIProfiles[i].Provider != provider {
				cfg.AIProfiles[i].DefaultModel = defaultModel
			} else if cfg.AIProfiles[i].DefaultModel == "" {
				cfg.AIProfiles[i].DefaultModel = defaultModel
			}

			cfg.AIProfiles[i].Provider = provider
			cfg.AIProfiles[i].SetAPIKey(apiKey, cfg.UseKeyring)
			found = true
			break
		}
	}

	if !found {
		p := config.AIProfile{
			Name:         profileName,
			Provider:     provider,
			DefaultModel: defaultModel,
		}
		p.SetAPIKey(apiKey, cfg.UseKeyring)
		cfg.AIProfiles = append(cfg.AIProfiles, p)
	}

	cfg.ActiveAIProfile = profileName

	if err := config.SaveConfig(cfg); err != nil {
		ui.PrintError(fmt.Sprintf("Error saving config: %v", err))
		return
	}

	ui.PrintSuccess(fmt.Sprintf("Profile '%s' saved and set as active!", profileName))
	InitAIClient(cfg)
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure API keys and preferences",
	Run: func(cmd *cobra.Command, args []string) {
		RunSetup()
	},
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
