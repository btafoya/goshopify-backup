package main

import (
	"fmt"
	"os"

	"github.com/btafoya/goshopify-backup/logger"
)

func main() {
	log := logger.NewLogger("info")

	cfg, err := GetConfig()
	if err != nil {
		log.Error("Configuration error", logger.LogFields{Error: err.Error()})
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(ExitConfigError)
	}

	log.Info("Shopify backup tool started", logger.LogFields{
		Module: "main",
		Store:  cfg.Store,
	})

	fmt.Printf("Config loaded:\n")
	fmt.Printf("  Store: %s\n", cfg.Store)
	fmt.Printf("  API Version: %s\n", cfg.APIVersion)
	fmt.Printf("  Backup Dir: %s\n", cfg.BackupDir)
	fmt.Printf("  Retention Days: %d\n", cfg.RetentionDays)
	fmt.Printf("  Force: %v\n", cfg.Force)

	log.Info("Shopify backup tool completed", logger.LogFields{
		Module: "main",
	})
}