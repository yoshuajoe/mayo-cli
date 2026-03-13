package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"mayo-cli/internal/config"

	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export [session_id] [output_file]",
	Short: "Export session results to a file",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		sessionID := args[0]
		outputFile := args[1]

		sessionDir := filepath.Join(config.GetConfigDir(), "sessions", sessionID)
		
		entries, err := os.ReadDir(sessionDir)
		if err != nil {
			fmt.Printf("❌ Error reading session: %v\n", err)
			return
		}

		f, err := os.Create(outputFile)
		if err != nil {
			fmt.Printf("❌ Error creating output file: %v\n", err)
			return
		}
		defer f.Close()

		f.WriteString(fmt.Sprintf("# Research Session Export: %s\n\n", sessionID))

		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
				content, _ := os.ReadFile(filepath.Join(sessionDir, entry.Name()))
				f.Write(content)
				f.WriteString("\n\n---\n\n")
			}
		}

		fmt.Printf("✅ Export complete! Result saved to %s\n", outputFile)
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
}
