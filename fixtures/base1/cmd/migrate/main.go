package main

import (
	"context"
	"log"

	"specs-dont-hallucinate/taskservice/internal/config"
	"specs-dont-hallucinate/taskservice/internal/migrations"
	"specs-dont-hallucinate/taskservice/internal/platform/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := migrations.Apply(ctx, pool, "db/migrations"); err != nil {
		log.Fatal(err)
	}
}
