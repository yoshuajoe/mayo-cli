package teleskop

import (
	"fmt"
	"time"
)

// BackgroundWorker handles the background scraping logic.
// In a real implementation, this could be a separate binary or a detached process.
func BackgroundWorker(id string, config ScraperConfig) {
	fmt.Printf("Starting background worker for %s...\n", id)
	
	// Ensure the storage is ready
	InitializeScraperTable(id)

	ticker := time.NewTicker(time.Duration(config.Interval) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Perform scraping logic
			fmt.Printf("[%s] Scraping for: %s\n", id, config.Keyword)
			// TODO: Call actual API and save data to SQLite
		}
	}
}
