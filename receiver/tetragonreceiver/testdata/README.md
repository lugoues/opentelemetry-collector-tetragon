# Test Data

## Event Fixtures (`events/`)

Proto-schema-faithful synthetic Tetragon event JSON fixtures for converter unit tests.
Format: protojson (snake_case field names, matching `protojson.MarshalOptions{UseProtoNames: true}`).

These fixtures are constructed from the `github.com/cilium/tetragon/api v1.6.0` protobuf
schema to exercise all converter code paths. Every field name and type corresponds to an
actual proto field in `tetragon/api/v1/tetragon/events.proto`.

**Validation:** Each fixture is verified to unmarshal via `protojson.Unmarshal` into
`tetragonv1.GetEventsResponse` as part of the test suite (TestFixtures_Unmarshal).

**To regenerate from a live Tetragon instance:**
```
tetra getevents -o json | head -100 > captured_events.jsonl
# Then split by event type into individual files
```

## Golden Files (`golden/`)

Expected plog.Logs YAML output from the converter, managed by `pkg/golden`.
To update golden files after intentional converter changes, run:
```
UPDATE_GOLDEN=true go test -run TestConvertEvent_Golden ./...
```
