package tetragonreceiver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()
	assert.Equal(t, component.MustNewType(componentType), factory.Type())
}

func TestCreateDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	require.NotNil(t, cfg)
	_, ok := cfg.(*Config)
	assert.True(t, ok, "expected *Config type")
}

func TestCreateLogsReceiver(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	recv, err := factory.CreateLogs(
		context.Background(),
		receivertest.NewNopSettings(component.MustNewType(componentType)),
		cfg,
		consumertest.NewNop(),
	)
	require.NoError(t, err)
	assert.NotNil(t, recv)
}

func TestShutdownBeforeStart(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	recv, err := factory.CreateLogs(
		context.Background(),
		receivertest.NewNopSettings(component.MustNewType(componentType)),
		cfg,
		consumertest.NewNop(),
	)
	require.NoError(t, err)

	// Shutdown without Start should not panic and return no error.
	assert.NotPanics(t, func() {
		err := recv.Shutdown(context.Background())
		assert.NoError(t, err)
	})
}
