package tetragonreceiver

import (
	"context"
	"fmt"
	"sync"
	"time"

	backoff "github.com/cenkalti/backoff/v5"
	tetragonv1 "github.com/cilium/tetragon/api/v1/tetragon"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/receiverhelper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

const (
	bufferSize    = 1000
	bufferWarnPct = 0.8 // Log warning when buffer reaches 80% capacity
)

// tetragonClient is a narrow interface covering only the RPCs we use.
// This enables test mocking without a real gRPC server.
type tetragonClient interface {
	GetEvents(ctx context.Context, in *tetragonv1.GetEventsRequest, opts ...grpc.CallOption) (
		tetragonv1.FineGuidanceSensors_GetEventsClient, error)
}

// tetragonReceiver implements the logs receiver for Tetragon events.
type tetragonReceiver struct {
	cfg       *Config
	settings  receiver.Settings // stored for ToClientConn telemetry settings
	logger    *zap.Logger
	consumer  consumer.Logs
	obsReport *receiverhelper.ObsReport
	conn      *grpc.ClientConn
	client    tetragonClient
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	host      component.Host
}

// Start connects to the Tetragon gRPC endpoint, spawns the stream goroutine,
// and returns immediately without blocking.
func (r *tetragonReceiver) Start(ctx context.Context, host component.Host) error {
	r.host = host

	// Skip dial if client was pre-set (test-only path).
	if r.client == nil {
		conn, err := r.cfg.ClientConfig.ToClientConn(ctx, host.GetExtensions(), r.settings.TelemetrySettings)
		if err != nil {
			return fmt.Errorf("failed to create gRPC client connection: %w", err)
		}
		r.conn = conn
		r.client = tetragonv1.NewFineGuidanceSensorsClient(conn)
	}

	// Use context.Background() — never the passed ctx — so the goroutine
	// outlives the Start() call (pre-phase decision, Pitfall 1).
	streamCtx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	r.wg.Add(1)
	go r.streamEvents(streamCtx)

	return nil
}

// Shutdown cancels the stream context, waits for the goroutine to exit,
// and closes the gRPC connection. Safe to call before Start (no-op cancel guard).
func (r *tetragonReceiver) Shutdown(_ context.Context) error {
	r.cancel()
	r.wg.Wait()
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

// streamEvents is the main streaming goroutine. It owns the buffered event
// channel and the consumer goroutine, and reconnects with exponential backoff
// on transient errors.
func (r *tetragonReceiver) streamEvents(ctx context.Context) {
	defer r.wg.Done()

	eventCh := make(chan *tetragonv1.GetEventsResponse, bufferSize)

	// Start the consumer goroutine that drains the channel into the pipeline.
	var consumeWg sync.WaitGroup
	consumeWg.Add(1)
	go func() {
		defer consumeWg.Done()
		r.consumeChannel(ctx, eventCh)
	}()

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = r.cfg.Retry.InitialInterval
	b.MaxInterval = r.cfg.Retry.MaxInterval

	for {
		if ctx.Err() != nil {
			close(eventCh)
			consumeWg.Wait()
			return
		}

		start := time.Now()
		err := r.runStream(ctx, eventCh)
		if ctx.Err() != nil {
			// Clean shutdown — do not reconnect.
			close(eventCh)
			consumeWg.Wait()
			return
		}

		// Only reset backoff after a stream that lasted long enough to indicate
		// a real connection. Without this guard, Reset() on every iteration
		// defeats exponential backoff — the receiver would retry at a fixed
		// InitialInterval instead of 1s -> 2s -> 4s -> ... -> MaxInterval.
		if time.Since(start) > 30*time.Second {
			b.Reset()
		}

		// Transient error — report and schedule retry.
		componentstatus.ReportStatus(r.host,
			componentstatus.NewRecoverableErrorEvent(err))

		wait := b.NextBackOff()

		r.logger.Warn("stream error, reconnecting",
			zap.Error(err),
			zap.Duration("backoff", wait))

		select {
		case <-ctx.Done():
			close(eventCh)
			consumeWg.Wait()
			return
		case <-time.After(wait):
		}
	}
}

// runStream opens a single gRPC stream, reads events into eventCh, and returns
// when the stream ends (either cleanly via ctx cancellation or on error).
func (r *tetragonReceiver) runStream(ctx context.Context, eventCh chan<- *tetragonv1.GetEventsResponse) error {
	stream, err := r.client.GetEvents(ctx, &tetragonv1.GetEventsRequest{})
	if err != nil {
		return fmt.Errorf("failed to open event stream: %w", err)
	}

	r.logger.Info("connected to Tetragon, streaming events")
	componentstatus.ReportStatus(r.host,
		componentstatus.NewEvent(componentstatus.StatusOK))

	var lastBufferWarn time.Time
	const bufferWarnInterval = 10 * time.Second

	for {
		resp, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("stream recv error: %w", err)
		}

		// Log a warning when the buffer nears capacity (80% threshold),
		// rate-limited to avoid flooding under sustained backpressure.
		bufLen := len(eventCh)
		if bufLen >= int(float64(bufferSize)*bufferWarnPct) {
			if time.Since(lastBufferWarn) >= bufferWarnInterval {
				r.logger.Warn("event buffer nearing capacity",
					zap.Int("buffer_len", bufLen),
					zap.Int("buffer_cap", bufferSize),
					zap.Float64("utilization_pct", float64(bufLen)/float64(bufferSize)*100))
				lastBufferWarn = time.Now()
			}
		}

		select {
		case eventCh <- resp:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// consumeChannel drains eventCh, converting each event and forwarding it to the
// next consumer. Decoupled from runStream to prevent backpressure stalls.
// Uses context.Background for ConsumeLogs so in-flight events can complete
// even when the streaming context is cancelled during shutdown.
func (r *tetragonReceiver) consumeChannel(ctx context.Context, eventCh <-chan *tetragonv1.GetEventsResponse) {
	for resp := range eventCh {
		logs := convertEvent(resp)
		obsCtx := r.obsReport.StartLogsOp(context.Background())
		err := r.consumer.ConsumeLogs(obsCtx, logs)
		r.obsReport.EndLogsOp(obsCtx, "tetragon", logs.LogRecordCount(), err)
		if err != nil {
			if consumererror.IsPermanent(err) {
				r.logger.Error("permanent consumer error, dropping logs", zap.Error(err))
			} else {
				r.logger.Warn("transient consumer error", zap.Error(err))
			}
		}
	}
}
