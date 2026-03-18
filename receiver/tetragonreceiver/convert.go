package tetragonreceiver

import (
	"bytes"
	"encoding/json"
	"time"

	tetragonv1 "github.com/cilium/tetragon/api/v1/tetragon"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"google.golang.org/protobuf/encoding/protojson"
)

// jsonMarshaler marshals Tetragon proto messages using proto field names (snake_case).
// UseProtoNames: true is critical — without it, protojson uses camelCase which breaks
// OpenObserve queries that expect Tetragon's native JSON format.
// EmitUnpopulated: false (default) omits zero-value fields for compact output.
var jsonMarshaler = protojson.MarshalOptions{UseProtoNames: true}

// convertEvent converts a single Tetragon GetEventsResponse into a plog.Logs containing
// exactly one LogRecord. The log body is the protojson-serialized event (snake_case fields).
func convertEvent(resp *tetragonv1.GetEventsResponse) plog.Logs {
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName("tetragonreceiver")
	lr := sl.LogRecords().AppendEmpty()

	// Timestamp: use event time field; ObservedTimestamp is receive time.
	if t := resp.GetTime(); t != nil {
		lr.SetTimestamp(pcommon.NewTimestampFromTime(t.AsTime()))
	}
	lr.SetObservedTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	// Body: protojson marshal of the full GetEventsResponse.
	// json.Compact normalizes whitespace — protojson's output format is not
	// guaranteed to be stable across calls, so we compact to canonical form.
	raw, err := jsonMarshaler.Marshal(resp)
	if err != nil {
		lr.Body().SetStr("error marshaling event: " + err.Error())
	} else {
		var buf bytes.Buffer
		if compactErr := json.Compact(&buf, raw); compactErr != nil {
			lr.Body().SetStr(string(raw))
		} else {
			lr.Body().SetStr(buf.String())
		}
	}

	// Severity: based on event type.
	setSeverity(lr, resp)

	// Attributes.
	attrs := lr.Attributes()
	// Receiver-specific attributes (not OTel semantic conventions).
	// event.domain and event.name follow the OTel event model naming pattern
	// but are not part of the official semantic conventions registry.
	attrs.PutStr("event.domain", "tetragon")
	attrs.PutStr("event.name", eventTypeName(resp))

	// Process attributes.
	proc := extractProcess(resp)
	if proc != nil {
		attrs.PutStr("tetragon.process.binary", proc.GetBinary())
		attrs.PutStr("tetragon.process.arguments", proc.GetArguments())
		attrs.PutInt("tetragon.process.pid", int64(proc.GetPid().GetValue()))
		attrs.PutInt("tetragon.process.uid", int64(proc.GetUid().GetValue()))
		attrs.PutStr("tetragon.process.exec_id", proc.GetExecId())
		attrs.PutStr("tetragon.process.cwd", proc.GetCwd())

		// Kubernetes attributes (when pod info is present).
		if pod := proc.GetPod(); pod != nil {
			attrs.PutStr("k8s.namespace.name", pod.GetNamespace())
			attrs.PutStr("k8s.pod.name", pod.GetName())
			if container := pod.GetContainer(); container != nil {
				attrs.PutStr("k8s.container.name", container.GetName())
			}
		}
	}

	// Parent attributes.
	parent := extractParent(resp)
	if parent != nil {
		attrs.PutStr("tetragon.parent.binary", parent.GetBinary())
		attrs.PutInt("tetragon.parent.pid", int64(parent.GetPid().GetValue()))
		attrs.PutStr("tetragon.parent.exec_id", parent.GetExecId())
	}

	// Event-specific attributes.
	extractEventAttrs(attrs, resp)

	return logs
}

// setSeverity sets the severity on the log record based on event type.
func setSeverity(lr plog.LogRecord, resp *tetragonv1.GetEventsResponse) {
	switch resp.GetEvent().(type) {
	case *tetragonv1.GetEventsResponse_ProcessExec,
		*tetragonv1.GetEventsResponse_ProcessExit,
		*tetragonv1.GetEventsResponse_ProcessLoader:
		lr.SetSeverityNumber(plog.SeverityNumberInfo)
		lr.SetSeverityText("INFO")
	case *tetragonv1.GetEventsResponse_ProcessKprobe,
		*tetragonv1.GetEventsResponse_ProcessTracepoint,
		*tetragonv1.GetEventsResponse_ProcessLsm,
		*tetragonv1.GetEventsResponse_ProcessUprobe,
		*tetragonv1.GetEventsResponse_ProcessUsdt:
		lr.SetSeverityNumber(plog.SeverityNumberWarn)
		lr.SetSeverityText("WARN")
	case *tetragonv1.GetEventsResponse_ProcessThrottle,
		*tetragonv1.GetEventsResponse_RateLimitInfo:
		lr.SetSeverityNumber(plog.SeverityNumberError)
		lr.SetSeverityText("ERROR")
	}
}

// eventTypeName returns the snake_case event type name for the given response.
func eventTypeName(resp *tetragonv1.GetEventsResponse) string {
	switch resp.GetEvent().(type) {
	case *tetragonv1.GetEventsResponse_ProcessExec:
		return "process_exec"
	case *tetragonv1.GetEventsResponse_ProcessExit:
		return "process_exit"
	case *tetragonv1.GetEventsResponse_ProcessKprobe:
		return "process_kprobe"
	case *tetragonv1.GetEventsResponse_ProcessTracepoint:
		return "process_tracepoint"
	case *tetragonv1.GetEventsResponse_ProcessLoader:
		return "process_loader"
	case *tetragonv1.GetEventsResponse_ProcessUprobe:
		return "process_uprobe"
	case *tetragonv1.GetEventsResponse_ProcessLsm:
		return "process_lsm"
	case *tetragonv1.GetEventsResponse_ProcessUsdt:
		return "process_usdt"
	case *tetragonv1.GetEventsResponse_ProcessThrottle:
		return "process_throttle"
	case *tetragonv1.GetEventsResponse_RateLimitInfo:
		return "rate_limit_info"
	default:
		return "unknown"
	}
}

// extractProcess returns the Process from any event type that has one.
// ProcessThrottle and RateLimitInfo return nil.
func extractProcess(resp *tetragonv1.GetEventsResponse) *tetragonv1.Process {
	switch e := resp.GetEvent().(type) {
	case *tetragonv1.GetEventsResponse_ProcessExec:
		return e.ProcessExec.GetProcess()
	case *tetragonv1.GetEventsResponse_ProcessExit:
		return e.ProcessExit.GetProcess()
	case *tetragonv1.GetEventsResponse_ProcessKprobe:
		return e.ProcessKprobe.GetProcess()
	case *tetragonv1.GetEventsResponse_ProcessTracepoint:
		return e.ProcessTracepoint.GetProcess()
	case *tetragonv1.GetEventsResponse_ProcessLoader:
		return e.ProcessLoader.GetProcess()
	case *tetragonv1.GetEventsResponse_ProcessUprobe:
		return e.ProcessUprobe.GetProcess()
	case *tetragonv1.GetEventsResponse_ProcessLsm:
		return e.ProcessLsm.GetProcess()
	case *tetragonv1.GetEventsResponse_ProcessUsdt:
		return e.ProcessUsdt.GetProcess()
	default:
		return nil
	}
}

// extractParent returns the parent Process from event types that have one.
func extractParent(resp *tetragonv1.GetEventsResponse) *tetragonv1.Process {
	switch e := resp.GetEvent().(type) {
	case *tetragonv1.GetEventsResponse_ProcessExec:
		return e.ProcessExec.GetParent()
	case *tetragonv1.GetEventsResponse_ProcessExit:
		return e.ProcessExit.GetParent()
	case *tetragonv1.GetEventsResponse_ProcessKprobe:
		return e.ProcessKprobe.GetParent()
	case *tetragonv1.GetEventsResponse_ProcessTracepoint:
		return e.ProcessTracepoint.GetParent()
	case *tetragonv1.GetEventsResponse_ProcessUprobe:
		return e.ProcessUprobe.GetParent()
	case *tetragonv1.GetEventsResponse_ProcessLsm:
		return e.ProcessLsm.GetParent()
	case *tetragonv1.GetEventsResponse_ProcessUsdt:
		return e.ProcessUsdt.GetParent()
	default:
		return nil
	}
}

// extractEventAttrs sets event-specific attributes on the attribute map.
func extractEventAttrs(attrs pcommon.Map, resp *tetragonv1.GetEventsResponse) {
	switch e := resp.GetEvent().(type) {
	case *tetragonv1.GetEventsResponse_ProcessKprobe:
		k := e.ProcessKprobe
		attrs.PutStr("tetragon.policy_name", k.GetPolicyName())
		attrs.PutStr("tetragon.action", k.GetAction().String())
		attrs.PutStr("tetragon.function_name", k.GetFunctionName())
	case *tetragonv1.GetEventsResponse_ProcessTracepoint:
		tp := e.ProcessTracepoint
		attrs.PutStr("tetragon.policy_name", tp.GetPolicyName())
		attrs.PutStr("tetragon.action", tp.GetAction().String())
		attrs.PutStr("tetragon.subsys", tp.GetSubsys())
		attrs.PutStr("tetragon.event", tp.GetEvent())
	case *tetragonv1.GetEventsResponse_ProcessLsm:
		l := e.ProcessLsm
		attrs.PutStr("tetragon.policy_name", l.GetPolicyName())
		attrs.PutStr("tetragon.action", l.GetAction().String())
		attrs.PutStr("tetragon.function_name", l.GetFunctionName())
	case *tetragonv1.GetEventsResponse_ProcessUprobe:
		u := e.ProcessUprobe
		attrs.PutStr("tetragon.policy_name", u.GetPolicyName())
		attrs.PutStr("tetragon.action", u.GetAction().String())
		attrs.PutStr("tetragon.function_name", u.GetSymbol())
	case *tetragonv1.GetEventsResponse_ProcessUsdt:
		ud := e.ProcessUsdt
		attrs.PutStr("tetragon.policy_name", ud.GetPolicyName())
		attrs.PutStr("tetragon.action", ud.GetAction().String())
	case *tetragonv1.GetEventsResponse_ProcessExit:
		ex := e.ProcessExit
		attrs.PutInt("tetragon.exit.status", int64(ex.GetStatus()))
		attrs.PutStr("tetragon.exit.signal", ex.GetSignal())
	}
}
