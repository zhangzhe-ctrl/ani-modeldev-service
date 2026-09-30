package protobuf_test

import (
	"bytes"
	"errors"
	"testing"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	trainingv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/training/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	codec "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/protobuf"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestCodecRealWireRoundTripPreservesPresenceAndDigests(t *testing.T) {
	t.Run("normative intent", func(t *testing.T) {
		input := conformance.IntentV1()
		decoded := intentRoundTrip(t, input)
		canonical, digest, err := cpup01.CanonicalIntent(decoded)
		if err != nil { t.Fatal(err) }
		if !bytes.Equal(canonical, conformance.IntentCanonicalV1()) || digest != conformance.IntentSHA256V1 { t.Fatal("protobuf changed normative intent bytes or digest") }
	})
	t.Run("optional presence", func(t *testing.T) {
		input := conformance.IntentV1()
		input.GeneralParameters = nil
		absent := intentRoundTrip(t, input)
		if absent.GeneralParameters != nil || absent.ImageVersionID != nil || absent.SourceExecutionID != nil { t.Fatal("wire round trip fabricated optional fields") }
		_, absentHash, err := cpup01.CanonicalIntent(absent)
		if err != nil { t.Fatal(err) }
		empty := []cpup01.Parameter{}
		input.GeneralParameters = &empty
		present := intentRoundTrip(t, input)
		if present.GeneralParameters == nil || len(*present.GeneralParameters) != 0 { t.Fatal("wire round trip lost explicit empty parameter selection") }
		_, presentHash, err := cpup01.CanonicalIntent(present)
		if err != nil { t.Fatal(err) }
		if absentHash == presentHash { t.Fatal("wire round trip collapsed intent presence") }
		image := "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
		source := "ffffffff-ffff-4fff-8fff-ffffffffffff"
		input.ImageVersionID, input.SourceExecutionID = &image, &source
		presentIDs := intentRoundTrip(t, input)
		if presentIDs.ImageVersionID == nil || *presentIDs.ImageVersionID != image || presentIDs.SourceExecutionID == nil || *presentIDs.SourceExecutionID != source { t.Fatal("wire round trip lost optional IDs") }
	})
	t.Run("normative frozen snapshot", func(t *testing.T) {
		message, err := codec.EncodeSnapshot(conformance.SnapshotV1())
		if err != nil { t.Fatal(err) }
		wire, err := proto.Marshal(message)
		if err != nil { t.Fatal(err) }
		var received modeldevv1.ExecutionSnapshot
		if err := proto.Unmarshal(wire, &received); err != nil { t.Fatal(err) }
		decoded, err := codec.DecodeSnapshot(&received)
		if err != nil { t.Fatal(err) }
		canonical, err := decoded.Canonical()
		if err != nil { t.Fatal(err) }
		digest, err := decoded.Digest()
		if err != nil { t.Fatal(err) }
		if !bytes.Equal(canonical, conformance.SnapshotCanonicalV1()) || digest != conformance.SnapshotSHA256V1 { t.Fatal("protobuf changed normative snapshot bytes or digest") }
	})
}

func intentRoundTrip(t *testing.T, input cpup01.Intent) cpup01.Intent {
	t.Helper()
	message, err := codec.EncodeIntent(input)
	if err != nil { t.Fatal(err) }
	wire, err := proto.Marshal(message)
	if err != nil { t.Fatal(err) }
	var received modeldevv1.UserIntent
	if err := proto.Unmarshal(wire, &received); err != nil { t.Fatal(err) }
	decoded, err := codec.DecodeIntent(&received)
	if err != nil { t.Fatal(err) }
	return decoded
}

func TestCodecRejectsUnknownWireFactsAndUnsupportedEnums(t *testing.T) {
	unknown := protowire.AppendVarint(protowire.AppendTag(nil, 500, protowire.VarintType), 1)
	intentCases := []struct {
		name string
		change func(*modeldevv1.UserIntent)
	}{
		{"top level unknown", func(m *modeldevv1.UserIntent) { m.ProtoReflect().SetUnknown(unknown) }},
		{"nested unknown", func(m *modeldevv1.UserIntent) { m.GeneralParameters.Values[0].ProtoReflect().SetUnknown(unknown) }},
		{"unknown kind", func(m *modeldevv1.UserIntent) { m.Kind = trainingv1.ExecutionKind(777) }},
		{"unspecified kind", func(m *modeldevv1.UserIntent) { m.Kind = trainingv1.ExecutionKind_EXECUTION_KIND_UNSPECIFIED }},
		{"missing parameter value", func(m *modeldevv1.UserIntent) { m.GeneralParameters.Values[0].Value = nil }},
		{"nil parameter entry", func(m *modeldevv1.UserIntent) { m.GeneralParameters.Values[0] = nil }},
		{"unsupported parameter type", func(m *modeldevv1.UserIntent) { m.GeneralParameters.Values[0].Value = &trainingv1.Parameter_BooleanValue{BooleanValue: true} }},
	}
	for _, test := range intentCases {
		t.Run("intent/"+test.name, func(t *testing.T) {
			message, err := codec.EncodeIntent(conformance.IntentV1())
			if err != nil { t.Fatal(err) }
			test.change(message)
			if _, err := codec.DecodeIntent(message); !errors.Is(err, cpup01.ErrInvalidArgument) { t.Fatalf("unsupported intent wire facts accepted: %v", err) }
		})
	}
	snapshotCases := []struct {
		name string
		change func(*modeldevv1.ExecutionSnapshot)
	}{
		{"top level unknown", func(m *modeldevv1.ExecutionSnapshot) { m.ProtoReflect().SetUnknown(unknown) }},
		{"nested unknown", func(m *modeldevv1.ExecutionSnapshot) { m.Release.Runtime.ProtoReflect().SetUnknown(unknown) }},
		{"timestamp unknown", func(m *modeldevv1.ExecutionSnapshot) { m.DeadlineAt.ProtoReflect().SetUnknown(unknown) }},
		{"unknown execution kind", func(m *modeldevv1.ExecutionSnapshot) { m.Kind = trainingv1.ExecutionKind(777) }},
		{"unknown delivery", func(m *modeldevv1.ExecutionSnapshot) { m.DeliveryMode = trainingv1.DeliveryMode(777) }},
		{"unknown workspace mode", func(m *modeldevv1.ExecutionSnapshot) { m.Workspace.Mode = trainingv1.WorkspaceMode(777) }},
		{"unknown output kind", func(m *modeldevv1.ExecutionSnapshot) { m.OutputContract.OutputKind = trainingv1.OutputKind(777) }},
		{"unknown output role", func(m *modeldevv1.ExecutionSnapshot) { m.OutputContract.RequiredFiles[0].Role = trainingv1.FileRole(777) }},
		{"missing environment", func(m *modeldevv1.ExecutionSnapshot) { m.Environment = nil }},
		{"missing object immutability", func(m *modeldevv1.ExecutionSnapshot) { m.Input.Object.Immutability = nil }},
		{"invalid timestamp nanos", func(m *modeldevv1.ExecutionSnapshot) { m.DeadlineAt.Nanos = 1000000000 }},
	}
	for _, test := range snapshotCases {
		t.Run("snapshot/"+test.name, func(t *testing.T) {
			message, err := codec.EncodeSnapshot(conformance.SnapshotV1())
			if err != nil { t.Fatal(err) }
			test.change(message)
			if _, err := codec.DecodeSnapshot(message); !errors.Is(err, cpup01.ErrInvalidArgument) { t.Fatalf("unsupported snapshot wire facts accepted: %v", err) }
		})
	}
	t.Run("nil and invalid domain values", func(t *testing.T) {
		if _, err := codec.DecodeIntent(nil); !errors.Is(err, cpup01.ErrInvalidArgument) { t.Errorf("nil intent: %v", err) }
		if _, err := codec.DecodeSnapshot(nil); !errors.Is(err, cpup01.ErrInvalidArgument) { t.Errorf("nil snapshot: %v", err) }
		if _, err := codec.EncodeIntent(cpup01.Intent{}); !errors.Is(err, cpup01.ErrInvalidArgument) { t.Errorf("invalid intent: %v", err) }
		if _, err := codec.EncodeSnapshot(cpup01.Snapshot{}); !errors.Is(err, cpup01.ErrInvalidArgument) { t.Errorf("invalid snapshot: %v", err) }
	})
}
