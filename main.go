package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/DataDog/migrate-tool/cmd"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := cmd.New("migrate", "Migrate").ExecuteContext(ctx); err != nil {
		cancel()
		os.Exit(1)
	}
}
