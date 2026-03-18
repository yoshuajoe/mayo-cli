package cmd

import (
	"fmt"
	"mayo-cli/internal/ui"
	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

func RunWizard() {
	ui.RenderSeparator()
	fmt.Println(ui.StyleTitle.Render("🐶 Welcome to Mayo - Your AI Research Partner!"))
	fmt.Println(ui.StyleMuted.Render("This wizard will guide you through your first setup."))
	ui.RenderSeparator()

	// 1. Core AI & Preferences Setup
	ui.RenderStep("🧠", "Step 1: AI & Preferences")
	RunSetup()

	// 2. Connect first Data Source
	ui.RenderStep("🔌", "Step 2: Connecting your first Data Source")
	var connectNow bool
	survey.AskOne(&survey.Confirm{
		Message: "Would you like to connect a data source now?",
		Default: true,
	}, &connectNow)

	if connectNow {
		var driver string
		survey.AskOne(&survey.Select{
			Message: "Source Type:",
			Options: []string{"file (CSV/XLSX)", "postgres", "mysql", "sqlite"},
		}, &driver)

		dsnMsg := "Connection String (DSN):"
		if driver == "file (CSV/XLSX)" {
			driver = "file"
			dsnMsg = "Path to file:"
		}

		var dsn string
		survey.AskOne(&survey.Input{
			Message: dsnMsg,
			Help:    "e.g., /path/to/data.csv or postgres://user:pass@host:port/db",
		}, &dsn)

		var alias string
		survey.AskOne(&survey.Input{
			Message: "Alias (nickname):",
			Default: "main",
		}, &alias)

		if dsn != "" {
			HandleConnect(driver, dsn, alias, "")
		}
	}

	ui.RenderSeparator()
	ui.PrintSuccess("Onboarding complete! You can now start asking questions like:")
	fmt.Println(ui.StyleHighlight.Render("  - 'Summarize the user demographics from the main database'"))
	fmt.Println(ui.StyleHighlight.Render("  - 'Which products had the highest returns last quarter?'"))
	ui.RenderSeparator()
}

var wizardCmd = &cobra.Command{
	Use:   "wizard",
	Aliases: []string{"init"},
	Short: "Start the onboarding wizard for first-time setup",
	Run: func(cmd *cobra.Command, args []string) {
		RunWizard()
	},
}

func init() {
	rootCmd.AddCommand(wizardCmd)
}
