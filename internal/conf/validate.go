package conf

import (
	"fmt"
	"strings"
)

// Validate checks that all required configuration fields are set.
// It returns nil when the config is valid, or an error describing all
// problems found (joined so the operator sees everything at once).
func (b *Bootstrap) Validate() error {
	var errs []string

	// --- Server ---
	if b.Server == nil {
		errs = append(errs, "server: missing")
	} else {
		if b.Server.Http == nil || strings.TrimSpace(b.Server.Http.Addr) == "" {
			errs = append(errs, "server.http.addr: required")
		}
		if b.Server.Grpc == nil || strings.TrimSpace(b.Server.Grpc.Addr) == "" {
			errs = append(errs, "server.grpc.addr: required")
		}
	}

	// --- Data ---
	if b.Data == nil {
		errs = append(errs, "data: missing")
	} else {
		if b.Data.Database == nil || strings.TrimSpace(b.Data.Database.Source) == "" {
			errs = append(errs, "data.database.source: required")
		}
		if b.Data.Redis == nil || strings.TrimSpace(b.Data.Redis.Addr) == "" {
			errs = append(errs, "data.redis.addr: required")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// validateSamplingRate checks the telemetry sampling_rate range.
func (t *Telemetry) validateSamplingRate() error {
	if t == nil {
		return nil
	}
	if t.SamplingRate < 0 || t.SamplingRate > 1.0 {
		return fmt.Errorf("telemetry.sampling_rate must be between 0.0 and 1.0, got %f", t.SamplingRate)
	}
	return nil
}

// ValidateTelemetry validates telemetry-specific configuration.
// Called separately from Bootstrap.Validate() because telemetry is optional.
func (b *Bootstrap) ValidateTelemetry() error {
	if b.Telemetry == nil {
		return nil
	}
	return b.Telemetry.validateSamplingRate()
}
