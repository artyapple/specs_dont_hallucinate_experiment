package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"

	"specs-dont-hallucinate/taskservice/internal/config"
	"specs-dont-hallucinate/taskservice/internal/httpapi"
	"specs-dont-hallucinate/taskservice/internal/platform/database"
	"specs-dont-hallucinate/taskservice/internal/platform/httpserver"
	"specs-dont-hallucinate/taskservice/internal/repository/postgres"
	"specs-dont-hallucinate/taskservice/internal/task"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	repository := postgres.NewTaskRepository(pool)
	service := task.NewService(repository)
	router := chi.NewRouter()
	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	httpapi.RegisterRoutes(router, service)

	if err := httpserver.Run(ctx, cfg.HTTPAddr, router); err != nil {
		log.Fatal(err)
	}
}
