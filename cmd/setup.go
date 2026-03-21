package cmd

import (
	"fmt"
	"mayo-cli/internal/config"
	"mayo-cli/internal/ui"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

func RunSetup() {
	cfg, _ := config.LoadConfig()
	if cfg == nil {
		cfg = &config.Config{PrivacyMode: true, AnalystEnabled: true, DefaultLimit: 1000}
	}

	// 1. Keyring Setup
	if err := config.CheckKeyringHealth(); err != nil {
		ui.PrintError(fmt.Sprintf("System keyring check failed: %v", err))
		ui.PrintInfo("Your system may not support system keyring (common in headless/remote environments).")
		ui.PrintInfo("We will default to storing credentials in plain-text config.json for now.")
		cfg.UseKeyring = false
	} else {
		survey.AskOne(&survey.Confirm{
			Message: "Store credentials in system keyring?",
			Default: cfg.UseKeyring,
		}, &cfg.UseKeyring)
	}

	// 2. Default Limit Setup
	var useLimit bool = cfg.DefaultLimit > 0
	survey.AskOne(&survey.Confirm{
		Message: "Enable default SQL LIMIT per query (prevent large loads)?",
		Default: useLimit,
	}, &useLimit)

	if useLimit {
		defaultLimit := "1000"
		if cfg.DefaultLimit > 0 {
			defaultLimit = strconv.Itoa(cfg.DefaultLimit)
		}
		var limitVal int
		survey.AskOne(&survey.Input{
			Message: "Max rows per query:",
			Default: defaultLimit,
		}, func(ans interface{}) error {
			if a, ok := ans.(string); ok {
				v, err := strconv.Atoi(a)
				limitVal = v
				return err
			}
			return nil
		})
		cfg.DefaultLimit = limitVal
	} else {
		cfg.DefaultLimit = 0
	}

	// 3. Interactive Mode Setup
	survey.AskOne(&survey.Confirm{
		Message: "Enable Interactive Mode (Confirm/Edit SQL before execution)?",
		Default: cfg.Interactive,
	}, &cfg.Interactive)

	// 4. Analyst Insight Setup
	survey.AskOne(&survey.Confirm{
		Message: "Enable Analyst Insight (Automatic AI analysis of query results)?",
		Default: cfg.AnalystEnabled,
	}, &cfg.AnalystEnabled)

	// 5. Teleskop.id Setup
	tk, _ := cfg.GetTeleskopAPIKey()
	var setupTeleskop bool = tk != ""
	survey.AskOne(&survey.Confirm{
		Message: "Configure Teleskop.id Scraper?",
		Default: setupTeleskop,
	}, &setupTeleskop)

	if setupTeleskop {
		var teleskopKey string
		survey.AskOne(&survey.Password{
			Message: "Enter Teleskop.id API Key (Keep empty to stay same):",
		}, &teleskopKey)
		if teleskopKey != "" {
			cfg.SetTeleskopAPIKey(teleskopKey)
		}
	}

	// 6. REST API Security Setup
	var setupAPI bool = cfg.ServeToken != ""
	survey.AskOne(&survey.Confirm{
		Message: "Configure Mayo API Security (Token)?",
		Default: setupAPI,
	}, &setupAPI)

	if setupAPI {
		var token string
		survey.AskOne(&survey.Password{
			Message: "Enter Secret API Token (Bearer Auth):",
		}, &token)
		if token != "" {
			cfg.ServeToken = token
		}
	}

	// 7. AI Profile Setup
	var profileName string
	var selection string
	profileOptions := []string{"[+] New Profile"}
	for _, p := range cfg.AIProfiles {
		profileOptions = append(profileOptions, p.Name)
	}

	defaultSelection := "[+] New Profile"
	if cfg.ActiveAIProfile != "" {
		defaultSelection = cfg.ActiveAIProfile
	}

	err := survey.AskOne(&survey.Select{
		Message: "Select AI Profile to configure:",
		Options: profileOptions,
		Default: defaultSelection,
	}, &selection)
	if err != nil {
		ui.PrintInfo("Setup cancelled.")
		return
	}

	if selection == "[+] New Profile" {
		err = survey.AskOne(&survey.Input{
			Message: "Enter name for new profile:",
		}, &profileName)
		if err != nil || profileName == "" {
			ui.PrintInfo("Setup cancelled.")
			return
		}
	} else {
		profileName = selection
	}
	if err != nil {
		ui.PrintInfo("Setup cancelled.")
		return
	}

	// Find existing profile if any
	var existing *config.AIProfile
	for i, p := range cfg.AIProfiles {
		if p.Name == profileName {
			existing = &cfg.AIProfiles[i]
			break
		}
	}

	var provider string
	options := config.GetProviderList()
	if existing != nil {
		// Add "Keep Current" option for select
		keepLabel := fmt.Sprintf("[Keep Current: %s]", existing.Provider)
		options = append([]string{keepLabel}, options...)
	}

	err = survey.AskOne(&survey.Select{
		Message: "Choose LLM Provider:",
		Options: options,
	}, &provider)
	if err != nil {
		ui.PrintInfo("Setup cancelled.")
		return
	}

	if strings.HasPrefix(provider, "[Keep Current") && existing != nil {
		provider = existing.Provider
		// Skip API key as well if keeping provider
	} else {
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
		}

		if !foundProfileUpdate(cfg, profileName, provider, apiKey, selectedModel) {
			p := config.AIProfile{
				Name:         profileName,
				Provider:     provider,
				DefaultModel: selectedModel,
			}
			p.SetAPIKey(apiKey, cfg.UseKeyring)
			cfg.AIProfiles = append(cfg.AIProfiles, p)
		}
	}

	cfg.ActiveAIProfile = profileName
	if err := config.SaveConfig(cfg); err != nil {
		ui.PrintError(fmt.Sprintf("Error saving config: %v", err))
		return
	}

	ui.PrintSuccess(fmt.Sprintf("Configuration saved! Profile '%s' is now active.", profileName))
	InitAIClient(cfg)
}

func foundProfileUpdate(cfg *config.Config, name, provider, key, model string) bool {
	for i, p := range cfg.AIProfiles {
		if p.Name == name {
			cfg.AIProfiles[i].Provider = provider
			if key != "" {
				cfg.AIProfiles[i].SetAPIKey(key, cfg.UseKeyring)
			}
			if model != "" {
				cfg.AIProfiles[i].DefaultModel = model
			}
			return true
		}
	}
	return false
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
