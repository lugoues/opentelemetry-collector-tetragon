package tetragonreceiver

import (
	"context"

	"github.com/cilium/otelcol-tetragon/receiver/tetragonreceiver/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/receiverhelper"
)

// NewFactory creates a new receiver factory for the Tetragon receiver.
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		metadata.Type,
		createDefaultConfig,
		receiver.WithLogs(createLogsReceiver, metadata.LogsStability),
	)
}

func createLogsReceiver(
	_ context.Context,
	settings receiver.Settings,
	cfg component.Config,
	nextConsumer consumer.Logs,
) (receiver.Logs, error) {
	rCfg := cfg.(*Config)

	obsReport, err := receiverhelper.NewObsReport(receiverhelper.ObsReportSettings{
		ReceiverID:             settings.ID,
		Transport:              "grpc",
		ReceiverCreateSettings: settings,
	})
	if err != nil {
		return nil, err
	}

	return &tetragonReceiver{
		cfg:       rCfg,
		settings:  settings, // stored for ToClientConn telemetry settings
		logger:    settings.Logger,
		consumer:  nextConsumer,
		obsReport: obsReport,
		// Initialize to no-op so Shutdown-before-Start is safe (Pitfall 6).
		cancel: func() {},
	}, nil
}
