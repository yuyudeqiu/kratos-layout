package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/pprof"
	"runtime"
	"time"

	v1 "github.com/go-kratos/kratos-layout/api/todo/v1"
	"github.com/go-kratos/kratos-layout/internal/conf"
	"github.com/go-kratos/kratos-layout/internal/service"
	"github.com/go-kratos/kratos/contrib/otel/v3/tracing"
	"github.com/go-kratos/kratos/v3/log"
	"github.com/go-kratos/kratos/v3/middleware/recovery"
	"github.com/go-kratos/kratos/v3/middleware/validate"
	kratoshttp "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/redis/go-redis/v9"

	"go.einride.tech/aip/fieldbehavior"
	"google.golang.org/protobuf/proto"
)

// serviceName is the meter/tracer scope name shared by both servers.
const serviceName = "kratos.layout"

// Pprof keeps the profiling server in the Wire dependency graph.
type Pprof struct{}

// HealthStatus represents the health check response.
type HealthStatus struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Database  string `json:"database,omitempty"`
	Redis     string `json:"redis,omitempty"`
}

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server, todo *service.TodoService, metricsHandler http.Handler, sqlDB *sql.DB, rdb redis.UniversalClient) *kratoshttp.Server {
	var opts = []kratoshttp.ServerOption{
		kratoshttp.Middleware(
			tracing.Server(),
			newServerMetricsMiddleware(serviceName),
			recovery.Recovery(),
			validate.Validator(func(req any) error {
				if msg, ok := req.(proto.Message); ok {
					if err := fieldbehavior.ValidateRequiredFields(msg); err != nil {
						return err
					}
				}
				return nil
			}),
		),
	}
	if c.Http.Network != "" {
		opts = append(opts, kratoshttp.Network(c.Http.Network))
	}
	if c.Http.Addr != "" {
		opts = append(opts, kratoshttp.Address(c.Http.Addr))
	}
	if c.Http.Timeout != nil {
		opts = append(opts, kratoshttp.Timeout(c.Http.Timeout.AsDuration()))
	}
	srv := kratoshttp.NewServer(opts...)
	v1.RegisterTodoServiceHTTPServer(srv, todo)

	// Prometheus metrics endpoint.
	if metricsHandler != nil {
		srv.Handle("/metrics", metricsHandler)
	}

	// Health check — verifies DB and Redis connectivity.
	srv.HandleFunc("/healthz", healthHandler(sqlDB, rdb))

	// Readiness check — lighter, just returns ok.
	srv.HandleFunc("/readyz", readyHandler())

	return srv
}

// healthHandler returns a handler that checks database and Redis connectivity.
func healthHandler(sqlDB *sql.DB, rdb redis.UniversalClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		status := HealthStatus{
			Status:    "ok",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Database:  "ok",
			Redis:     "ok",
		}

		// Database check
		if sqlDB == nil {
			status.Status = "degraded"
			status.Database = "unavailable"
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := sqlDB.PingContext(ctx); err != nil {
				log.WarnContext(ctx, "health check: database ping failed", "error", err)
				status.Status = "degraded"
				status.Database = fmt.Sprintf("error: %v", err)
			}
		}

		// Redis check
		if rdb == nil {
			status.Status = "degraded"
			status.Redis = "unavailable"
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
			defer cancel()
			if err := rdb.Ping(ctx).Err(); err != nil {
				log.WarnContext(ctx, "health check: redis ping failed", "error", err)
				status.Status = "degraded"
				status.Redis = fmt.Sprintf("error: %v", err)
			}
		}

		if status.Status != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(status)
	}
}

// readyHandler returns a handler that always returns ok.
// It's a lightweight readiness probe separate from the liveness health check.
func readyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthStatus{
			Status:    "ok",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// NewPprof starts the pprof server when enabled.
func NewPprof(c *conf.Server) (Pprof, func(), error) {
	stop, err := StartPprof(c.Pprof)
	return Pprof{}, stop, err
}

// StartPprof starts a pprof HTTP server on the configured address.
// Returns a stop function that shuts down the pprof server.
func StartPprof(cfg *conf.Server_Pprof) (func(), error) {
	if cfg == nil || !cfg.Enabled {
		log.Info("pprof disabled")
		return func() {}, nil
	}

	addr := cfg.Addr
	if addr == "" {
		addr = ":6060"
	}

	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(1)

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	pprofSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("pprof listening", "addr", addr)
		if err := pprofSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("pprof server error", "error", err)
		}
	}()

	stop := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := pprofSrv.Shutdown(ctx); err != nil {
			log.Error("pprof shutdown error", "error", err)
		}
	}

	return stop, nil
}
