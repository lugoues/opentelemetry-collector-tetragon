package tetragonreceiver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tetragonv1 "github.com/cilium/tetragon/api/v1/tetragon"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/golden"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatatest/plogtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestConvertEvent_Golden validates converter output for all 11 event types using
// golden file comparison. This is the primary validation strategy per CONTEXT.md.
//
// To regenerate golden files after intentional converter changes:
//
//	UPDATE_GOLDEN=true go test -run TestConvertEvent_Golden ./...
func TestConvertEvent_Golden(t *testing.T) {
	tests := []struct {
		name       string
		fixture    string
		goldenFile string
	}{
		{"process_exec", "testdata/events/process_exec.json", "testdata/golden/process_exec.yaml"},
		{"process_exit", "testdata/events/process_exit.json", "testdata/golden/process_exit.yaml"},
		{"process_kprobe", "testdata/events/process_kprobe.json", "testdata/golden/process_kprobe.yaml"},
		{"process_tracepoint", "testdata/events/process_tracepoint.json", "testdata/golden/process_tracepoint.yaml"},
		{"process_loader", "testdata/events/process_loader.json", "testdata/golden/process_loader.yaml"},
		{"process_uprobe", "testdata/events/process_uprobe.json", "testdata/golden/process_uprobe.yaml"},
		{"process_lsm", "testdata/events/process_lsm.json", "testdata/golden/process_lsm.yaml"},
		{"process_usdt", "testdata/events/process_usdt.json", "testdata/golden/process_usdt.yaml"},
		{"process_throttle", "testdata/events/process_throttle.json", "testdata/golden/process_throttle.yaml"},
		{"rate_limit_info", "testdata/events/rate_limit_info.json", "testdata/golden/rate_limit_info.yaml"},
		{"test", "testdata/events/test.json", "testdata/golden/test.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := os.ReadFile(tt.fixture)
			require.NoError(t, err)

			var resp tetragonv1.GetEventsResponse
			require.NoError(t, protojson.Unmarshal(raw, &resp))

			got := convertEvent(&resp)

			// Update golden files when UPDATE_GOLDEN=true is set.
			if os.Getenv("UPDATE_GOLDEN") == "true" {
				require.NoError(t, os.MkdirAll(filepath.Dir(tt.goldenFile), 0o755))
				require.NoError(t, golden.WriteLogsToFile(tt.goldenFile, got))
			}

			expected, err := golden.ReadLogs(tt.goldenFile)
			require.NoError(t, err)

			// IgnoreObservedTimestamp since ObservedTimestamp is time.Now().
			err = plogtest.CompareLogs(expected, got, plogtest.IgnoreObservedTimestamp())
			require.NoError(t, err)
		})
	}
}

// TestConvertEvent_BodySnakeCase validates that the body uses snake_case field names
// (protojson UseProtoNames: true), not camelCase. This guards the critical OpenObserve
// query compatibility constraint.
func TestConvertEvent_BodySnakeCase(t *testing.T) {
	raw, err := os.ReadFile("testdata/events/process_exec.json")
	require.NoError(t, err)

	var resp tetragonv1.GetEventsResponse
	require.NoError(t, protojson.Unmarshal(raw, &resp))

	got := convertEvent(&resp)

	require.Equal(t, 1, got.ResourceLogs().Len())
	require.Equal(t, 1, got.ResourceLogs().At(0).ScopeLogs().Len())
	require.Equal(t, 1, got.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().Len())

	body := got.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body().Str()
	require.NotEmpty(t, body)

	// Must contain snake_case key "process_exec" (UseProtoNames: true).
	require.True(t, strings.Contains(body, `"process_exec"`),
		"body must contain snake_case key 'process_exec', got: %s", body)

	// Must NOT contain camelCase key "processExec" (would indicate UseProtoNames: false).
	require.False(t, strings.Contains(body, `"processExec"`),
		"body must not contain camelCase key 'processExec', got: %s", body)
}

// TestConvertEvent_ThrottleNoProcess verifies that process_throttle events (which have
// no process field) do not emit tetragon.process.binary attribute.
func TestConvertEvent_ThrottleNoProcess(t *testing.T) {
	raw, err := os.ReadFile("testdata/events/process_throttle.json")
	require.NoError(t, err)

	var resp tetragonv1.GetEventsResponse
	require.NoError(t, protojson.Unmarshal(raw, &resp))

	got := convertEvent(&resp)
	lr := got.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

	_, ok := lr.Attributes().Get("tetragon.process.binary")
	require.False(t, ok, "process_throttle should not have tetragon.process.binary attribute")
}

// TestConvertEvent_RateLimitNoProcess verifies that rate_limit_info events (which have
// no process field) do not emit tetragon.process.binary attribute.
func TestConvertEvent_RateLimitNoProcess(t *testing.T) {
	raw, err := os.ReadFile("testdata/events/rate_limit_info.json")
	require.NoError(t, err)

	var resp tetragonv1.GetEventsResponse
	require.NoError(t, protojson.Unmarshal(raw, &resp))

	got := convertEvent(&resp)
	lr := got.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

	_, ok := lr.Attributes().Get("tetragon.process.binary")
	require.False(t, ok, "rate_limit_info should not have tetragon.process.binary attribute")
}

// TestConvertEvent_UnknownEventType verifies that an unrecognized event type
// (nil event oneof) is handled gracefully — eventTypeName returns "unknown" and
// no process/parent attributes are set.
func TestConvertEvent_UnknownEventType(t *testing.T) {
	resp := &tetragonv1.GetEventsResponse{} // nil event oneof

	assert.Equal(t, "unknown", eventTypeName(resp))
	assert.Nil(t, extractProcess(resp))
	assert.Nil(t, extractParent(resp))

	got := convertEvent(resp)
	lr := got.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

	eventName, ok := lr.Attributes().Get("event.name")
	require.True(t, ok)
	assert.Equal(t, "unknown", eventName.Str())

	_, ok = lr.Attributes().Get("tetragon.process.binary")
	assert.False(t, ok, "unknown event should not have process attributes")
}

// TestConvertEvent_AbsentWrapperFields verifies that optional wrapper fields
// (pid/uid) that are absent on the proto do not produce false zero-valued
// attributes, and that a nil process/parent yields no process attributes at all.
func TestConvertEvent_AbsentWrapperFields(t *testing.T) {
	t.Run("process without pid/uid wrappers", func(t *testing.T) {
		resp := &tetragonv1.GetEventsResponse{
			Event: &tetragonv1.GetEventsResponse_ProcessExec{
				ProcessExec: &tetragonv1.ProcessExec{
					Process: &tetragonv1.Process{Binary: "/bin/nopid"},
					Parent:  &tetragonv1.Process{Binary: "/bin/parent"},
				},
			},
		}
		lr := convertEvent(resp).ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

		_, ok := lr.Attributes().Get("tetragon.process.pid")
		assert.False(t, ok, "absent pid wrapper must not become pid 0")
		_, ok = lr.Attributes().Get("tetragon.process.uid")
		assert.False(t, ok, "absent uid wrapper must not become uid 0")
		_, ok = lr.Attributes().Get("tetragon.parent.pid")
		assert.False(t, ok, "absent parent pid wrapper must not become pid 0")

		binary, ok := lr.Attributes().Get("tetragon.process.binary")
		require.True(t, ok)
		assert.Equal(t, "/bin/nopid", binary.Str())
	})

	t.Run("present zero uid is emitted", func(t *testing.T) {
		resp := &tetragonv1.GetEventsResponse{
			Event: &tetragonv1.GetEventsResponse_ProcessExec{
				ProcessExec: &tetragonv1.ProcessExec{
					Process: &tetragonv1.Process{
						Binary: "/bin/root",
						Uid:    wrapperspb.UInt32(0),
					},
				},
			},
		}
		lr := convertEvent(resp).ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

		uid, ok := lr.Attributes().Get("tetragon.process.uid")
		require.True(t, ok, "explicitly-set uid 0 (root) must be emitted")
		assert.Equal(t, int64(0), uid.Int())
	})

	t.Run("nil process and parent", func(t *testing.T) {
		resp := &tetragonv1.GetEventsResponse{
			Event: &tetragonv1.GetEventsResponse_ProcessExec{
				ProcessExec: &tetragonv1.ProcessExec{},
			},
		}
		lr := convertEvent(resp).ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

		_, ok := lr.Attributes().Get("tetragon.process.binary")
		assert.False(t, ok, "nil process must not emit process attributes")
		_, ok = lr.Attributes().Get("tetragon.parent.binary")
		assert.False(t, ok, "nil parent must not emit parent attributes")
	})

	t.Run("absent event time leaves timestamp unset", func(t *testing.T) {
		resp := &tetragonv1.GetEventsResponse{
			Event: &tetragonv1.GetEventsResponse_ProcessExec{
				ProcessExec: &tetragonv1.ProcessExec{},
			},
		}
		lr := convertEvent(resp).ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0)

		assert.Equal(t, int64(0), lr.Timestamp().AsTime().UnixNano(),
			"missing event time must leave Timestamp at zero, not fabricate one")
		assert.NotZero(t, lr.ObservedTimestamp().AsTime().UnixNano())
	})
}

// TestFixtures_Unmarshal verifies that all 11 JSON fixtures can be successfully
// unmarshaled into tetragonv1.GetEventsResponse, confirming proto-schema fidelity.
func TestFixtures_Unmarshal(t *testing.T) {
	files, err := filepath.Glob("testdata/events/*.json")
	require.NoError(t, err)
	require.Len(t, files, 11, "expected 11 event fixtures")

	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			require.NoError(t, err)
			var resp tetragonv1.GetEventsResponse
			require.NoError(t, protojson.Unmarshal(raw, &resp), "fixture must unmarshal into GetEventsResponse")
		})
	}
}
