package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/company/echo/internal/config"
	"github.com/company/echo/internal/domain"
	"github.com/company/echo/internal/infrastructure/detector"
	"github.com/company/echo/internal/infrastructure/embedder"
	"github.com/company/echo/internal/infrastructure/httpserver"
	"github.com/company/echo/internal/infrastructure/mcp"
	"github.com/company/echo/internal/infrastructure/store"
	echosync "github.com/company/echo/internal/sync"
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
	serveCmd.Flags().String("embedder", "local", "Embedding provider: local, vertex-ai, openai, cohere")
	serveCmd.Flags().String("log-level", "info", "Log level: debug, info, warn, error")
	serveCmd.Flags().String("data-dir", "", "Data directory (default: ~/.config/echo)")
	serveCmd.Flags().String("http-addr", ":7438", "HTTP server address (empty to disable)")
	serveCmd.Flags().String("model-path", "", "Path to ONNX model file (default: ~/.config/echo/models/all-MiniLM-L6-v2.onnx)")
	serveCmd.Flags().String("vocab-path", "", "Path to vocab.txt file (default: ~/.config/echo/models/vocab.txt)")
	serveCmd.Flags().String("runtime-path", "", "Path to libonnxruntime.so (optional, searches default paths)")

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
			httpPort, _ := cmd.Flags().GetString("http-port")
			return setup.SetupOpenCode(setup.OpenCodeOptions{HTTPPort: httpPort})
		},
	}

	opencodeCmd.Flags().String("http-port", "7438", "HTTP server port for plugin communication")

	setupCmd.AddCommand(opencodeCmd)
	rootCmd.AddCommand(setupCmd)

	// Sync command.
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync learnings from git",
		Long:  "Import learnings from .echo/chunks directory and update manifest.",
		RunE:  runSync,
	}

	syncCmd.Flags().String("import", "", "Import chunks from .echo/chunks (default: current directory)")
	syncCmd.Flags().String("data-dir", "", "Data directory (default: ~/.config/echo)")

	rootCmd.AddCommand(syncCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	// Load configuration.
	cfg := config.Load()

	// Override with flags.
	mode, _ := cmd.Flags().GetString("mode")
	embedderProvider, _ := cmd.Flags().GetString("embedder")
	logLevel, _ := cmd.Flags().GetString("log-level")
	dataDir, _ := cmd.Flags().GetString("data-dir")
	httpAddr, _ := cmd.Flags().GetString("http-addr")
	modelPath, _ := cmd.Flags().GetString("model-path")
	vocabPath, _ := cmd.Flags().GetString("vocab-path")
	runtimePath, _ := cmd.Flags().GetString("runtime-path")

	if mode != "" {
		cfg.Mode = mode
	}
	if embedderProvider != "" {
		cfg.Embedder = embedderProvider
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	if dataDir != "" {
		cfg.DataDir = dataDir
		cfg.DBPath = dataDir + "/echo.db"
		modelDir := dataDir + "/models"
		cfg.ModelPath = modelDir + "/all-MiniLM-L6-v2.onnx"
		cfg.VocabPath = modelDir + "/vocab.txt"
	}
	if modelPath != "" {
		cfg.ModelPath = modelPath
	}
	if vocabPath != "" {
		cfg.VocabPath = vocabPath
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

	// Create embedder based on mode.
	var embedderImpl domain.Embedder // nil in local mode

	switch cfg.Mode {
	case "local":
		// No embedder, BM25 only.
		logger.Info("running in local mode (BM25 lexical search)")

	case "embeddings":
		// Ensure model files exist (download if needed).
		modelDir := cfg.DataDir + "/models"
		downloadedModel, downloadedVocab, err := embedder.EnsureModel(modelDir)
		if err != nil {
			return fmt.Errorf("ensure model: %w", err)
		}

		// Use downloaded paths if not explicitly set.
		effectiveModelPath := cfg.ModelPath
		if effectiveModelPath == "" {
			effectiveModelPath = downloadedModel
		}
		effectiveVocabPath := cfg.VocabPath
		if effectiveVocabPath == "" {
			effectiveVocabPath = downloadedVocab
		}

		// Create ONNX embedder.
		onnxE, err := embedder.NewONNXEmbedder(effectiveModelPath, effectiveVocabPath, runtimePath)
		if err != nil {
			return fmt.Errorf("create ONNX embedder: %w", err)
		}
		defer onnxE.Close()
		embedderImpl = onnxE

		logger.Info("running in embeddings mode (semantic search)",
			"model", effectiveModelPath,
			"vocab", effectiveVocabPath,
			"dimensions", onnxE.Dimensions(),
		)

	case "cloud":
		return fmt.Errorf("cloud mode not yet implemented (Phase 3b)")

	default:
		return fmt.Errorf("unknown mode: %s", cfg.Mode)
	}

	// Create store with vector support if embedder is available.
	var textStore *store.SQLiteFTS5Store
	var err error

	if embedderImpl != nil {
		// Embeddings mode: create store with vector dimensions.
		textStore, err = store.NewSQLiteFTS5StoreWithDimensions(cfg.DBPath, embedderImpl.Dimensions())
	} else {
		// Local mode: create store without vector support.
		textStore, err = store.NewSQLiteFTS5Store(cfg.DBPath)
	}
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer textStore.Close()

	// Initialize usecases.
	saveUC := usecase.NewSaveLearning(textStore, embedderImpl, projDet, identDet)
	searchUC := usecase.NewSearchLearning(textStore, projDet, embedderImpl)
	policyUC := usecase.NewGetPolicies(textStore, projDet)

	// Create and run MCP server.
	mcpServer := mcp.NewServer(saveUC, searchUC, policyUC, logger)

	// Create context that cancels on SIGINT/SIGTERM.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start HTTP server if address is configured.
	if httpAddr != "" {
		httpSrv, err := httpserver.NewServer(httpAddr, cfg.DBPath, logger)
		if err != nil {
			return fmt.Errorf("create HTTP server: %w", err)
		}

		go func() {
			if err := httpSrv.Start(ctx); err != nil && err != context.Canceled {
				logger.Error("HTTP server error", "error", err)
				cancel()
			}
		}()

		logger.Info("echo HTTP server starting", "addr", httpAddr)
	}

	// Handle shutdown signals.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("echo MCP server starting",
		"mode", cfg.Mode,
		"embedder", cfg.Embedder,
		"data_dir", cfg.DataDir,
		"http_addr", httpAddr,
	)

	// Run MCP server in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		errCh <- mcpServer.Run(ctx)
	}()

	// Wait for signal or error.
	select {
	case sig := <-sigCh:
		logger.Info("received signal, shutting down", "signal", sig)
		cancel()
		return <-errCh
	case err := <-errCh:
		return err
	}
}

func runSync(cmd *cobra.Command, args []string) error {
	importFlag, _ := cmd.Flags().GetString("import")
	dataDir, _ := cmd.Flags().GetString("data-dir")

	// Determine project directory.
	projectDir := "."
	if importFlag != "" {
		projectDir = importFlag
	}

	// Load configuration.
	cfg := config.Load()
	if dataDir != "" {
		cfg.DataDir = dataDir
		cfg.DBPath = dataDir + "/echo.db"
	}

	// Setup logger.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create store.
	textStore, err := store.NewSQLiteFTS5Store(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer textStore.Close()

	// Create syncer and import chunks.
	syncer := echosync.NewSyncer(textStore, logger)
	count, err := syncer.ImportChunks(context.Background(), projectDir)
	if err != nil {
		return fmt.Errorf("import chunks: %w", err)
	}

	fmt.Printf("✅ Imported %d chunks from %s/.echo/chunks\n", count, projectDir)
	return nil
}
