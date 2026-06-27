package server

import (
	"encoding/json"
	"net/http"

	v1 "github.com/go-kratos/kratos-layout/api/todo/v1"
	"github.com/go-kratos/kratos-layout/internal/conf"
	"github.com/go-kratos/kratos-layout/internal/service"
	"github.com/go-kratos/kratos/contrib/otel/v3/tracing"
	"github.com/go-kratos/kratos/v3/middleware/recovery"
	"github.com/go-kratos/kratos/v3/middleware/validate"
	kratoshttp "github.com/go-kratos/kratos/v3/transport/http"

	"go.einride.tech/aip/fieldbehavior"
	"google.golang.org/protobuf/proto"
)

// serviceName is the meter/tracer scope name shared by both servers.
const serviceName = "kratos.layout"

// NewHTTPServer new an HTTP server.
func NewHTTPServer(c *conf.Server, todo *service.TodoService, metricsHandler http.Handler) *kratoshttp.Server {
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

	// Health check endpoint.
	srv.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	return srv
}
