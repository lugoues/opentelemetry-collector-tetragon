package tetragonreceiver

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tetragonv1 "github.com/cilium/tetragon/api/v1/tetragon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ---- Mock implementations ----

// mockGetEventsClient implements grpc.ServerStreamingClient[tetragonv1.GetEventsResponse],
// i.e., tetragonv1.FineGuidanceSensors_GetEventsClient.
type mockGetEventsClient struct {
	mu        sync.Mutex
	responses []*tetragonv1.GetEventsResponse
	idx       int
	err       error
	blockCtx  context.Context // non-nil: block until Done instead of returning EOF
	streamCtx context.Context // set by mockTetragonClient.GetEvents; mirrors real gRPC ctx
}

// Recv returns the next response or blocks/errors per configuration.
// When blockCtx is set, Recv blocks until either blockCtx or streamCtx is Done.
func (m *mockGetEventsClient) Recv() (*tetragonv1.GetEventsResponse, error) {
	m.mu.Lock()
	if m.err != nil {
		err := m.err
		m.mu.Unlock()
		return nil, err
	}
	if m.idx < len(m.responses) {
		resp := m.responses[m.idx]
		m.idx++
		m.mu.Unlock()
		return resp, nil
	}
	m.mu.Unlock()

	if m.blockCtx != nil {
		// Block until either the explicit block context or the stream context is done.
		// This mirrors real gRPC behaviour: cancelling the call context unblocks Recv.
		streamCtx := m.streamCtx
		if streamCtx == nil {
			streamCtx = context.Background()
		}
		select {
		case <-m.blockCtx.Done():
			return nil, m.blockCtx.Err()
		case <-streamCtx.Done():
			return nil, streamCtx.Err()
		}
	}
	return nil, io.EOF
}

// grpc.ClientStream implementation (required by grpc.ServerStreamingClient).
func (m *mockGetEventsClient) Header() (metadata.MD, error) { return nil, nil }
func (m *mockGetEventsClient) Trailer() metadata.MD         { return nil }
func (m *mockGetEventsClient) CloseSend() error             { return nil }
func (m *mockGetEventsClient) Context() context.Context {
	if m.streamCtx != nil {
		return m.streamCtx
	}
	return context.Background()
}
func (m *mockGetEventsClient) SendMsg(_ any) error { return nil }
func (m *mockGetEventsClient) RecvMsg(_ any) error { return nil }

// mockTetragonClient implements tetragonClient.
type mockTetragonClient struct {
	mu           sync.Mutex
	stream       tetragonv1.FineGuidanceSensors_GetEventsClient
	getEventsErr error
	callCount    int
}

func (m *mockTetragonClient) GetEvents(ctx context.Context, _ *tetragonv1.GetEventsRequest, _ ...grpc.CallOption) (
	tetragonv1.FineGuidanceSensors_GetEventsClient, error,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.getEventsErr != nil {
		return nil, m.getEventsErr
	}
	// Thread the call context into the stream so Recv() can unblock on cancellation.
	if mc, ok := m.stream.(*mockGetEventsClient); ok {
		mc.streamCtx = ctx
	}
	return m.stream, nil
}

// ---- Test helper ----

// newTestReceiver creates a tetragonReceiver wired through the factory (to get obsReport),
// then replaces the client with the provided mock.
func newTestReceiver(t *testing.T, client tetragonClient) (*tetragonReceiver, *consumertest.LogsSink) {
	t.Helper()
	sink := &consumertest.LogsSink{}
	factory := NewFactory()
	settings := receivertest.NewNopSettings(component.MustNewType(componentType))
	cfg := factory.CreateDefaultConfig().(*Config)

	recv, err := createLogsReceiver(context.Background(), settings, cfg, sink)
	require.NoError(t, err)

	r := recv.(*tetragonReceiver)
	r.client = client
	return r, sink
}

// waitForLogs uses assert.Eventually to wait until at least minRecords LogRecords
// have arrived in the sink.
func waitForLogs(t *testing.T, sink *consumertest.LogsSink, minRecords int, timeout time.Duration) bool {
	t.Helper()
	return assert.Eventually(t, func() bool {
		return totalRecords(sink) >= minRecords
	}, timeout, 10*time.Millisecond)
}

// ---- Tests ----

// TestReceiverStartShutdown verifies Start() returns immediately (non-blocking)
// and Shutdown() completes cleanly.
func TestReceiverStartShutdown(t *testing.T) {
	blockCtx, blockCancel := context.WithCancel(context.Background())
	defer blockCancel()

	streamClient := &mockGetEventsClient{blockCtx: blockCtx}
	mockClient := &mockTetragonClient{stream: streamClient}

	r, _ := newTestReceiver(t, mockClient)

	err := r.Start(context.Background(), componenttest.NewNopHost())
	require.NoError(t, err, "Start should return nil immediately")

	done := make(chan error, 1)
	go func() {
		done <- r.Shutdown(context.Background())
	}()

	select {
	case err := <-done:
		assert.NoError(t, err, "Shutdown should return nil")
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown timed out — goroutine likely stuck")
	}
}

// TestReceiverShutdownBeforeStart verifies calling Shutdown without Start is safe.
func TestReceiverShutdownBeforeStart(t *testing.T) {
	r, _ := newTestReceiver(t, &mockTetragonClient{})

	assert.NotPanics(t, func() {
		err := r.Shutdown(context.Background())
		assert.NoError(t, err)
	})
}

// TestReceiverStreamEvents verifies that events arriving on the gRPC stream
// are forwarded through the buffered channel to the consumer.
func TestReceiverStreamEvents(t *testing.T) {
	responses := []*tetragonv1.GetEventsResponse{
		makeExecResponse("/bin/test1"),
		makeExecResponse("/bin/test2"),
		makeExecResponse("/bin/test3"),
	}
	streamClient := &mockGetEventsClient{responses: responses}
	mockClient := &mockTetragonClient{stream: streamClient}

	r, sink := newTestReceiver(t, mockClient)
	require.NoError(t, r.Start(context.Background(), componenttest.NewNopHost()))

	ok := waitForLogs(t, sink, 3, 5*time.Second)
	assert.True(t, ok, "expected 3 log records, got %d", totalRecords(sink))

	require.NoError(t, r.Shutdown(context.Background()))
}

// TestReceiverReconnectsOnStreamError verifies that a GetEvents error triggers
// reconnection and events eventually flow through.
func TestReceiverReconnectsOnStreamError(t *testing.T) {
	// First call returns error; second returns a stream with one event.
	successStream := &mockGetEventsClient{responses: []*tetragonv1.GetEventsResponse{
		makeExecResponse("/bin/curl"),
	}}

	var callCount atomic.Int32
	client := &mockTetragonClientFn{
		fn: func(_ context.Context, _ *tetragonv1.GetEventsRequest, _ ...grpc.CallOption) (
			tetragonv1.FineGuidanceSensors_GetEventsClient, error,
		) {
			n := callCount.Add(1)
			if n == 1 {
				return nil, io.EOF
			}
			return successStream, nil
		},
	}

	r, sink := newTestReceiver(t, client)
	require.NoError(t, r.Start(context.Background(), componenttest.NewNopHost()))

	ok := waitForLogs(t, sink, 1, 10*time.Second)
	assert.True(t, ok, "expected 1 log record after reconnect")
	assert.GreaterOrEqual(t, callCount.Load(), int32(2), "expected at least 2 GetEvents calls")

	require.NoError(t, r.Shutdown(context.Background()))
}

// TestReceiverCleanShutdownDuringBackoff verifies that Shutdown unblocks
// a receiver waiting in exponential backoff.
func TestReceiverCleanShutdownDuringBackoff(t *testing.T) {
	// Client always errors — receiver will enter backoff loop.
	// Use callCount to detect when the receiver has attempted at least one
	// connection, which means it has entered the backoff loop.
	var callCount atomic.Int32
	client := &mockTetragonClientFn{
		fn: func(_ context.Context, _ *tetragonv1.GetEventsRequest, _ ...grpc.CallOption) (
			tetragonv1.FineGuidanceSensors_GetEventsClient, error,
		) {
			callCount.Add(1)
			return nil, io.ErrUnexpectedEOF
		},
	}

	r, _ := newTestReceiver(t, client)
	require.NoError(t, r.Start(context.Background(), componenttest.NewNopHost()))

	// Wait until the receiver has attempted at least one connection (entered backoff).
	require.Eventually(t, func() bool {
		return callCount.Load() >= 1
	}, 5*time.Second, 5*time.Millisecond, "receiver should attempt at least one connection")

	done := make(chan error, 1)
	go func() {
		done <- r.Shutdown(context.Background())
	}()

	select {
	case err := <-done:
		assert.NoError(t, err, "Shutdown should return nil even during backoff")
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown timed out — receiver is stuck in backoff sleep")
	}
}

// TestReceiverRetryDisabled verifies that when retry is disabled, the receiver
// stops permanently after the first stream error instead of reconnecting.
func TestReceiverRetryDisabled(t *testing.T) {
	var callCount atomic.Int32
	called := make(chan struct{}, 1)
	client := &mockTetragonClientFn{
		fn: func(_ context.Context, _ *tetragonv1.GetEventsRequest, _ ...grpc.CallOption) (
			tetragonv1.FineGuidanceSensors_GetEventsClient, error,
		) {
			callCount.Add(1)
			select {
			case called <- struct{}{}:
			default:
			}
			return nil, io.ErrUnexpectedEOF
		},
	}

	r, _ := newTestReceiver(t, client)
	r.cfg.Retry.Enabled = false
	require.NoError(t, r.Start(context.Background(), componenttest.NewNopHost()))

	// Wait until at least one GetEvents call has been made.
	select {
	case <-called:
	case <-time.After(5 * time.Second):
		t.Fatal("receiver never called GetEvents")
	}

	// The goroutine should exit on its own after the first error.
	done := make(chan error, 1)
	go func() {
		done <- r.Shutdown(context.Background())
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown timed out — receiver should have exited after first error")
	}

	assert.Equal(t, int32(1), callCount.Load(), "should only attempt one connection when retry is disabled")
}

// TestReceiverMaxElapsedTime verifies that a non-zero MaxElapsedTime causes the
// receiver to stop retrying after the configured duration. The streamEvents
// goroutine should exit on its own without Shutdown being called.
func TestReceiverMaxElapsedTime(t *testing.T) {
	var callCount atomic.Int32
	client := &mockTetragonClientFn{
		fn: func(_ context.Context, _ *tetragonv1.GetEventsRequest, _ ...grpc.CallOption) (
			tetragonv1.FineGuidanceSensors_GetEventsClient, error,
		) {
			callCount.Add(1)
			return nil, io.ErrUnexpectedEOF
		},
	}

	r, _ := newTestReceiver(t, client)
	r.cfg.Retry.Enabled = true
	r.cfg.Retry.InitialInterval = 10 * time.Millisecond
	r.cfg.Retry.MaxInterval = 50 * time.Millisecond
	r.cfg.Retry.MaxElapsedTime = 500 * time.Millisecond
	require.NoError(t, r.Start(context.Background(), componenttest.NewNopHost()))

	// Wait for the goroutine to exit on its own (MaxElapsedTime exceeded).
	// The wg.Wait() will return once streamEvents exits.
	exited := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(exited)
	}()

	select {
	case <-exited:
		// streamEvents exited on its own due to MaxElapsedTime.
	case <-time.After(5 * time.Second):
		t.Fatal("streamEvents did not exit after MaxElapsedTime")
	}

	// Should have retried multiple times before giving up.
	calls := callCount.Load()
	assert.Greater(t, calls, int32(1), "should have retried at least once before MaxElapsedTime")
	assert.Less(t, calls, int32(200), "should have stopped retrying after MaxElapsedTime")

	// Clean up — Shutdown should be a no-op now.
	require.NoError(t, r.Shutdown(context.Background()))
}

// TestReceiverShutdownRespectsContext verifies that Shutdown returns promptly when
// its context deadline is reached, even if the goroutine hasn't finished.
func TestReceiverShutdownRespectsContext(t *testing.T) {
	// Create a stream that blocks forever, simulating a stuck downstream.
	blockCtx, blockCancel := context.WithCancel(context.Background())
	defer blockCancel()

	streamClient := &mockGetEventsClient{blockCtx: blockCtx}
	mockClient := &mockTetragonClient{stream: streamClient}

	r, _ := newTestReceiver(t, mockClient)
	require.NoError(t, r.Start(context.Background(), componenttest.NewNopHost()))

	// Shutdown with a very short deadline.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer shutdownCancel()

	done := make(chan error, 1)
	go func() {
		done <- r.Shutdown(shutdownCtx)
	}()

	select {
	case <-done:
		// Shutdown returned — it respected the context deadline.
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown blocked beyond context deadline")
	}

	// Clean up: unblock the stream so the goroutine can exit.
	blockCancel()
}

// TestReceiverConsumesLogs verifies event attributes flow through the pipeline correctly.
func TestReceiverConsumesLogs(t *testing.T) {
	responses := []*tetragonv1.GetEventsResponse{
		makeExecResponse("/usr/bin/curl"),
	}
	streamClient := &mockGetEventsClient{responses: responses}
	mockClient := &mockTetragonClient{stream: streamClient}

	r, sink := newTestReceiver(t, mockClient)
	require.NoError(t, r.Start(context.Background(), componenttest.NewNopHost()))

	ok := waitForLogs(t, sink, 1, 5*time.Second)
	require.True(t, ok, "expected 1 log record")

	require.NoError(t, r.Shutdown(context.Background()))

	allLogs := sink.AllLogs()
	require.NotEmpty(t, allLogs)

	rl := allLogs[0].ResourceLogs().At(0)
	sl := rl.ScopeLogs().At(0)
	lr := sl.LogRecords().At(0)

	attrs := lr.Attributes()
	eventName, ok := attrs.Get("event.name")
	assert.True(t, ok, "expected event.name attribute")
	assert.Equal(t, "process_exec", eventName.Str())

	binary, ok := attrs.Get("tetragon.process.binary")
	assert.True(t, ok, "expected tetragon.process.binary attribute")
	assert.Equal(t, "/usr/bin/curl", binary.Str())
}

// ---- Helpers ----

// makeExecResponse creates a minimal GetEventsResponse with a ProcessExec event.
func makeExecResponse(binary string) *tetragonv1.GetEventsResponse {
	return &tetragonv1.GetEventsResponse{
		Event: &tetragonv1.GetEventsResponse_ProcessExec{
			ProcessExec: &tetragonv1.ProcessExec{
				Process: &tetragonv1.Process{
					Binary: binary,
				},
			},
		},
	}
}

// totalRecords counts all log records across all LogData in the sink.
func totalRecords(sink *consumertest.LogsSink) int {
	total := 0
	for _, ld := range sink.AllLogs() {
		total += ld.LogRecordCount()
	}
	return total
}

// mockTetragonClientFn allows injecting a custom GetEvents function.
type mockTetragonClientFn struct {
	fn func(context.Context, *tetragonv1.GetEventsRequest, ...grpc.CallOption) (
		tetragonv1.FineGuidanceSensors_GetEventsClient, error)
}

func (m *mockTetragonClientFn) GetEvents(ctx context.Context, in *tetragonv1.GetEventsRequest, opts ...grpc.CallOption) (
	tetragonv1.FineGuidanceSensors_GetEventsClient, error,
) {
	return m.fn(ctx, in, opts...)
}
