package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	showHelp    bool
	showVersion bool
)

func init() {
	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&showHelp, "h", false, "Show help")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&showVersion, "v", false, "Show version")
}

func main() {
	flag.Parse()

	if showHelp {
		printHelp()
		os.Exit(0)
	}

	if showVersion {
		printVersion()
		os.Exit(0)
	}

	// Load configuration
	cfg, err := GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(ExitConfigError)
	}

	// Initialize TUI model
	model, err := InitialModel(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize TUI: %v\n", err)
		os.Exit(ExitConfigError)
	}

	// Start Bubbletea program
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(ExitFailed)
	}
}

func printHelp() {
	fmt.Printf(`Shopify Restore CLI - Restore Shopify store data from JSON backups

USAGE:
    shopify-restore [OPTIONS]

OPTIONS:
    --backup-dir DIR      Backup directory (default: /backups/shopify)
    --backup-date DATE    Specific backup date to restore (YYYY-MM-DD)
    --store URL           Target Shopify store URL
    --token TOKEN         Shopify access token
    --dry-run, -n         Validate only, don't restore
    --force, -f           Skip conflict prompts, use defaults
    --resume              Resume from interrupted restore
    --verbose, -v         Verbose logging
    --images-restore      Always restore product images
    --images-skip         Always skip product images
    --help, -h            Show this help message
    --version, -v         Show version

ENVIRONMENT VARIABLES:
    SHOPIFY_STORE         Target store URL (https://*.myshopify.com)
    SHOPIFY_ACCESS_TOKEN  Shopify access token
    SHOPIFY_API_VERSION   Shopify API version (default: 2025-07)
    BACKUP_DIR            Backup directory (default: /backups/shopify)
    LOG_DIR               Log directory (default: /var/log/goshopify)
    ROLLBACK_DIR          Rollback script directory

KEYBOARD SHORTCUTS:
    ↑/k         Move cursor up
    ↓/j         Move cursor down
    Space       Toggle selection
    Ctrl+A      Select all items
    Enter       Confirm/Proceed
    Esc         Go back/Cancel
    Ctrl+C      Quit
    ?/F1        Show help
    /           Start search/filter
    Tab         Switch panels

EXAMPLES:
    # Interactive mode
    shopify-restore

    # Non-interactive with flags
    shopify-restore --backup-dir /backups/shopify --backup-date 2026-04-02 \\
        --store staging.myshopify.com --token shpat_xxxx

    # Dry-run (validate only)
    shopify-restore --dry-run

    # Resume interrupted restore
    shopify-restore --resume

EXIT CODES:
    0    Successful restore
    1    Restore failed
    2    Configuration error
    3    User aborted
    4    Validation error
    5    Network error

`)
}

func printVersion() {
	fmt.Println("shopify-restore version 0.1.0")
}