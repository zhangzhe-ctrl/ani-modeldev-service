//go:build governance_contract

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	trainingv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/training/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	contractpb "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/protobuf"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The runner selects this provider separately from the Resolve-only provider.
// A direct generated client first accepts control. The consumer must replay
// control over its own mTLS connection, then use the real worker for target.
func TestGovernanceDeliveryContractProvider(t *testing.T) {
	directory, fixture, connection, stop := startGovernanceContractProvider(t)
	intent, err := contractpb.DecodeIntent(fixture.request.Intent)
	if err != nil {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: shared fixture intent cannot decode")
	}
	intentCanonical, intentHash, err := cpup01.CanonicalIntent(intent)
	if err != nil {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: shared fixture intent cannot canonicalize")
	}
	var normalized struct {
		Schema string `json:"schema"`
		cpup01.Intent
	}
	var snapshot cpup01.Snapshot
	if json.Unmarshal(intentCanonical, &normalized) != nil || json.Unmarshal(fixture.wantCanonical, &snapshot) != nil {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: independent canonical fixture cannot decode")
	}
	// The delivery consumer creates its first real Governance binding via CAS.
	// Only its generation differs from the Resolve fixture's fixed generation 7.
	snapshot.Release.AcceptedBindingGeneration = 1
	snapshotCanonical, err := snapshot.Canonical()
	if err != nil {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: first-binding snapshot cannot canonicalize")
	}
	specHash, err := snapshot.Digest()
	if err != nil {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: first-binding snapshot cannot hash")
	}
	handshake := governanceDeliveryHandshake{
		Schema: "ani.cpu-p01.governance-delivery-fixture.v1", Address: fixture.config.Server.Grpc.Addr,
		Frozen: governanceDeliveryFrozen{
			ResourceTenantID: fixture.tenantID, Actor: "governance:user:42", Intent: normalized.Intent,
			Snapshot: snapshot, IntentHash: intentHash, ExecutionSpecHash: specHash,
			AcceptedAt: fixture.request.AcceptedAt.AsTime(),
		},
		Target: governanceDeliveryIdentity{OperationID: uuid.NewString(), ExecutionID: uuid.NewString()},
		Control: governanceDeliveryIdentity{OperationID: uuid.NewString(), ExecutionID: uuid.NewString()},
	}
	handshake.TLS.CAFile = filepath.Join(directory, "ca.pem")
	handshake.TLS.CertFile = filepath.Join(directory, "governance.pem")
	handshake.TLS.KeyFile = filepath.Join(directory, "governance.key")
	control, target := handshake.Frozen.envelope(handshake.Control), handshake.Frozen.envelope(handshake.Target)
	if control.OperationID == target.OperationID || control.ExecutionID == target.ExecutionID {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: control and target identities must differ")
	}
	for _, original := range []cpup01.AdmissionEnvelope{control, target} {
		wantIntent, wantSnapshot, err := original.CanonicalPayloads()
		if err != nil || !bytes.Equal(wantIntent, intentCanonical) || !bytes.Equal(wantSnapshot, snapshotCanonical) {
			t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: immutable fixture envelope differs from authored facts")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wireIntent, intentErr := contractpb.EncodeIntent(control.Intent)
	wireSnapshot, snapshotErr := contractpb.EncodeSnapshot(control.Snapshot)
	if intentErr != nil || snapshotErr != nil {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: valid command cannot encode through shared codec")
	}
	request := &modeldevv1.AcceptExecutionRequest{
		Identity: &trainingv1.ExecutionIdentity{OperationId: control.OperationID, ExecutionId: control.ExecutionID, ExecutionSpecHash: control.SpecHash},
		ResourceTenantId: control.TenantID, AdmittedActorId: control.Actor, IntentHash: control.IntentHash,
		Intent: wireIntent, Snapshot: wireSnapshot, AcceptedAt: timestamppb.New(control.AcceptedAt),
	}
	commandContext := metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-ani-tenant-id", control.TenantID, "x-ani-actor", control.Actor, "x-ani-request-id", uuid.NewString(),
	))
	receipt, err := modeldevv1.NewModelDevCommandServiceClient(connection).AcceptExecution(commandContext, request)
	wantStates := &modeldevv1.ExecutionStates{
		ComputeState: modeldevv1.ComputeState_COMPUTE_STATE_ACCEPTED,
		DeliveryState: modeldevv1.DeliveryState_DELIVERY_STATE_PENDING,
		CloseState: modeldevv1.CloseState_CLOSE_STATE_OPEN,
		ResourceState: modeldevv1.ResourceState_RESOURCE_STATE_NOT_APPLICABLE,
	}
	if err != nil || receipt == nil || !proto.Equal(receipt.Identity, request.Identity) || receipt.Replayed || receipt.Revision != 1 ||
		!proto.Equal(receipt.States, wantStates) || len(receipt.ProtoReflect().GetUnknown()) != 0 {
		t.Fatalf("GOVERNANCE_DELIVERY_PREFLIGHT: direct control Accept failed (code=%s); worker behavior NOT_RUN", status.Code(err))
	}
	observer := fixture.openPool()
	if !governanceDeliveryOriginalPresent(t, ctx, observer, control) || governanceDeliveryOriginalPresent(t, ctx, observer, target) {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: expected one independent durable control and absent target")
	}
	observer.Close()
	cancel()
	raw, err := json.Marshal(handshake)
	if err != nil || len(raw) > 16384 {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: bounded private handshake encoding failed")
	}
	staging := filepath.Join(directory, "handshake.pending")
	if err := os.WriteFile(staging, raw, 0600); err != nil {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: private handshake write failed")
	}
	if err := os.Rename(staging, filepath.Join(directory, "handshake.json")); err != nil {
		t.Fatal("GOVERNANCE_DELIVERY_PREFLIGHT: atomic handshake publication failed")
	}
	t.Log("GOVERNANCE_DELIVERY_PREFLIGHT PASS: real buildApp/mTLS and independent PostgreSQL control Accept; target absent; private handshake published")
	waitGovernanceContractStop(t, directory)
	_ = connection.Close()
	stop()
	finalContext, finalCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer finalCancel()
	finalObserver := fixture.openPool()
	if !governanceDeliveryOriginalPresent(t, finalContext, finalObserver, control) {
		t.Fatal("GOVERNANCE_DELIVERY_BEHAVIOR: original control disappeared")
	}
	targetPresent := governanceDeliveryOriginalPresent(t, finalContext, finalObserver, target)
	storedInput, err := input.New(finalObserver).Get(finalContext, fixture.tenantID, fixture.ready.Import.InputVersionID)
	if err != nil || !reflect.DeepEqual(storedInput, fixture.ready) {
		t.Fatal("GOVERNANCE_DELIVERY_BEHAVIOR: command delivery changed the READY input")
	}
	var identities, executions, closes, submissions, runs int
	err = finalObserver.QueryRow(finalContext, `SELECT
		(SELECT count(*) FROM modeldev_execution_identities),
		(SELECT count(*) FROM modeldev_executions),
		(SELECT count(*) FROM modeldev_close_intents),
		(SELECT count(*) FROM modeldev_pipeline_dispatches),
		(SELECT count(*) FROM modeldev_pipeline_confirmed_runs)`).Scan(&identities, &executions, &closes, &submissions, &runs)
	wantExecutions := 1
	if targetPresent {
		wantExecutions = 2
	}
	if err != nil || identities != wantExecutions || executions != wantExecutions || closes != 0 || submissions != 0 || runs != 0 {
		t.Fatal("GOVERNANCE_DELIVERY_BEHAVIOR: undeclared identity/execution/close/dispatch/Run facts or failed observation")
	}
	requireAdmissionMaterialPoolReleased(t, finalObserver, fixture.applicationName)
	// Absence is a fact, not a worker success. The runner requires absence plus
	// the exact consumer stub failure for RED, and presence plus durable ACK for
	// GREEN. No expectation mode changes what this provider verifies.
	if targetPresent {
		t.Log("GOVERNANCE_DELIVERY_OBSERVATION: target_present; exact control and target; executions=2; close/dispatch/Run=0; READY unchanged; app stopped")
	} else {
		t.Log("GOVERNANCE_DELIVERY_OBSERVATION: target_absent; exact control; executions=1; close/dispatch/Run=0; READY unchanged; app stopped")
	}
}

type governanceDeliveryIdentity struct {
	OperationID string `json:"operation_id"`
	ExecutionID string `json:"execution_id"`
}

type governanceDeliveryFrozen struct {
	ResourceTenantID string `json:"resource_tenant_id"`
	Actor string `json:"actor"`
	Intent cpup01.Intent `json:"intent"`
	Snapshot cpup01.Snapshot `json:"snapshot"`
	IntentHash string `json:"intent_hash"`
	ExecutionSpecHash string `json:"execution_spec_hash"`
	AcceptedAt time.Time `json:"accepted_at"`
}

func (f governanceDeliveryFrozen) envelope(identity governanceDeliveryIdentity) cpup01.AdmissionEnvelope {
	return cpup01.AdmissionEnvelope{
		TenantID: f.ResourceTenantID, Actor: f.Actor, OperationID: identity.OperationID, ExecutionID: identity.ExecutionID,
		Intent: f.Intent, IntentHash: f.IntentHash, Snapshot: f.Snapshot, SpecHash: f.ExecutionSpecHash, AcceptedAt: f.AcceptedAt,
	}
}

type governanceDeliveryHandshake struct {
	Schema string `json:"schema"`
	Address string `json:"address"`
	TLS struct {
		CAFile string `json:"ca_file"`
		CertFile string `json:"cert_file"`
		KeyFile string `json:"key_file"`
	} `json:"tls"`
	Frozen governanceDeliveryFrozen `json:"frozen"`
	Target governanceDeliveryIdentity `json:"target"`
	Control governanceDeliveryIdentity `json:"control"`
}

// Read bytes and scalar fields directly through an independently opened PG
// pool, without using the command response or the production row decoder.
func governanceDeliveryOriginalPresent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, original cpup01.AdmissionEnvelope) bool {
	t.Helper()
	var tenantID, actor, operationID, executionID, intentHash, specHash, revision, closeGeneration string
	var intent, snapshot []byte
	var acceptedAt time.Time
	err := pool.QueryRow(ctx, `SELECT e.tenant_id::text,e.actor,e.operation_id::text,e.execution_id::text,
		e.intent_canonical,e.intent_hash,e.snapshot_canonical,e.spec_hash,e.accepted_at,
		i.owner_revision::text,i.close_generation::text
		FROM modeldev_executions e JOIN modeldev_execution_identities i USING (tenant_id,execution_id)
		WHERE e.tenant_id=$1::uuid AND e.execution_id=$2::uuid`, original.TenantID, original.ExecutionID).
		Scan(&tenantID, &actor, &operationID, &executionID, &intent, &intentHash, &snapshot, &specHash, &acceptedAt, &revision, &closeGeneration)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatal("GOVERNANCE_DELIVERY_OBSERVATION: independent PostgreSQL read failed")
	}
	wantIntent, wantSnapshot, err := original.CanonicalPayloads()
	if err != nil || tenantID != original.TenantID || actor != original.Actor || operationID != original.OperationID || executionID != original.ExecutionID ||
		intentHash != original.IntentHash || specHash != original.SpecHash || !bytes.Equal(intent, wantIntent) || !bytes.Equal(snapshot, wantSnapshot) ||
		!acceptedAt.Equal(original.AcceptedAt) || revision != "1" || closeGeneration != "0" {
		t.Fatal("GOVERNANCE_DELIVERY_OBSERVATION: stored command differs from the complete original canonical identity/payload/time or has unrelated owner facts")
	}
	return true
}
