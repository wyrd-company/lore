package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/wyrd-company/lore/internal/config"
	"github.com/wyrd-company/lore/internal/database"
	"github.com/wyrd-company/lore/internal/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("lore failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}
	switch args[0] {
	case "version", "--version", "-version":
		fmt.Printf("lore %s\n", version.Value)
		return nil
	case "migrate":
		cfg := config.FromEnvironment()
		if err := cfg.ValidateDatabase(); err != nil {
			return err
		}
		return database.Migrate(context.Background(), cfg.DatabaseURL)
	case "sync", "watch", "annotations", "briefings":
		return fmt.Errorf("%s is reserved for a later milestone", args[0])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Println("Lore command-line client")
	fmt.Println("usage: lore <version|migrate|sync|watch|annotations|briefings>")
}
