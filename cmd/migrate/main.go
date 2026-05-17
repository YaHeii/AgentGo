package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/YaHeii/agentGo/internal/db"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dbPath := fs.String("db", "agentgo.db", "sqlite database path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		return fmt.Errorf("usage: migrate [-db path] <up|down> [steps]")
	}

	direction := rest[0]
	steps := 0
	if len(rest) == 2 {
		parsed, err := strconv.Atoi(rest[1])
		if err != nil {
			return fmt.Errorf("parse steps: %w", err)
		}
		steps = parsed
	}

	switch direction {
	case "up":
		return db.MigrateUp(ctx, *dbPath, steps)
	case "down":
		return db.MigrateDown(ctx, *dbPath, steps)
	default:
		return fmt.Errorf("unknown direction %q", direction)
	}
}
