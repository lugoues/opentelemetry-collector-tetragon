package tetragonreceiver

import (
	"errors"
	"fmt"
	"time"

	tetragonv1 "github.com/cilium/tetragon/api/v1/tetragon"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/config/configtls"
)

// Config defines configuration for the Tetragon receiver.
type Config struct {
	configgrpc.ClientConfig `mapstructure:",squash"`
	Retry                   configretry.BackOffConfig `mapstructure:"retry"`
	// Filters is sent to Tetragon on the GetEvents request, so filtering
	// happens server-side before events cross the wire (saving CPU and
	// network, not just downstream storage). Empty means stream everything.
	Filters FiltersConfig `mapstructure:"filters"`
}

// FiltersConfig maps to Tetragon's GetEventsRequest allow/deny lists. Tetragon
// applies the allow list first, then the deny list. Prefer deny_list for noise
// reduction so newly added event types keep flowing by default.
type FiltersConfig struct {
	AllowList []EventFilter `mapstructure:"allow_list"`
	DenyList  []EventFilter `mapstructure:"deny_list"`
}

// EventFilter is one Tetragon filter. Fields within a filter are ANDed;
// multiple filters in a list are ORed (Tetragon semantics). Only the fields
// useful for this receiver are exposed; extend as needed.
type EventFilter struct {
	// EventSet limits to these event types, e.g. ["PROCESS_EXEC",
	// "PROCESS_EXIT", "PROCESS_KPROBE"]. Names must match Tetragon's
	// EventType enum; unknown names fail validation.
	EventSet []string `mapstructure:"event_set"`
	// BinaryRegex limits to processes whose binary matches any of these
	// regexes (e.g. exclude health-check binaries from a deny list).
	BinaryRegex []string `mapstructure:"binary_regex"`
}

// Validate validates the configuration.
// It returns an error if the endpoint is empty, if the embedded ClientConfig
// is invalid (e.g., invalid TLS paths — CONF-02 handled by ClientConfig.Validate()),
// or if any filter references an unknown event type.
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("endpoint is required")
	}
	if err := c.ClientConfig.Validate(); err != nil {
		return err
	}
	if err := c.Filters.validate(); err != nil {
		return err
	}
	return c.Retry.Validate()
}

func (f FiltersConfig) validate() error {
	for _, list := range [][]EventFilter{f.AllowList, f.DenyList} {
		for _, ef := range list {
			for _, name := range ef.EventSet {
				if _, ok := tetragonv1.EventType_value[name]; !ok {
					return fmt.Errorf("unknown event type %q in filters", name)
				}
			}
		}
	}
	return nil
}

// buildGetEventsRequest translates the configured filters into the Tetragon
// request. A zero-value request (no filters) streams all events.
func (c *Config) buildGetEventsRequest() *tetragonv1.GetEventsRequest {
	return &tetragonv1.GetEventsRequest{
		AllowList: buildFilters(c.Filters.AllowList),
		DenyList:  buildFilters(c.Filters.DenyList),
	}
}

func buildFilters(in []EventFilter) []*tetragonv1.Filter {
	if len(in) == 0 {
		return nil
	}
	out := make([]*tetragonv1.Filter, 0, len(in))
	for _, ef := range in {
		filt := &tetragonv1.Filter{BinaryRegex: ef.BinaryRegex}
		for _, name := range ef.EventSet {
			// Validated in Validate(); default to EVENT_UNDEF if missing.
			filt.EventSet = append(filt.EventSet, tetragonv1.EventType(tetragonv1.EventType_value[name]))
		}
		out = append(out, filt)
	}
	return out
}

// createDefaultConfig returns a *Config with sensible defaults for local development:
// endpoint localhost:54321, TLS disabled, retry enabled with 1s initial / 30s max backoff.
func createDefaultConfig() component.Config {
	cfg := configgrpc.NewDefaultClientConfig()
	cfg.Endpoint = "localhost:54321"
	cfg.TLS = configtls.ClientConfig{Insecure: true}
	return &Config{
		ClientConfig: cfg,
		Retry: configretry.BackOffConfig{
			Enabled:         true,
			InitialInterval: 1 * time.Second,
			MaxInterval:     30 * time.Second,
			MaxElapsedTime:  0,
		},
	}
}
