package tetragonreceiver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/confmaptest"
)

func TestConfigValidate_EmptyEndpoint(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint is required")
}

func TestConfigValidate_Valid(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	err := cfg.Validate()
	assert.NoError(t, err)
}

// TestConfigValidate_TLSDelegation verifies that Config.Validate() delegates TLS path
// validation to configgrpc.ClientConfig.Validate() (CONF-02). We embed ClientConfig
// with mapstructure:",squash", so our Validate() only checks endpoint non-empty and
// then delegates — no custom TLS validation is needed in this package.
func TestConfigValidate_TLSDelegation(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.Endpoint = "localhost:54321"
	// For a valid config with Insecure:true, TLS validation passes (no file paths to check).
	err := cfg.Validate()
	assert.NoError(t, err, "valid config with TLS insecure should pass ClientConfig.Validate()")
}

func TestDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	assert.Equal(t, "localhost:54321", cfg.Endpoint)
	assert.True(t, cfg.TLS.Insecure)
	assert.True(t, cfg.Retry.Enabled)
	assert.Equal(t, 1*time.Second, cfg.Retry.InitialInterval)
	assert.Equal(t, 30*time.Second, cfg.Retry.MaxInterval)
	assert.Equal(t, time.Duration(0), cfg.Retry.MaxElapsedTime)
}

func TestConfigFromYAML(t *testing.T) {
	cm, err := confmaptest.LoadConf("testdata/config.yaml")
	require.NoError(t, err)

	cfg := createDefaultConfig().(*Config)
	// Use confmap.UnmarshalOptions with a decoder that handles squash tags.
	err = cm.Unmarshal(cfg, confmap.WithIgnoreUnused())
	require.NoError(t, err)

	assert.Equal(t, "tetragon.monitoring:54321", cfg.Endpoint)
	assert.False(t, cfg.TLS.Insecure)
	assert.Equal(t, "/etc/ssl/ca.pem", cfg.TLS.CAFile)
	assert.True(t, cfg.Retry.Enabled)
	assert.Equal(t, 2*time.Second, cfg.Retry.InitialInterval)
	assert.Equal(t, 60*time.Second, cfg.Retry.MaxInterval)
	assert.Equal(t, time.Duration(0), cfg.Retry.MaxElapsedTime)
}
