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
		Options: []string{"gemini", "openai", "groq", "anthropic"},
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

	defaultModel := ""
	if provider == "openai" {
		defaultModel = "gpt-4o"
	} else if provider == "groq" {
		defaultModel = "llama-3.3-70b-versatile"
	} else if provider == "anthropic" {
		defaultModel = "claude-3-5-sonnet-20241022"
	} else if provider == "gemini" {
		defaultModel = "gemini-1.5-flash"
	}

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
			cfg.AIProfiles[i].APIKey = apiKey
			found = true
			break
		}
	}

	if !found {
		cfg.AIProfiles = append(cfg.AIProfiles, config.AIProfile{
			Name:         profileName,
			Provider:     provider,
			APIKey:       apiKey,
			DefaultModel: defaultModel,
		})
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
