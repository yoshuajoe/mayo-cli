package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"mayo-cli/internal/config"

	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history [session_id]",
	Short: "View session history",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		sessionID := args[0]
		sessionDir := filepath.Join(config.GetConfigDir(), "sessions", sessionID)
		
		entries, err := os.ReadDir(sessionDir)
		if err != nil {
			fmt.Printf("Error reading session history: %v\n", err)
			return
		}

		fmt.Printf("📜 History for Session %s:\n", sessionID)
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
				fmt.Printf("\n--- %s ---\n", entry.Name())
				content, _ := os.ReadFile(filepath.Join(sessionDir, entry.Name()))
				fmt.Println(string(content))
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(historyCmd)
}
