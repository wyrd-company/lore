package main

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/wyrd-company/lore/internal/cli"
)

func main() {
	if code := run(os.Args[1:], os.Stdout, os.Stderr); code != 0 {
		os.Exit(code)
	}
}

func run(args []string, out, errOut io.Writer) int {
	if err := cli.New(out, errOut).Run(context.Background(), args); err != nil {
		if !cli.IsReportedError(err) {
			slog.New(slog.NewTextHandler(errOut, nil)).Error("lore failed", "error", err)
		}
		return 1
	}
	return 0
}
