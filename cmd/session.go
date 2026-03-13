package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"mayo-cli/internal/config"
	"mayo-cli/internal/session"

	"github.com/spf13/cobra"
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage research sessions",
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sessions",
	Run: func(cmd *cobra.Command, args []string) {
		sessionsDir := filepath.Join(config.GetConfigDir(), "sessions")
		entries, err := os.ReadDir(sessionsDir)
		if err != nil {
			fmt.Printf("Error reading sessions: %v\n", err)
			return
		}

		fmt.Println("📁 Available Sessions:")
		for _, entry := range entries {
			if entry.IsDir() {
				fmt.Printf("- %s\n", entry.Name())
			}
		}
	},
}

var sessionNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new session",
	Run: func(cmd *cobra.Command, args []string) {
		sess, err := session.NewSession()
		if err != nil {
			fmt.Printf("Error creating session: %v\n", err)
			return
		}
		fmt.Printf("✨ New session created: %s\n", sess.ID)
	},
}

func init() {
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionNewCmd)
	rootCmd.AddCommand(sessionCmd)
}
