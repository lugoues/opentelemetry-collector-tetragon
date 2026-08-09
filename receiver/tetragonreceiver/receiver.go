package tetragonreceiver

import (
	"context"
	"errors"
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

	// healthyStreamThreshold is how long a stream must stay up before the
	// backoff state and the retry-episode deadline are reset.
	healthyStreamThreshold = 30 * time.Second
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
	// cancelConsume interrupts in-flight ConsumeLogs calls. It is kept separate
	// from cancel so buffered events can still drain during a normal shutdown,
	// while a shutdown that exceeds its deadline can force-stop a blocked consumer.
	cancelConsume context.CancelFunc
	wg            sync.WaitGroup
	host          component.Host
	// healthyThreshold is how long a stream must stay up before backoff state
	// and the retry-episode deadline are reset. Set from healthyStreamThreshold
	// in the factory; overridable in tests.
	healthyThreshold time.Duration
}

// Start connects to the Tetragon gRPC endpoint, spawns the stream goroutine,
// and returns immediately without blocking.
func (r *tetragonReceiver) Start(ctx context.Context, host component.Host) error {
	r.host = host

	// Skip dial if client was pre-set (test-only path).
	if r.client == nil {
		conn, err := r.cfg.ToClientConn(ctx, host.GetExtensions(), r.settings.TelemetrySettings)
		if err != nil {
			return fmt.Errorf("failed to create gRPC client connection: %w", err)
		}
		r.conn = conn
		r.client = tetragonv1.NewFineGuidanceSensorsClient(conn)
	}

	// Use context.Background() — never the passed ctx — so the goroutines
	// outlive the Start() call (pre-phase decision, Pitfall 1).
	streamCtx, streamCancel := context.WithCancel(context.Background())
	consumeCtx, consumeCancel := context.WithCancel(context.Background())
	r.cancel = streamCancel
	r.cancelConsume = consumeCancel

	r.wg.Add(1)
	go r.streamEvents(streamCtx, consumeCtx)

	return nil
}

// Shutdown cancels the stream context, waits for the goroutine to exit,
// and closes the gRPC connection. Safe to call before Start (no-op cancel guard).
// If the shutdown context expires before the goroutines finish, the consume
// context is cancelled to interrupt a blocked downstream ConsumeLogs call and
// the context error is returned so the collector knows shutdown was incomplete.
func (r *tetragonReceiver) Shutdown(ctx context.Context) error {
	r.cancel()

	// Wait for the goroutine to exit, but respect the shutdown context deadline
	// so we don't hang indefinitely if the downstream consumer is broken.
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	var shutdownErr error
	select {
	case <-done:
	case <-ctx.Done():
		// Deadline passed while events were still draining: force-interrupt any
		// in-flight ConsumeLogs call and return promptly to honor the caller's
		// deadline — the goroutines observe the cancellation and exit on their own.
		r.cancelConsume()
		r.logger.Warn("shutdown deadline exceeded, cancelling in-flight consume")
		shutdownErr = ctx.Err()
	}
	r.cancelConsume()

	if r.conn != nil {
		return errors.Join(shutdownErr, r.conn.Close())
	}
	return shutdownErr
}

// streamEvents is the main streaming goroutine. It owns the buffered event
// channel and the consumer goroutine, and reconnects with exponential backoff
// on transient errors.
func (r *tetragonReceiver) streamEvents(ctx, consumeCtx context.Context) {
	defer r.wg.Done()

	eventCh := make(chan *tetragonv1.GetEventsResponse, bufferSize)

	// Start the consumer goroutine that drains the channel into the pipeline.
	var consumeWg sync.WaitGroup
	consumeWg.Add(1)
	go func() {
		defer consumeWg.Done()
		r.consumeChannel(consumeCtx, eventCh)
	}()

	// finish closes the event channel and waits for the consumer to drain it.
	finish := func() {
		close(eventCh)
		consumeWg.Wait()
	}

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = r.cfg.Retry.InitialInterval
	b.MaxInterval = r.cfg.Retry.MaxInterval

	// In cenkalti/backoff/v5, MaxElapsedTime was removed from the struct, so it
	// is enforced manually. It bounds a single retry episode: the clock starts
	// at the first failure after a healthy stream and is cleared once a stream
	// stays up past healthyStreamThreshold, so a receiver that has been running
	// fine for days still gets its full retry window when the stream drops.
	var retryDeadline time.Time

	for {
		if ctx.Err() != nil {
			finish()
			return
		}

		start := time.Now()
		err := r.runStream(ctx, eventCh)
		if ctx.Err() != nil {
			// Clean shutdown — do not reconnect.
			finish()
			return
		}

		// If retry is disabled, fail permanently on the first stream error.
		if !r.cfg.Retry.Enabled {
			r.logger.Error("stream failed and retry is disabled", zap.Error(err))
			componentstatus.ReportStatus(r.host,
				componentstatus.NewPermanentErrorEvent(err))
			finish()
			return
		}

		// Only reset backoff state after a stream that lasted long enough to
		// indicate a real connection. Without this guard, Reset() on every
		// iteration defeats exponential backoff — the receiver would retry at a
		// fixed InitialInterval instead of 1s -> 2s -> 4s -> ... -> MaxInterval.
		if time.Since(start) > r.healthyThreshold {
			b.Reset()
			retryDeadline = time.Time{}
		}
		if r.cfg.Retry.MaxElapsedTime > 0 && retryDeadline.IsZero() {
			retryDeadline = time.Now().Add(r.cfg.Retry.MaxElapsedTime)
		}

		wait := b.NextBackOff()

		// Give up when the episode deadline has passed, or when waiting for the
		// next attempt would overshoot it.
		if !retryDeadline.IsZero() && time.Now().Add(wait).After(retryDeadline) {
			r.logger.Error("max elapsed time reached, giving up", zap.Error(err))
			componentstatus.ReportStatus(r.host,
				componentstatus.NewPermanentErrorEvent(err))
			finish()
			return
		}

		// Transient error — report and schedule retry.
		componentstatus.ReportStatus(r.host,
			componentstatus.NewRecoverableErrorEvent(err))

		r.logger.Warn("stream error, reconnecting",
			zap.Error(err),
			zap.Duration("backoff", wait))

		select {
		case <-ctx.Done():
			finish()
			return
		case <-time.After(wait):
		}
	}
}

// runStream opens a single gRPC stream, reads events into eventCh, and returns
// when the stream ends (either cleanly via ctx cancellation or on error).
func (r *tetragonReceiver) runStream(ctx context.Context, eventCh chan<- *tetragonv1.GetEventsResponse) error {
	stream, err := r.client.GetEvents(ctx, r.cfg.buildGetEventsRequest())
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
// ctx is the consume context, which deliberately outlives the streaming context
// so buffered events can finish exporting during a normal shutdown; it is only
// cancelled when Shutdown's deadline expires (or after Shutdown completes).
func (r *tetragonReceiver) consumeChannel(ctx context.Context, eventCh <-chan *tetragonv1.GetEventsResponse) {
	for resp := range eventCh {
		logs := convertEvent(resp)
		obsCtx := r.obsReport.StartLogsOp(ctx)
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
