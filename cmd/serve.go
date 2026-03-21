package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	
	"mayo-cli/internal/api"
	"mayo-cli/internal/config"

	"github.com/spf13/cobra"
)

var servePort string
var serveToken string
var serveSession string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start Mayo as a Master Multi-Session API server",
	Long: `Start Mayo as a secured REST API server that can serve ALL sessions.
Other applications can query specific sessions via the URL path.

Authentication: Bearer token (set via --token or /setup).

Endpoints:
  POST /v1/:session_id/query   — Query a specific session
  GET  /v1/:session_id/status  — Get status of a session
  GET  /v1/sessions            — List all available sessions`,
	Run: func(cmd *cobra.Command, args []string) {
		if spawned {
			// If backgrounded, ignore SIGHUP from parent's terminal exit
			signal.Ignore(syscall.SIGHUP)
		}
		
		cfg, _ := config.LoadConfig()

		// 1. Resolve port: flag > config > default
		port := 8080
		if servePort != "" {
			fmt.Sscanf(servePort, "%d", &port)
		} else if cfg != nil && cfg.ServePort > 0 {
			port = cfg.ServePort
		}

		// 2. Resolve token: flag > config
		token := ""
		if cfg != nil && cfg.ServeToken != "" {
			token = cfg.ServeToken
		}
		if serveToken != "" {
			token = serveToken
		}

		// Starts the Multi-Session Server
		server := api.NewServer(cfg, token, port)
		if err := server.Start(); err != nil {
			fmt.Printf("❌ Server error: %v\n", err)
			os.Exit(1)
		}
	},
}

var spawned bool

func init() {
	serveCmd.Flags().StringVarP(&servePort, "port", "p", "", "Port to listen on (default: 8080)")
	serveCmd.Flags().StringVarP(&serveToken, "token", "t", "", "Bearer token for API authentication")
	serveCmd.Flags().StringVar(&serveSession, "session", "", "Internal use: indicates a specific session for authorization")
	serveCmd.Flags().BoolVar(&spawned, "spawned", false, "Internal use: indicates a Mayo background process")
	serveCmd.Flags().MarkHidden("spawned")
	serveCmd.Flags().MarkHidden("session")
	rootCmd.AddCommand(serveCmd)
}
