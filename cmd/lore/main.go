package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/wyrd-company/lore/internal/cli"
)

func main() {
	if err := cli.New(os.Stdout, os.Stderr).Run(context.Background(), os.Args[1:]); err != nil {
		slog.Error("lore failed", "error", err)
		os.Exit(1)
	}
}
