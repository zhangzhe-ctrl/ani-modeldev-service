package cpup01_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
)

func TestAdmissionEnvelopeReturnsImmutableCanonicalPayloads(t *testing.T) {
	snapshot := conformance.SnapshotV1()
	intent := cpup01.Intent{Name: "shared-admission", Kind: snapshot.Kind, PresetID: snapshot.Release.PresetID, DatasetVersionID: snapshot.Input.InputVersionID}
	wantIntent, intentHash, err := cpup01.CanonicalIntent(intent)
	if err != nil { t.Fatal(err) }
	envelope := cpup01.AdmissionEnvelope{
		TenantID: "11111111-2222-4333-8444-555555555555",
		Actor: "governance:user:66666666-7777-4888-8999-aaaaaaaaaaaa",
		OperationID: "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff",
		ExecutionID: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		Intent: intent, IntentHash: intentHash,
		Snapshot: snapshot, SpecHash: conformance.SnapshotSHA256V1,
		AcceptedAt: snapshot.DeadlineAt.Add(-time.Hour),
	}
	gotIntent, gotSnapshot, err := envelope.CanonicalPayloads()
	if err != nil { t.Fatalf("valid frozen admission: %v", err) }
	if !bytes.Equal(gotIntent, wantIntent) || !bytes.Equal(gotSnapshot, conformance.SnapshotCanonicalV1()) {
		t.Fatal("shared validator changed canonical payloads")
	}
}
