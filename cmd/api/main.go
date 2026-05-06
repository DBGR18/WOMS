package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/d11nn/woms/internal/api"
)

func main() {
	addr := env("HTTP_ADDR", ":8080")
	jwtSecret := env("JWT_SECRET", "change-me-in-production")
	var store api.Store
	if env("API_STORE", "memory") == "postgres" {
		postgresStore, err := api.NewPostgresStore(env("DATABASE_URL", ""), env("DEMO_SEED_DATA", "true") != "false")
		if err != nil {
			log.Fatalf("postgres store failed: %v", err)
		}
		defer postgresStore.Close()
		store = postgresStore
	} else {
		memoryStore := api.NewMemoryStore()
		if env("DEMO_SEED_DATA", "true") != "false" {
			memoryStore = api.NewDemoMemoryStore()
		}
		store = memoryStore
	}
	publisher := api.ScheduleJobPublisher(api.NoopScheduleJobPublisher{})
	if env("KAFKA_PUBLISH_ENABLED", "true") != "false" {
		publisher = api.NewKafkaScheduleJobPublisher(strings.Split(env("KAFKA_BROKERS", "kafka:9092"), ","), env("KAFKA_SCHEDULE_TOPIC", "woms.schedule.jobs"))
		defer publisher.Close()
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           api.NewServerWithPublisher(jwtSecret, store, publisher),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("woms api listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("api server failed: %v", err)
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
