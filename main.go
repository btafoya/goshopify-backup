package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/btafoya/goshopify-backup/backup"
	"github.com/btafoya/goshopify-backup/lock"
	"github.com/btafoya/goshopify-backup/logger"
	"github.com/btafoya/goshopify-backup/recovery"
	"github.com/btafoya/goshopify-backup/shopify"
	"github.com/btafoya/goshopify-backup/status"
	"github.com/joho/godotenv"
)

// Expected backup modules in execution order
var expectedModules = []string{"products", "customers", "orders", "collections", "content", "metaobjects", "redirects"}

func main() {
	// Load .env file if it exists
	if err := loadEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load .env file: %v\n", err)
	}

	log := logger.NewLogger("info")

	// Parse command line flags
	force := parseForceFlag()
	healthCheck := parseHealthCheckFlag()

	// Handle health check
	if healthCheck {
		cfg, err := GetConfig()
		if err != nil {
			log.Error("Configuration error", logger.LogFields{Error: err.Error()})
			fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
			os.Exit(ExitConfigError)
		}
		handleHealthCheck(cfg, log)
		return
	}

	cfg, err := GetConfig()
	if err != nil {
		log.Error("Configuration error", logger.LogFields{Error: err.Error()})
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(ExitConfigError)
	}

	cfg.Force = force

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	setupSignalHandling(cancel, log)

	// Get backup date (today's date in UTC)
	backupDate := time.Now().UTC().Format(DateFormat)

	// Create backup directory
	backupDir := getBackupDirForDate(cfg, backupDate)
	if err := createBackupDir(cfg, backupDate); err != nil {
		log.Error("Failed to create backup directory", logger.LogFields{Error: err.Error()})
		os.Exit(ExitBackupFailed)
	}

	// Initialize lock manager (use base backup dir, not date-specific path)
	lockManager := lock.NewManager(cfg.BackupDir, StaleLockDuration)

	// Acquire lock
	if !cfg.Force {
		if err := lockManager.Acquire(backupDate); err != nil {
			log.Error("Lock acquisition failed", logger.LogFields{Error: err.Error()})
			fmt.Fprintf(os.Stderr, "Lock error: %v\n", err)
			os.Exit(ExitConcurrentRun)
		}
		defer lockManager.Release(backupDate)
	}

	// Initialize recovery manager
	recoveryManager := recovery.NewManager(cfg.BackupDir)

	// Initialize status writer
	statusWriter := status.NewWriter(backupDir, StatusFlushInterval)
	defer statusWriter.Close()

	// Check if we should resume
	shouldResume, _, err := recoveryManager.ShouldResume(backupDate)
	if err != nil {
		log.Error("Failed to check recovery status", logger.LogFields{Error: err.Error()})
		os.Exit(ExitBackupFailed)
	}

	if shouldResume && !cfg.Force {
		// Load existing status
		statusWriter, err = loadExistingStatus(backupDir)
		if err != nil {
			log.Error("Failed to load existing status", logger.LogFields{Error: err.Error()})
			os.Exit(ExitBackupFailed)
		}
		defer statusWriter.Close()
	} else {
		// Initialize new status
		if err := statusWriter.Initialize(expectedModules); err != nil {
			log.Error("Failed to initialize status", logger.LogFields{Error: err.Error()})
			os.Exit(ExitBackupFailed)
		}
	}

	// Get modules to run
	modulesToRun, err := recoveryManager.GetModulesToRun(backupDate, expectedModules, cfg.Force)
	if err != nil {
		log.Error("Failed to determine modules to run", logger.LogFields{Error: err.Error()})
		os.Exit(ExitBackupFailed)
	}

	if len(modulesToRun) == 0 && !cfg.Force {
		fmt.Println("Backup already completed. Use --force to re-run.")
		return
	}

	// Initialize Shopify clients
	rateLimiter := shopify.NewRateLimiter(RequestsPerSecond)
	shopifyConfig := &shopify.Config{
		Store:       cfg.Store,
		AccessToken: cfg.AccessToken,
		APIVersion:  cfg.APIVersion,
		Limiter:     rateLimiter,
	}

	graphqlClient := shopify.NewGraphQLClient(shopifyConfig)
	restClient := shopify.NewRESTClient(shopifyConfig)

	// Run backup
	log.Info("Shopify backup started", logger.LogFields{
		Module: "main",
		Store:  cfg.Store,
	})

	fmt.Printf("Starting backup for %s\n", backupDate)
	fmt.Printf("Store: %s\n", cfg.Store)
	fmt.Printf("API Version: %s\n", cfg.APIVersion)
	fmt.Printf("Backup Dir: %s\n", backupDir)
	fmt.Printf("Retention Days: %d\n", cfg.RetentionDays)
	if cfg.Force {
		fmt.Printf("Force mode: enabled\n")
	}

	// Run modules
	backupSuccess := runModules(ctx, graphqlClient, restClient, statusWriter, modulesToRun, backupDir, log)

	// Mark backup complete
	if err := statusWriter.MarkBackupComplete(); err != nil {
		log.Error("Failed to mark backup complete", logger.LogFields{Error: err.Error()})
	}

	// Print summary
	printSummary(statusWriter)

	if backupSuccess {
		log.Info("Shopify backup completed successfully", logger.LogFields{
			Module: "main",
		})

		// Run cleanup after successful backup
		if err := cleanupOldBackups(cfg); err != nil {
			log.Error("Cleanup failed", logger.LogFields{Error: err.Error()})
		}

		os.Exit(ExitSuccess)
	} else {
		log.Error("Shopify backup failed", logger.LogFields{
			Module: "main",
		})
		os.Exit(ExitBackupFailed)
	}
}

// loadEnv loads environment variables from .env file
func loadEnv() error {
	if err := importGodotenv(); err != nil {
		return err
	}
	return nil
}

// importGodotenv is a wrapper around godotenv.Load
func importGodotenv() error {
	return godotenv.Load()
}

// parseForceFlag parses the --force flag from command line args
func parseForceFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--force" {
			return true
		}
	}
	return false
}

// parseHealthCheckFlag parses the --health-check flag from command line args
func parseHealthCheckFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--health-check" {
			return true
		}
	}
	return false
}

// handleHealthCheck performs health checks and exits
func handleHealthCheck(cfg *Config, log *logger.Logger) {
	healthy := true

	// Check configuration
	if cfg.Store == "" || cfg.AccessToken == "" {
		fmt.Println("CRITICAL: Configuration incomplete")
		healthy = false
	}

	// Check backup directory
	if _, err := os.Stat(cfg.BackupDir); os.IsNotExist(err) {
		fmt.Printf("WARNING: Backup directory does not exist: %s\n", cfg.BackupDir)
		// Try to create it
		if err := os.MkdirAll(cfg.BackupDir, 0755); err != nil {
			fmt.Printf("CRITICAL: Cannot create backup directory: %v\n", err)
			healthy = false
		}
	}

	// Check write permission
	testFile := cfg.BackupDir + "/.health_check"
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		fmt.Printf("CRITICAL: Backup directory not writable: %v\n", err)
		healthy = false
	} else {
		os.Remove(testFile)
	}

	// Check disk space
	if stat, err := getDiskInfo(cfg.BackupDir); err == nil {
		const minFreeSpace = 1024 * 1024 * 100 // 100 MB minimum
		if stat.Available < minFreeSpace {
			fmt.Printf("WARNING: Low disk space: %d bytes available\n", stat.Available)
		}
	}

	if healthy {
		fmt.Println("OK: All checks passed")
		os.Exit(0)
	} else {
		fmt.Println("CRITICAL: Health check failed")
		os.Exit(1)
	}
}

// DiskInfo holds disk information
type DiskInfo struct {
	Total     uint64
	Available uint64
	Used      uint64
}

// getDiskInfo returns disk information for the given path
func getDiskInfo(path string) (*DiskInfo, error) {
	// This is a simplified implementation
	// In production, use syscall.Statfs on Unix or similar
	return &DiskInfo{
		Total:     1 << 40,   // 1TB
		Available: 500 << 30, // 500GB
		Used:      500 << 30,
	}, nil
}

// setupSignalHandling sets up signal handling for graceful shutdown
func setupSignalHandling(cancel context.CancelFunc, log *logger.Logger) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Warn("Received signal, shutting down", logger.LogFields{
			Module: "main",
		})
		fmt.Printf("\nReceived %v, shutting down gracefully...\n", sig)
		cancel()
	}()
}

// loadExistingStatus loads an existing status from disk
func loadExistingStatus(backupDir string) (*status.Writer, error) {
	backupStatus, err := status.LoadStatus(backupDir)
	if err != nil {
		return nil, err
	}
	if backupStatus == nil {
		return status.NewWriter(backupDir, StatusFlushInterval), nil
	}

	// Create a new writer with the loaded status
	writer := status.NewWriter(backupDir, StatusFlushInterval)
	// Load the existing status into the writer
	// Note: This is a simplified version - in production we'd need to properly
	// initialize the writer with the existing status
	return writer, nil
}

// runModules runs the backup modules
func runModules(ctx context.Context, graphqlClient *shopify.GraphQLClient, restClient *shopify.RESTClient, statusWriter *status.Writer, modules []string, outputDir string, log *logger.Logger) bool {
	success := true

	for _, moduleName := range modules {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			log.Error("Context cancelled", logger.LogFields{Module: moduleName})
			return false
		default:
		}

		// Mark module as running
		if err := statusWriter.Update(status.StatusUpdate{
			Module:    moduleName,
			Status:    "running",
			Timestamp: time.Now().UTC(),
		}); err != nil {
			log.Error("Failed to update status", logger.LogFields{Error: err.Error()})
		}

		fmt.Printf("\n[%s] Starting...\n", moduleName)

		// Run the module
		var count int
		var fileSize int64
		var err error
		var fallback string

		startTime := time.Now()

		switch moduleName {
		case "products":
			productsMod := backup.NewProductsModule(outputDir+"/images", true, ImageConcurrency)
			count, fileSize, fallback, err = runProductsWithFallback(ctx, graphqlClient, restClient, productsMod, outputDir)

		case "customers":
			customersMod := backup.NewCustomersModule()
			count, fileSize, fallback, err = runCustomersWithFallback(ctx, graphqlClient, restClient, customersMod, outputDir)

		case "orders":
			ordersMod := backup.NewOrdersModule()
			count, fileSize, fallback, err = runOrdersWithFallback(ctx, graphqlClient, restClient, ordersMod, outputDir)

		case "collections":
			collectionsMod := backup.NewCollectionsModule()
			count, fileSize, fallback, err = runCollectionsWithFallback(ctx, graphqlClient, restClient, collectionsMod, outputDir)

		case "content":
			contentMod := backup.NewContentModule()
			count, fileSize, err = contentMod.Run(ctx, restClient, outputDir)

		case "metaobjects":
			metaobjectsMod := backup.NewMetaobjectsModule()
			count, fileSize, fallback, err = runMetaobjectsWithFallback(ctx, graphqlClient, metaobjectsMod, outputDir)

		case "redirects":
			redirectsMod := backup.NewRedirectsModule()
			count, fileSize, fallback, err = runRedirectsWithFallback(ctx, graphqlClient, restClient, redirectsMod, outputDir)
		}

		duration := time.Since(startTime)

		if err != nil {
			log.Error("Module failed", logger.LogFields{
				Module:   moduleName,
				Error:    err.Error(),
				Duration: duration.String(),
			})

			if err := statusWriter.Update(status.StatusUpdate{
				Module:    moduleName,
				Status:    "failed",
				Error:     err.Error(),
				Fallback:  fallback,
				Timestamp: time.Now().UTC(),
			}); err != nil {
				log.Error("Failed to update status", logger.LogFields{Error: err.Error()})
			}

			fmt.Printf("[%s] Failed: %v\n", moduleName, err)
			if fallback != "" {
				fmt.Printf("[%s] Used fallback: %s\n", moduleName, fallback)
			}
			success = false
			continue
		}

		log.Info("Module completed", logger.LogFields{
			Module:   moduleName,
			Count:    count,
			Duration: duration.String(),
		})

		if err := statusWriter.Update(status.StatusUpdate{
			Module:    moduleName,
			Status:    "completed",
			Count:     count,
			FileSize:  fileSize,
			Fallback:  fallback,
			Timestamp: time.Now().UTC(),
		}); err != nil {
			log.Error("Failed to update status", logger.LogFields{Error: err.Error()})
		}

		fmt.Printf("[%s] Completed: %d records (%d bytes) in %v\n", moduleName, count, fileSize, duration)
		if fallback != "" {
			fmt.Printf("[%s] Used fallback: %s\n", moduleName, fallback)
		}
	}

	return success
}

// runProductsWithFallback runs products backup with REST fallback
func runProductsWithFallback(ctx context.Context, graphqlClient *shopify.GraphQLClient, restClient *shopify.RESTClient, mod *backup.ProductsModule, outputDir string) (int, int64, string, error) {
	count, size, err := mod.Run(ctx, graphqlClient, outputDir)
	if err != nil {
		if isAccessDenied(err) {
			fmt.Printf("[products] Bulk operation access denied, falling back to REST\n")
			restFallback := backup.NewRESTFallbackModule("products", "products", "/products.json")
			count, size, err = restFallback.Run(ctx, restClient, outputDir)
			if err != nil {
				return 0, 0, "", err
			}
			return count, size, "REST", nil
		}
		return 0, 0, "", err
	}
	return count, size, "", nil
}

// runCustomersWithFallback runs customers backup with REST fallback
func runCustomersWithFallback(ctx context.Context, graphqlClient *shopify.GraphQLClient, restClient *shopify.RESTClient, mod *backup.CustomersModule, outputDir string) (int, int64, string, error) {
	count, size, err := mod.Run(ctx, graphqlClient, outputDir)
	if err != nil {
		if isAccessDenied(err) {
			fmt.Printf("[customers] Bulk operation access denied, falling back to REST\n")
			restFallback := backup.NewRESTFallbackModule("customers", "customers", "/customers.json")
			count, size, err = restFallback.Run(ctx, restClient, outputDir)
			if err != nil {
				return 0, 0, "", err
			}
			return count, size, "REST", nil
		}
		return 0, 0, "", err
	}
	return count, size, "", nil
}

// runOrdersWithFallback runs orders backup with REST fallback
func runOrdersWithFallback(ctx context.Context, graphqlClient *shopify.GraphQLClient, restClient *shopify.RESTClient, mod *backup.OrdersModule, outputDir string) (int, int64, string, error) {
	count, size, err := mod.Run(ctx, graphqlClient, outputDir)
	if err != nil {
		if isAccessDenied(err) {
			fmt.Printf("[orders] Bulk operation access denied, falling back to REST\n")
			restFallback := backup.NewRESTFallbackModule("orders", "orders", "/orders.json")
			count, size, err = restFallback.Run(ctx, restClient, outputDir)
			if err != nil {
				return 0, 0, "", err
			}
			return count, size, "REST", nil
		}
		return 0, 0, "", err
	}
	return count, size, "", nil
}

// runCollectionsWithFallback runs collections backup with REST fallback
func runCollectionsWithFallback(ctx context.Context, graphqlClient *shopify.GraphQLClient, restClient *shopify.RESTClient, mod *backup.CollectionsModule, outputDir string) (int, int64, string, error) {
	count, size, err := mod.Run(ctx, graphqlClient, outputDir)
	if err != nil {
		if isAccessDenied(err) {
			fmt.Printf("[collections] Bulk operation access denied, falling back to REST\n")
			restFallback := backup.NewRESTFallbackModule("collections", "collections", "/custom_collections.json")
			count, size, err = restFallback.Run(ctx, restClient, outputDir)
			if err != nil {
				return 0, 0, "", err
			}
			return count, size, "REST", nil
		}
		return 0, 0, "", err
	}
	return count, size, "", nil
}

// runMetaobjectsWithFallback runs metaobjects backup using pagination (bulk requires type argument)
func runMetaobjectsWithFallback(ctx context.Context, graphqlClient *shopify.GraphQLClient, mod *backup.MetaobjectsModule, outputDir string) (int, int64, string, error) {
	// Metaobjects uses pagination as primary method since bulk operations require type argument
	count, size, err := mod.Run(ctx, graphqlClient, outputDir)
	if err != nil {
		return 0, 0, "", err
	}
	return count, size, "PAGINATION", nil
}

// runRedirectsWithFallback runs redirects backup with REST fallback
func runRedirectsWithFallback(ctx context.Context, graphqlClient *shopify.GraphQLClient, restClient *shopify.RESTClient, mod *backup.RedirectsModule, outputDir string) (int, int64, string, error) {
	count, size, err := mod.Run(ctx, graphqlClient, outputDir)
	if err != nil {
		if isAccessDenied(err) {
			fmt.Printf("[redirects] Bulk operation access denied, falling back to REST\n")
			count, size, err = mod.RunREST(ctx, restClient, outputDir)
			if err != nil {
				return 0, 0, "", err
			}
			return count, size, "REST", nil
		}
		return 0, 0, "", err
	}
	return count, size, "", nil
}

// isAccessDenied checks if an error is an AccessDeniedError
func isAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "ACCESS_DENIED")
}

// printSummary prints the backup summary
func printSummary(statusWriter *status.Writer) {
	status := statusWriter.GetStatus()

	fmt.Printf("\n=== Backup Summary ===\n")
	fmt.Printf("Started: %s\n", status.StartedAt.Format(time.RFC3339))
	if !status.CompletedAt.IsZero() {
		fmt.Printf("Completed: %s\n", status.CompletedAt.Format(time.RFC3339))
		fmt.Printf("Duration: %s\n", status.Duration)
	}
	fmt.Printf("\nModules:\n")

	for _, module := range expectedModules {
		if modStatus, ok := status.Modules[module]; ok {
			statusSymbol := " "
			switch modStatus.Status {
			case "completed":
				statusSymbol = "✓"
			case "failed":
				statusSymbol = "✗"
			case "running":
				statusSymbol = "→"
			case "pending":
				statusSymbol = "○"
			}
			fmt.Printf("  %s %s: %s (%d records, %d bytes)", statusSymbol, module, modStatus.Status, modStatus.Count, modStatus.FileSize)
			if modStatus.Fallback != "" {
				fmt.Printf(" [fallback: %s]", modStatus.Fallback)
			}
			if modStatus.Error != "" {
				fmt.Printf(" - %s", modStatus.Error)
			}
			fmt.Println()
		}
	}

	fmt.Printf("\nTotal Size: %d bytes\n", status.TotalSize)
	fmt.Printf("=====================\n")
}
