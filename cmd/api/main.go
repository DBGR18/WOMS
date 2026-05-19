package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/d11nn/woms/internal/api"
)

func main() {
	addr := env("HTTP_ADDR", ":8080")
	jwtSecret := env("JWT_SECRET", "change-me-in-production")
	dependencyTimeout := envDuration("API_DEPENDENCY_RETRY_TIMEOUT_MS", 2*time.Minute)
	dependencyInterval := envDuration("API_DEPENDENCY_RETRY_INTERVAL_MS", 2*time.Second)
	var store api.Store
	if env("API_STORE", "memory") == "postgres" {
		ctx, cancel := context.WithTimeout(context.Background(), dependencyTimeout)
		var postgresStore *api.PostgresStore
		err := retryDependency(ctx, "postgres store", dependencyInterval, func(context.Context) error {
			var err error
			postgresStore, err = api.NewPostgresStore(env("DATABASE_URL", ""), env("DEMO_SEED_DATA", "true") != "false")
			return err
		})
		cancel()
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
		brokers := strings.Split(env("KAFKA_BROKERS", "kafka:9092"), ",")
		ctx, cancel := context.WithTimeout(context.Background(), dependencyTimeout)
		if err := retryDependency(ctx, "kafka broker", dependencyInterval, func(ctx context.Context) error {
			return pingTCP(ctx, strings.TrimSpace(brokers[0]))
		}); err != nil {
			cancel()
			log.Fatalf("kafka broker failed: %v", err)
		}
		cancel()
		publisher = api.NewKafkaScheduleJobPublisher(brokers, env("KAFKA_SCHEDULE_TOPIC", "woms.schedule.jobs"))
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
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	millis, err := strconv.Atoi(value)
	if err != nil || millis < 0 {
		return fallback
	}
	return time.Duration(millis) * time.Millisecond
}

func retryDependency(ctx context.Context, name string, interval time.Duration, operation func(context.Context) error) error {
	if interval <= 0 {
		interval = time.Second
	}
	var lastErr error
	for attempt := 1; ; attempt++ {
		if err := operation(ctx); err == nil {
			if attempt > 1 {
				log.Printf("%s ready after %d attempts", name, attempt)
			}
			return nil
		} else {
			lastErr = err
			log.Printf("%s not ready attempt=%d error=%v", name, attempt, err)
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("%s not ready before timeout: %w", name, lastErr)
		case <-timer.C:
		}
	}
}

func pingTCP(ctx context.Context, address string) error {
	if address == "" {
		return fmt.Errorf("empty address")
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	return conn.Close()
}
