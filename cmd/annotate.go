package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"mayo-cli/internal/ui"
	"strings"
)

var annotateCmd = &cobra.Command{
	Use:   "annotate [alias].[table] [description]",
	Short: "Add a custom description/annotation to a table or column",
	Example: `  /annotate p1.users This table contains production user data
  /annotate p1.users.email Primary email for user notifications`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		target := args[0]
		description := strings.Join(args[1:], " ")

		if GlobalOrchestrator == nil {
			ui.PrintError("No active orchestrator. Connect to a database first.")
			return
		}

		parts := strings.Split(target, ".")
		if len(parts) < 2 {
			ui.PrintError("Invalid target format. Use alias.table or alias.table.column")
			return
		}

		alias := parts[0]
		tableName := parts[1]
		columnName := ""
		if len(parts) > 2 {
			columnName = parts[2]
		}

		conn, ok := GlobalOrchestrator.Connections[alias]
		if !ok {
			ui.PrintError(fmt.Sprintf("Alias '%s' not found.", alias))
			return
		}

		if conn.Schema == nil {
			ui.RenderStep("🔍", "Schema not loaded. Syncing...")
			GlobalOrchestrator.SyncSchema(cmd.Context())
		}

		updated := false
		for i := range conn.Schema.Tables {
			if conn.Schema.Tables[i].Name == tableName {
				if columnName == "" {
					conn.Schema.Tables[i].Description = description
					updated = true
				} else {
					for j := range conn.Schema.Tables[i].Columns {
						if conn.Schema.Tables[i].Columns[j].Name == columnName {
							conn.Schema.Tables[i].Columns[j].Description = description
							updated = true
						}
					}
				}
			}
		}

		if updated {
			GlobalOrchestrator.SaveMetadata(alias)
			ui.PrintSuccess(fmt.Sprintf("Annotation saved for %s", target))
		} else {
			ui.PrintError(fmt.Sprintf("Target '%s' not found in schema.", target))
		}
	},
}

func init() {
	rootCmd.AddCommand(annotateCmd)
}
