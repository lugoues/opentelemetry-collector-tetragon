package tetragonreceiver

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
)

const componentType = "tetragon"

// NewFactory creates a new receiver factory for the Tetragon receiver.
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		component.MustNewType(componentType),
		createDefaultConfig,
		receiver.WithLogs(createLogsReceiver, component.StabilityLevelAlpha),
	)
}

// tetragonReceiver is the receiver implementation.
// The full implementation (gRPC streaming) is provided in Plan 03.
type tetragonReceiver struct {
	cfg      *Config
	logger   *zap.Logger
	consumer consumer.Logs
	// cancel is initialized to a no-op in the factory to guard against
	// Shutdown-before-Start being called (pre-phase decision, Pitfall 6).
	cancel context.CancelFunc
}

// Start begins receiving events from Tetragon.
// Full implementation provided in Plan 03.
func (r *tetragonReceiver) Start(_ context.Context, _ component.Host) error {
	return nil
}

// Shutdown stops the receiver.
// The no-op cancel guard ensures this is safe to call before Start.
func (r *tetragonReceiver) Shutdown(_ context.Context) error {
	r.cancel()
	return nil
}

func createLogsReceiver(
	_ context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (receiver.Logs, error) {
	rCfg := cfg.(*Config)
	return &tetragonReceiver{
		cfg:      rCfg,
		logger:   settings.Logger,
		consumer: nextConsumer,
		// Initialize to no-op so Shutdown-before-Start is safe (Pitfall 6).
		cancel: func() {},
	}, nil
}
