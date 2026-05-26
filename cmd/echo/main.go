package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/company/echo/internal/config"
	"github.com/company/echo/internal/domain"
	"github.com/company/echo/internal/infrastructure/detector"
	"github.com/company/echo/internal/infrastructure/mcp"
	"github.com/company/echo/internal/infrastructure/store"
	"github.com/company/echo/internal/setup"
	"github.com/company/echo/internal/usecase"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "echo",
		Short: "Echo - Shared team memory for AI agents",
		Long:  "Echo is a shared team memory layer that sits between developers and their AI agents.",
	}

	// Serve command.
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Echo MCP server",
		Long:  "Start the Echo MCP server over stdio transport for AI agent integration.",
		RunE:  runServe,
	}

	serveCmd.Flags().String("mode", "local", "Operating mode: local, embeddings, cloud")
	serveCmd.Flags().String("embedder", "vertex-ai", "Embedding provider: vertex-ai, openai, cohere")
	serveCmd.Flags().String("log-level", "info", "Log level: debug, info, warn, error")
	serveCmd.Flags().String("data-dir", "", "Data directory (default: ~/.config/echo)")

	rootCmd.AddCommand(serveCmd)

	// Admin command (placeholder for Phase 4).
	adminCmd := &cobra.Command{
		Use:   "admin",
		Short: "Admin CLI for organization-scoped learnings",
		Long:  "Admin commands for managing organization-scoped policies (Phase 4).",
	}
	adminCmd.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add a global rule",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Admin add not yet implemented (Phase 4)")
		},
	})
	adminCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all global rules",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Admin list not yet implemented (Phase 4)")
		},
	})
	adminCmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update a global rule",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Admin update not yet implemented (Phase 4)")
		},
	})
	adminCmd.AddCommand(&cobra.Command{
		Use:   "delete",
		Short: "Delete a global rule",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Admin delete not yet implemented (Phase 4)")
		},
	})

	rootCmd.AddCommand(adminCmd)

	// Setup command.
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure Echo for your AI agent",
		Long:  "Automatically configure your AI agent to use Echo.",
	}

	opencodeCmd := &cobra.Command{
		Use:   "opencode",
		Short: "Configure Echo for OpenCode",
		Long:  "Add Echo MCP server and plugin to your OpenCode configuration.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setup.SetupOpenCode()
		},
	}

	setupCmd.AddCommand(opencodeCmd)
	rootCmd.AddCommand(setupCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	// Load configuration.
	cfg := config.Load()

	// Override with flags.
	mode, _ := cmd.Flags().GetString("mode")
	embedder, _ := cmd.Flags().GetString("embedder")
	logLevel, _ := cmd.Flags().GetString("log-level")
	dataDir, _ := cmd.Flags().GetString("data-dir")

	if mode != "" {
		cfg.Mode = mode
	}
	if embedder != "" {
		cfg.Embedder = embedder
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	if dataDir != "" {
		cfg.DataDir = dataDir
		cfg.DBPath = dataDir + "/echo.db"
	}

	// Setup logger.
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))

	// Initialize infrastructure.
	projDet := detector.NewGitProjectDetector("")
	identDet := detector.NewGitIdentityDetector("")

	// Create store based on mode.
	var textStore *store.SQLiteFTS5Store
	var err error

	switch cfg.Mode {
	case "local", "embeddings":
		textStore, err = store.NewSQLiteFTS5Store(cfg.DBPath)
		if err != nil {
			return fmt.Errorf("create store: %w", err)
		}
		defer textStore.Close()

		// If embeddings mode, run migration.
		if cfg.Mode == "embeddings" {
			// Phase 2 migration: create vec_learnings table.
			// For now, skip since we don't have sqlite-vec yet.
			logger.Info("embeddings mode enabled (sqlite-vec migration pending)")
		}

	case "cloud":
		return fmt.Errorf("cloud mode not yet implemented (Phase 3)")

	default:
		return fmt.Errorf("unknown mode: %s", cfg.Mode)
	}

	// Embedder: nil for Phase 1, configured for Phase 2+.
	var embedderImpl domain.Embedder // nil in Phase 1

	// Initialize usecases.
	saveUC := usecase.NewSaveLearning(textStore, embedderImpl, projDet, identDet)
	searchUC := usecase.NewSearchLearning(textStore, projDet)
	policyUC := usecase.NewGetPolicies(textStore, projDet)

	// Create and run MCP server.
	mcpServer := mcp.NewServer(saveUC, searchUC, policyUC, logger)

	logger.Info("echo MCP server starting",
		"mode", cfg.Mode,
		"embedder", cfg.Embedder,
		"data_dir", cfg.DataDir,
	)

	return mcpServer.Run(context.Background())
}
