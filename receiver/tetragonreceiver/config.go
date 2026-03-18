package tetragonreceiver

import (
	"errors"
	"time"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/component"
)

// Config defines configuration for the Tetragon receiver.
type Config struct {
	configgrpc.ClientConfig `mapstructure:",squash"`
	Retry                   configretry.BackOffConfig `mapstructure:"retry"`
}

// Validate validates the configuration.
// It returns an error if the endpoint is empty, or if the embedded ClientConfig
// is invalid (e.g., invalid TLS paths — CONF-02 handled by ClientConfig.Validate()).
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("endpoint is required")
	}
	if err := c.ClientConfig.Validate(); err != nil {
		return err
	}
	return c.Retry.Validate()
}

// createDefaultConfig creates the default configuration for the tetragon receiver.
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
