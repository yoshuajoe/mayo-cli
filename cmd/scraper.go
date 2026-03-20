package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"mayo-cli/internal/config"
	"mayo-cli/internal/teleskop"
	"mayo-cli/internal/ui"

	"github.com/spf13/cobra"
)

var scraperCmd = &cobra.Command{
	Use:   "scraper",
	Short: "Manage Teleskop.id scrapers",
}

var spawnCmd = &cobra.Command{
	Use:   "spawn [keyword]",
	Short: "Spawn a new background scraper",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.LoadConfig()
		apiKey, err := cfg.GetTeleskopAPIKey()
		if err != nil || apiKey == "" {
			if err != nil {
				ui.PrintError(fmt.Sprintf("Keyring error: %v", err))
			} else {
				ui.PrintError("Teleskop.id API Key not configured. Run /setup first.")
			}
			return
		}

		keyword := args[0]
		interval, _ := cmd.Flags().GetInt("interval")
		maxPage, _ := cmd.Flags().GetInt("max-page")
		lang, _ := cmd.Flags().GetString("lang")

		client := teleskop.NewClient(apiKey)
		config := teleskop.ScraperConfig{
			Keyword:  keyword,
			Interval: interval,
			MaxPage:  maxPage,
			Language: lang,
		}

		id, err := client.SpawnScraper(context.Background(), config)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to spawn scraper: %v", err))
			return
		}

		manager := teleskop.NewManager()
		status := &teleskop.ScraperStatus{
			ID:        id,
			Status:    "running",
			StartTime: time.Now(),
		}
		manager.RegisterScraper(status)
		manager.Save()

		ui.PrintSuccess(fmt.Sprintf("Scraper spawned successfully! ID: %s", id))
	},
}

var listScrapersCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active scrapers",
	Run: func(cmd *cobra.Command, args []string) {
		manager := teleskop.NewManager()
		if len(manager.Scrapers) == 0 {
			ui.PrintInfo("No scrapers found.")
			return
		}

		ui.PrintInfo("Active Scrapers Status Summary:")
		headers := []string{"ID", "Status", "Started", "200", "!200"}
		var rows [][]string

		cfg, _ := config.LoadConfig()
		apiKey, _ := cfg.GetTeleskopAPIKey()
		client := teleskop.NewClient(apiKey)

		for id, s := range manager.Scrapers {
			// Fetch fresh status for each
			status, _ := client.GetScraperStatus(context.Background(), id)

			rows = append(rows, []string{
				id,
				s.Status,
				s.StartTime.Format("01-02 15:04"),
				fmt.Sprintf("%d", status.Requests200),
				fmt.Sprintf("%d", status.RequestsNon200),
			})
		}
		ui.RenderTable(headers, rows)
	},
}

var logsCmd = &cobra.Command{
	Use:   "logs [id]",
	Short: "View scraper activity logs",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		cfg, _ := config.LoadConfig()
		apiKey, _ := cfg.GetTeleskopAPIKey()
		client := teleskop.NewClient(apiKey)

		logs, err := client.GetLogs(context.Background(), id)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to get logs: %v", err))
			return
		}

		ui.PrintInfo(fmt.Sprintf("Scraper Activity Logs [%s]:", id))
		for _, log := range logs {
			fmt.Println(log)
		}
	},
}

var statusCmd = &cobra.Command{
	Use:   "status [id]",
	Short: "Check scraper status and logs",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		cfg, _ := config.LoadConfig()
		apiKey, _ := cfg.GetTeleskopAPIKey()
		client := teleskop.NewClient(apiKey)

		status, err := client.GetScraperStatus(context.Background(), id)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to get status: %v", err))
			return
		}

		ui.PrintInfo(fmt.Sprintf("Scraper Status [%s]:", id))
		fmt.Printf("Status: %s\n", status.Status)
		fmt.Printf("Requests: 200: %d | Non-200: %d\n", status.Requests200, status.RequestsNon200)
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop [id]",
	Short: "Stop a running scraper",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		cfg, _ := config.LoadConfig()
		apiKey, _ := cfg.GetTeleskopAPIKey()
		client := teleskop.NewClient(apiKey)

		err := client.StopScraper(context.Background(), id)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to stop scraper: %v", err))
			return
		}

		manager := teleskop.NewManager()
		if s, ok := manager.Scrapers[id]; ok {
			s.Status = "stopped"
			manager.Save()
		}

		ui.PrintSuccess(fmt.Sprintf("Scraper %s stopped.", id))
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a scraper and its data",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		cfg, _ := config.LoadConfig()
		apiKey, _ := cfg.GetTeleskopAPIKey()
		client := teleskop.NewClient(apiKey)

		err := client.DeleteScraper(context.Background(), id)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to delete scraper: %v", err))
			return
		}

		manager := teleskop.NewManager()
		manager.RemoveScraper(id)
		manager.Save()

		// Delete local database
		dbPath := teleskop.GetScraperDBPath(id)
		os.Remove(dbPath)

		ui.PrintSuccess(fmt.Sprintf("Scraper %s and its data deleted.", id))
	},
}

var usageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Check Teleskop.id API usage",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.LoadConfig()
		apiKey, _ := cfg.GetTeleskopAPIKey()
		client := teleskop.NewClient(apiKey)

		usage, err := client.GetUsage(context.Background())
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to get usage: %v", err))
			return
		}

		ui.PrintInfo("Teleskop.id API Usage:")
		fmt.Printf("Requests: 200: %d | Non-200: %d | Total: %d\n", usage.Requests200, usage.RequestsNon200, usage.TotalRequests)
	},
}

var headScraperCmd = &cobra.Command{
	Use:   "head [id]",
	Short: "Show sample data from scraper",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		n, _ := cmd.Flags().GetInt("n")
		cols, rows, err := teleskop.GetHead(id, n)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to get data: %v", err))
			return
		}
		ui.RenderTable(cols, rows)
	},
}

var tailScraperCmd = &cobra.Command{
	Use:   "tail [id]",
	Short: "Show last scraped data",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		n, _ := cmd.Flags().GetInt("n")
		cols, rows, err := teleskop.GetTail(id, n)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to get data: %v", err))
			return
		}
		ui.RenderTable(cols, rows)
	},
}

var summaryScraperCmd = &cobra.Command{
	Use:   "summary [id]",
	Short: "Show scraper summary",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		summary, err := teleskop.GetSummary(id)
		if err != nil {
			ui.PrintError(fmt.Sprintf("Failed to get summary: %v", err))
			return
		}
		ui.PrintInfo(summary)
	},
}

func init() {
	spawnCmd.Flags().IntP("interval", "i", 60, "Interval in minutes")
	spawnCmd.Flags().IntP("max-page", "m", 10, "Maximum pages to scrape")
	spawnCmd.Flags().StringP("lang", "l", "id", "Language (id/en/etc)")
	headScraperCmd.Flags().IntP("n", "n", 5, "Number of rows to show")
	tailScraperCmd.Flags().IntP("n", "n", 5, "Number of rows to show")

	scraperCmd.AddCommand(spawnCmd, listScrapersCmd, statusCmd, logsCmd, stopCmd, deleteCmd, usageCmd, headScraperCmd, tailScraperCmd, summaryScraperCmd)
	rootCmd.AddCommand(scraperCmd)
}
