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

// waitForLogs polls sink.AllLogs() until at least minRecords LogRecords have arrived
// or the timeout is exceeded.
func waitForLogs(t *testing.T, sink *consumertest.LogsSink, minRecords int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		total := 0
		for _, ld := range sink.AllLogs() {
			total += ld.LogRecordCount()
		}
		if total >= minRecords {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
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

	var callCount int32
	client := &mockTetragonClientFn{
		fn: func(_ context.Context, _ *tetragonv1.GetEventsRequest, _ ...grpc.CallOption) (
			tetragonv1.FineGuidanceSensors_GetEventsClient, error,
		) {
			n := atomic.AddInt32(&callCount, 1)
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
	assert.GreaterOrEqual(t, atomic.LoadInt32(&callCount), int32(2), "expected at least 2 GetEvents calls")

	require.NoError(t, r.Shutdown(context.Background()))
}

// TestReceiverCleanShutdownDuringBackoff verifies that Shutdown unblocks
// a receiver waiting in exponential backoff.
func TestReceiverCleanShutdownDuringBackoff(t *testing.T) {
	// Client always errors — receiver will enter backoff loop.
	client := &mockTetragonClient{getEventsErr: io.ErrUnexpectedEOF}

	r, _ := newTestReceiver(t, client)
	require.NoError(t, r.Start(context.Background(), componenttest.NewNopHost()))

	// Give the receiver time to enter backoff.
	time.Sleep(100 * time.Millisecond)

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
