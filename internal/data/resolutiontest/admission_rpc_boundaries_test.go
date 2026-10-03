package resolutiontest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	trainingv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/training/v1"
	contractpb "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/protobuf"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/admissionfacts"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/catalogue"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/service"
	"google.golang.org/grpc/codes"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const admissionRPCRequestID = "abcdefab-1234-4234-8234-abcdefabcdef"

func TestResolveAdmissionRPCRejectsInvalidWireRequests(t *testing.T) {
	fixture := prepareResolution(t)
	reader := loadAdmissionRPCFacts(t, fixture)
	client := startAdmissionClient(t, fixture, biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, reader))
	before := requireAdmissionRPCControl(t, fixture, client)
	unknown := protowire.AppendBytes(protowire.AppendTag(nil, 500, protowire.BytesType), []byte("rpc-untrusted-facts-sentinel"))
	for _, test := range []struct {
		name   string
		change func(*modeldevv1.ResolveAdmissionRequest)
	}{
		{"missing intent", func(r *modeldevv1.ResolveAdmissionRequest) { r.Intent = nil }},
		{"missing Release", func(r *modeldevv1.ResolveAdmissionRequest) { r.Release = nil }},
		{"missing accepted time", func(r *modeldevv1.ResolveAdmissionRequest) { r.AcceptedAt = nil }},
		{"outer unknown", func(r *modeldevv1.ResolveAdmissionRequest) { r.ProtoReflect().SetUnknown(unknown) }},
		{"Release unknown", func(r *modeldevv1.ResolveAdmissionRequest) { r.Release.ProtoReflect().SetUnknown(unknown) }},
		{"timestamp unknown", func(r *modeldevv1.ResolveAdmissionRequest) { r.AcceptedAt.ProtoReflect().SetUnknown(unknown) }},
		{"nested parameter unknown", func(r *modeldevv1.ResolveAdmissionRequest) {
			r.Intent.GeneralParameters.Values[0].ProtoReflect().SetUnknown(unknown)
		}},
		{"unknown kind", func(r *modeldevv1.ResolveAdmissionRequest) { r.Intent.Kind = trainingv1.ExecutionKind(999) }},
		{"zero generation", func(r *modeldevv1.ResolveAdmissionRequest) { r.Release.BindingGeneration = 0 }},
		{"timestamp outside supported years", func(r *modeldevv1.ResolveAdmissionRequest) { r.AcceptedAt.Seconds = 253402300800 }},
		{"submicrosecond time", func(r *modeldevv1.ResolveAdmissionRequest) { r.AcceptedAt.Nanos = 1 }},
		{"zero instant", func(r *modeldevv1.ResolveAdmissionRequest) { r.AcceptedAt = timestamppb.New(time.Time{}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := proto.Clone(admissionRPCRequest(t, fixture)).(*modeldevv1.ResolveAdmissionRequest)
			test.change(request)
			// Do not re-encode through the strict contract codec: rejection must
			// happen after the generated client sends these actual wire facts.
			md, _ := metadata.FromOutgoingContext(admissionRPCContext(fixture.ctx, fixture.selection.TenantID))
			requestID := uuid.NewString()
			md.Set("x-ani-request-id", requestID)
			response, err := client.ResolveAdmission(metadata.NewOutgoingContext(fixture.ctx, md), request)
			requireAdmissionRPCError(t, response, err, codes.InvalidArgument, modeldevv1.ErrorReason_ERROR_REASON_INVALID_ARGUMENT, requestID)
		})
	}
	if after := requireAdmissionRPCControl(t, fixture, client); !proto.Equal(before, after) {
		t.Fatal("rejected wire requests changed the subsequent candidate")
	}
	requireAdmissionRPCUnchanged(t, fixture)
}

func TestResolveAdmissionRPCScopesFactsAndInputsByVerifiedTenant(t *testing.T) {
	fixture := prepareResolution(t)
	other := fixture
	other.selection.TenantID = "99999999-9999-4999-8999-999999999999"
	other.facts.TenantID = other.selection.TenantID
	other.facts.InputScope.ApprovedPrefix = "other/input"
	other.facts.PublicationScope.ApprovedPrefix = "other/artifacts"
	other.facts.Environment.NamespaceName = "module-fixture-other"
	other.facts.Environment.Identities.TenantProxyIdentity = "fixture:other"
	reader := loadAdmissionRPCFacts(t, fixture)
	client := startAdmissionClient(t, fixture, biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, reader))
	original := requireAdmissionRPCControl(t, fixture, client)
	request := admissionRPCRequest(t, fixture)
	otherContext := admissionRPCContext(fixture.ctx, other.selection.TenantID)
	response, err := client.ResolveAdmission(otherContext, request)
	requireAdmissionRPCError(t, response, err, codes.Unavailable, modeldevv1.ErrorReason_ERROR_REASON_ENVIRONMENT_NOT_READY, admissionRPCRequestID)

	// Both tenants now have real pinned facts, but only A has the input row.
	reader = loadAdmissionRPCFacts(t, fixture, other)
	client = startAdmissionClient(t, fixture, biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, reader))
	response, err = client.ResolveAdmission(otherContext, request)
	hidden := requireAdmissionRPCError(t, response, err, codes.NotFound, modeldevv1.ErrorReason_ERROR_REASON_RESOURCE_NOT_FOUND, admissionRPCRequestID)
	missing := proto.Clone(request).(*modeldevv1.ResolveAdmissionRequest)
	missing.Intent.DatasetVersionId = "77777777-7777-4777-8777-777777777777"
	response, err = client.ResolveAdmission(otherContext, missing)
	absent := requireAdmissionRPCError(t, response, err, codes.NotFound, modeldevv1.ErrorReason_ERROR_REASON_RESOURCE_NOT_FOUND, admissionRPCRequestID)
	if !proto.Equal(hidden, absent) {
		t.Fatal("wire error disclosed another tenant's input existence")
	}

	otherImport := fixture.imported
	otherImport.TenantID, otherImport.RequestID = other.selection.TenantID, "88888888-8888-4888-8888-888888888888"
	otherImport.Scope, otherImport.Object.Key = other.facts.InputScope, "other/input/data.csv"
	version := "synthetic-other-tenant-version"
	otherImport.Object.VersionID, otherImport.Object.SHA256 = &version, strings.Repeat("e", 64)
	repository := input.New(fixture.openPool())
	if _, err := repository.FreezeImport(fixture.ctx, otherImport); err != nil {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: tenant B import failed; behavior NOT_RUN: %v", err)
	}
	otherReady, err := repository.RecordVerifiedCSV(fixture.ctx, otherImport, biz.VerifiedCSV{
		VerifiedObject: biz.VerifiedObject{Object: otherImport.Object, VerifiedAt: otherImport.RequestedAt.Add(time.Minute)},
		SchemaVersion:  "ani.cpu.csv.v1", RowCount: 1024, FeatureCount: 16,
	})
	if err != nil || otherReady.State != biz.InputStateReady {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: tenant B READY failed; behavior NOT_RUN: %v", err)
	}
	if recovered, err := input.New(fixture.openPool()).Get(fixture.ctx, otherImport.TenantID, otherImport.InputVersionID); err != nil || !reflect.DeepEqual(recovered, otherReady) {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: tenant B READY recovery failed; behavior NOT_RUN: %v", err)
	}
	response, err = client.ResolveAdmission(otherContext, request)
	if err != nil || response == nil {
		t.Fatalf("tenant B could not resolve its own same-ID input: %v", err)
	}
	snapshot, err := contractpb.DecodeSnapshot(response.Snapshot)
	if err != nil || !reflect.DeepEqual(snapshot.Input.Object, otherImport.Object) || !reflect.DeepEqual(snapshot.Environment, other.facts.Environment) || snapshot.PublicationScope != other.facts.PublicationScope || response.ExecutionSpecHash == original.ExecutionSpecHash {
		t.Fatalf("wire candidate mixed trusted tenant facts or input identities: %v", err)
	}
	canonical, err := snapshot.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	if response.ExecutionSpecHash != hex.EncodeToString(digest[:]) || !proto.Equal(original, requireAdmissionRPCControl(t, fixture, client)) {
		t.Fatal("tenant B candidate digest or tenant A original changed")
	}
	requireAdmissionRPCUnchanged(t, fixture, otherReady)
}

func TestResolveAdmissionRPCMapsFiniteFailures(t *testing.T) {
	fixture := prepareResolution(t)
	reader := loadAdmissionRPCFacts(t, fixture)
	resolver := biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, reader)
	client := startAdmissionClient(t, fixture, resolver)
	requireAdmissionRPCControl(t, fixture, client)
	t.Run("incompatible selected Release", func(t *testing.T) {
		request := admissionRPCRequest(t, fixture)
		request.Intent.PresetId = "77777777-7777-4777-8777-777777777777"
		response, err := client.ResolveAdmission(admissionRPCContext(fixture.ctx, fixture.selection.TenantID), request)
		requireAdmissionRPCError(t, response, err, codes.FailedPrecondition, modeldevv1.ErrorReason_ERROR_REASON_NO_COMPATIBLE_RELEASE, admissionRPCRequestID)
	})
	t.Run("durable input still validating", func(t *testing.T) {
		imported := fixture.imported
		imported.InputVersionID, imported.RequestID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		imported.Object.Key = "tenant/input/not-ready.csv"
		stored, err := input.New(fixture.openPool()).FreezeImport(fixture.ctx, imported)
		if err != nil || stored.State != biz.InputStateValidating || stored.Verification != nil {
			t.Fatalf("ADMISSION_RPC_PREFLIGHT: VALIDATING row failed; behavior NOT_RUN: %v", err)
		}
		request := admissionRPCRequest(t, fixture)
		request.Intent.DatasetVersionId = imported.InputVersionID
		response, err := client.ResolveAdmission(admissionRPCContext(fixture.ctx, fixture.selection.TenantID), request)
		requireAdmissionRPCError(t, response, err, codes.FailedPrecondition, modeldevv1.ErrorReason_ERROR_REASON_INPUT_NOT_READY, admissionRPCRequestID)
		requireAdmissionRPCUnchanged(t, fixture, stored)
	})
	t.Run("actual unavailable catalogue", func(t *testing.T) {
		privatePath := filepath.Join(t.TempDir(), "rpc-private-path-sentinel")
		unavailable := biz.NewManagedAdmissionResolver(catalogue.NewReader(privatePath), fixture.inputReader, reader)
		client := startAdmissionClient(t, fixture, unavailable)
		response, err := client.ResolveAdmission(admissionRPCContext(fixture.ctx, fixture.selection.TenantID), admissionRPCRequest(t, fixture))
		requireAdmissionRPCError(t, response, err, codes.Unavailable, modeldevv1.ErrorReason_ERROR_REASON_UPSTREAM_UNAVAILABLE, admissionRPCRequestID)
		if strings.Contains(err.Error(), privatePath) {
			t.Fatal("wire error exposed the private catalogue path")
		}
	})
	for _, test := range []struct {
		name    string
		failure error
		code    codes.Code
	}{
		{"unknown dependency error", errors.New("rpc-private-dependency-sentinel"), codes.Unavailable},
		{"wrapped cancellation", fmt.Errorf("rpc-private-dependency-sentinel: %w", context.Canceled), codes.Canceled},
		{"wrapped deadline", fmt.Errorf("rpc-private-dependency-sentinel: %w", context.DeadlineExceeded), codes.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := admissionRPCFailingFacts{delegate: reader, failure: test.failure, observed: make(chan struct{})}
			client := startAdmissionClient(t, fixture, biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, failure))
			response, err := client.ResolveAdmission(admissionRPCContext(fixture.ctx, fixture.selection.TenantID), admissionRPCRequest(t, fixture))
			select {
			case <-failure.observed:
			default:
				t.Fatal("ADMISSION_RPC_PREFLIGHT: failure injection did not follow actual facts read; mapping behavior NOT_RUN")
			}
			if fixture.ctx.Err() != nil {
				t.Fatal("ADMISSION_RPC_PREFLIGHT: caller expired before error mapping could be observed; behavior NOT_RUN")
			}
			if test.code == codes.Unavailable {
				requireAdmissionRPCError(t, response, err, test.code, modeldevv1.ErrorReason_ERROR_REASON_UPSTREAM_UNAVAILABLE, admissionRPCRequestID)
			} else if response != nil || status.Code(err) != test.code || status.Convert(err).Message() != errors.Unwrap(test.failure).Error() || len(status.Convert(err).Details()) != 0 {
				t.Fatalf("wrapped context error lost its finite wire status: %v", err)
			}
			if strings.Contains(err.Error(), "rpc-private-dependency-sentinel") {
				t.Fatal("wire error exposed the dependency error text")
			}
		})
	}
	requireAdmissionRPCControl(t, fixture, client)
	requireAdmissionRPCUnchanged(t, fixture)
}

func TestResolveAdmissionRPCCancellationReachesFactsReader(t *testing.T) {
	fixture := prepareResolution(t)
	reader := loadAdmissionRPCFacts(t, fixture)
	blocked := admissionRPCWaitingFacts{delegate: reader, entered: make(chan struct{}), exited: make(chan error, 1)}
	client := startAdmissionClient(t, fixture, biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, blocked))
	ctx, cancel := context.WithCancel(admissionRPCContext(fixture.ctx, fixture.selection.TenantID))
	defer cancel()
	request := admissionRPCRequest(t, fixture)
	type outcome struct {
		response *modeldevv1.ResolveAdmissionResponse
		err      error
	}
	finished := make(chan outcome, 1)
	go func() {
		response, err := client.ResolveAdmission(ctx, request)
		finished <- outcome{response, err}
	}()
	select {
	case <-blocked.entered:
		t.Log("ADMISSION_RPC_BOUNDARY_PREFLIGHT PASS: actual facts read reached over established mTLS before cancellation")
	case <-time.After(3 * time.Second):
		t.Fatal("ADMISSION_RPC_PREFLIGHT: server did not reach actual facts read; cancellation behavior NOT_RUN")
	}
	cancel()
	select {
	case got := <-finished:
		if got.response != nil || status.Code(got.err) != codes.Canceled {
			t.Fatalf("canceled in-flight RPC returned a candidate or wrong status: %v", got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled RPC did not complete within its bound")
	}
	select {
	case err := <-blocked.exited:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("server facts reader stopped for a different cause than client cancellation: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client cancellation did not reach the server's facts reader")
	}
	client = startAdmissionClient(t, fixture, biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, reader))
	requireAdmissionRPCControl(t, fixture, client)
	requireAdmissionRPCUnchanged(t, fixture)
}

func TestResolveAdmissionRPCRequiresConfiguredHandlerAndResolver(t *testing.T) {
	fixture := prepareResolution(t)
	reader := loadAdmissionRPCFacts(t, fixture)
	resolver := biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, reader)
	ctx := admissionRPCContext(fixture.ctx, fixture.selection.TenantID)
	request := admissionRPCRequest(t, fixture)
	connection := startAdmissionConnection(t, fixture, nil)
	response, err := modeldevv1.NewModelDevAdmissionServiceClient(connection).ResolveAdmission(ctx, request)
	if response != nil || status.Code(err) != codes.Unimplemented {
		t.Fatalf("unconfigured admission handler returned a candidate or unexpected status: %v", err)
	}
	if _, err := healthv1.NewHealthClient(connection).Check(ctx, &healthv1.HealthCheckRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("unconfigured admission listener did not retain its live method restriction: %v", err)
	}
	response, err = startAdmissionClient(t, fixture, nil).ResolveAdmission(ctx, request)
	requireAdmissionRPCError(t, response, err, codes.Unavailable, modeldevv1.ErrorReason_ERROR_REASON_ENVIRONMENT_NOT_READY, admissionRPCRequestID)

	// A nil Go request cannot cross the socket as a nil pointer. These are
	// direct adapter checks, not substitutes for the actual wire tests above.
	handler := service.NewAdmission(resolver)
	md, _ := metadata.FromOutgoingContext(ctx)
	for _, unverified := range []context.Context{fixture.ctx, metadata.NewIncomingContext(fixture.ctx, md)} {
		response, err = handler.ResolveAdmission(unverified, request)
		requireAdmissionRPCError(t, response, err, codes.Unauthenticated, modeldevv1.ErrorReason_ERROR_REASON_UNAUTHENTICATED, "")
	}
	verified := service.WithVerifiedGovernanceDelivery(fixture.ctx, service.GovernanceDelivery{TenantID: fixture.selection.TenantID, Actor: "governance:user:42", RequestID: admissionRPCRequestID})
	response, err = handler.ResolveAdmission(verified, nil)
	requireAdmissionRPCError(t, response, err, codes.InvalidArgument, modeldevv1.ErrorReason_ERROR_REASON_INVALID_ARGUMENT, admissionRPCRequestID)

	connection = startAdmissionConnection(t, fixture, handler)
	requireAdmissionRPCControl(t, fixture, modeldevv1.NewModelDevAdmissionServiceClient(connection))
	if _, err := healthv1.NewHealthClient(connection).Check(ctx, &healthv1.HealthCheckRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("admission registration broadened the registered health capability: %v", err)
	}
	for _, method := range []string{modeldevv1.ModelDevQueryService_GetExecution_FullMethodName, modeldevv1.ModelDevStepService_BeginExecution_FullMethodName} {
		if err := connection.Invoke(ctx, method, &emptypb.Empty{}, &emptypb.Empty{}); status.Code(err) != codes.Unimplemented {
			t.Fatalf("admission listener registered an unrelated capability %s: %v", method, err)
		}
	}
	requireAdmissionRPCUnchanged(t, fixture)
}

func TestResolveAdmissionRPCRejectsUntrustedMetadata(t *testing.T) {
	fixture := prepareResolution(t)
	reader := loadAdmissionRPCFacts(t, fixture)
	client := startAdmissionClient(t, fixture, biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, reader))
	requireAdmissionRPCControl(t, fixture, client)
	for _, test := range []struct {
		name   string
		change func(metadata.MD)
	}{
		{"missing tenant", func(md metadata.MD) { md.Delete("x-ani-tenant-id") }},
		{"duplicate tenant", func(md metadata.MD) { md.Append("x-ani-tenant-id", fixture.selection.TenantID) }},
		{"duplicate request ID", func(md metadata.MD) { md.Append("x-ani-request-id", admissionRPCRequestID) }},
		{"user authorization", func(md metadata.MD) { md.Set("authorization", "rpc-untrusted-credential-sentinel") }},
		{"ambient delegation", func(md metadata.MD) { md.Set("ani-delegation", "rpc-untrusted-credential-sentinel") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			md, _ := metadata.FromOutgoingContext(admissionRPCContext(fixture.ctx, fixture.selection.TenantID))
			test.change(md)
			response, err := client.ResolveAdmission(metadata.NewOutgoingContext(fixture.ctx, md), admissionRPCRequest(t, fixture))
			if response != nil || status.Code(err) != codes.Unauthenticated || strings.Contains(err.Error(), "rpc-untrusted-credential-sentinel") {
				t.Fatalf("untrusted metadata reached admission or leaked its content: %v", err)
			}
		})
	}
	requireAdmissionRPCControl(t, fixture, client)
	requireAdmissionRPCUnchanged(t, fixture)
}

func admissionRPCRequest(t *testing.T, fixture resolutionFixture) *modeldevv1.ResolveAdmissionRequest {
	t.Helper()
	intent, err := contractpb.EncodeIntent(fixture.selection.Intent)
	if err != nil {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: request encoding failed; behavior NOT_RUN: %v", err)
	}
	return &modeldevv1.ResolveAdmissionRequest{Intent: intent, Release: &modeldevv1.AdmissionReleaseSelection{
		ReleaseId: fixture.selection.Release.ReleaseID, ReleaseDigest: fixture.selection.Release.ReleaseDigest, BindingGeneration: fixture.selection.Release.BindingGeneration,
	}, AcceptedAt: timestamppb.New(fixture.selection.AcceptedAt)}
}

func admissionRPCContext(ctx context.Context, tenant string) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("x-ani-tenant-id", tenant, "x-ani-actor", "governance:user:42", "x-ani-request-id", admissionRPCRequestID))
}

func loadAdmissionRPCFacts(t *testing.T, fixtures ...resolutionFixture) *admissionfacts.Reader {
	t.Helper()
	var sources []admissionfacts.FileSource
	for _, fixture := range fixtures {
		sources = append(sources, writeManagedFactsFixture(t, fixture))
	}
	reader, err := admissionfacts.Load(fixtures[0].ctx, sources)
	if err != nil {
		t.Fatalf("ADMISSION_RPC_PREFLIGHT: facts load failed; behavior NOT_RUN: %v", err)
	}
	for _, fixture := range fixtures {
		got, err := reader.ReadAdmissionFacts(fixture.ctx, fixture.selection.TenantID, fixture.selection.Release.ReleaseID, fixture.selection.Release.ReleaseDigest)
		if err != nil || !reflect.DeepEqual(got, fixture.facts) {
			t.Fatalf("ADMISSION_RPC_PREFLIGHT: pinned facts mismatch; behavior NOT_RUN: %v", err)
		}
	}
	return reader
}

func requireAdmissionRPCControl(t *testing.T, fixture resolutionFixture, client modeldevv1.ModelDevAdmissionServiceClient) *modeldevv1.ResolveAdmissionResponse {
	t.Helper()
	response, err := client.ResolveAdmission(admissionRPCContext(fixture.ctx, fixture.selection.TenantID), admissionRPCRequest(t, fixture))
	if err != nil || response == nil {
		t.Fatalf("healthy RPC control failed: %v", err)
	}
	got, err := contractpb.DecodeSnapshot(response.Snapshot)
	want := expectedManagedSnapshot(fixture)
	canonical, canonicalErr := want.Canonical()
	digest := sha256.Sum256(canonical)
	if err != nil || canonicalErr != nil || !reflect.DeepEqual(got, want) || response.ExecutionSpecHash != hex.EncodeToString(digest[:]) {
		t.Fatalf("healthy RPC control changed the independent snapshot/digest: decode=%v expected=%v", err, canonicalErr)
	}
	t.Log("ADMISSION_RPC_BOUNDARY_PREFLIGHT PASS: real file/PG facts and authenticated RPC control")
	return response
}

func requireAdmissionRPCError(t *testing.T, response *modeldevv1.ResolveAdmissionResponse, err error, code codes.Code, reason modeldevv1.ErrorReason, correlation string) *modeldevv1.ErrorDetail {
	t.Helper()
	if response != nil || status.Code(err) != code {
		t.Fatalf("RPC failure returned a candidate or wrong code: %v", err)
	}
	failure := status.Convert(err)
	details := failure.Details()
	if len(details) != 1 {
		t.Fatalf("RPC failure lacks one typed safe detail: %v", err)
	}
	detail, ok := details[0].(*modeldevv1.ErrorDetail)
	messages := map[modeldevv1.ErrorReason]string{
		modeldevv1.ErrorReason_ERROR_REASON_INVALID_ARGUMENT:      "invalid admission resolution request",
		modeldevv1.ErrorReason_ERROR_REASON_UNAUTHENTICATED:       "authenticated Governance delivery required",
		modeldevv1.ErrorReason_ERROR_REASON_NO_COMPATIBLE_RELEASE: "selected Release is unavailable or incompatible",
		modeldevv1.ErrorReason_ERROR_REASON_RESOURCE_NOT_FOUND:    "input version unavailable",
		modeldevv1.ErrorReason_ERROR_REASON_INPUT_NOT_READY:       "input version is not ready",
		modeldevv1.ErrorReason_ERROR_REASON_ENVIRONMENT_NOT_READY: "managed admission environment is not ready",
		modeldevv1.ErrorReason_ERROR_REASON_UPSTREAM_UNAVAILABLE:  "admission resolution unavailable",
	}
	if !ok || detail.Reason != reason || detail.CorrelationId != correlation || detail.SafeMessage != messages[reason] || failure.Message() != messages[reason] || len(detail.Violations) != 0 {
		t.Fatalf("RPC failure changed its finite reason, correlation or safe message: %v", err)
	}
	return detail
}

func requireAdmissionRPCUnchanged(t *testing.T, fixture resolutionFixture, extra ...biz.InputVersion) {
	t.Helper()
	pool := fixture.openPool()
	var executions, identities, closes, dispatches int
	err := pool.QueryRow(fixture.ctx, `SELECT (SELECT count(*) FROM modeldev_executions),
		(SELECT count(*) FROM modeldev_execution_identities), (SELECT count(*) FROM modeldev_close_intents),
		(SELECT count(*) FROM modeldev_pipeline_dispatches)`).Scan(&executions, &identities, &closes, &dispatches)
	if err != nil || executions != 0 || identities != 0 || closes != 0 || dispatches != 0 {
		t.Fatalf("read-only RPC changed command facts: execution=%d identity=%d close=%d dispatch=%d error=%v", executions, identities, closes, dispatches, err)
	}
	for _, want := range append([]biz.InputVersion{fixture.ready}, extra...) {
		got, err := input.New(pool).Get(fixture.ctx, want.Import.TenantID, want.Import.InputVersionID)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("RPC changed durable input: %v", err)
		}
	}
}

// Failure injection still delegates the actual pinned facts read first. These
// wrappers never provide fake facts or represent a production adapter.
type admissionRPCFailingFacts struct {
	delegate biz.AdmissionFactsReader
	failure  error
	observed chan struct{}
}

func (reader admissionRPCFailingFacts) ReadAdmissionFacts(ctx context.Context, tenant, release, digest string) (biz.TenantAdmissionFacts, error) {
	if _, err := reader.delegate.ReadAdmissionFacts(ctx, tenant, release, digest); err != nil {
		return biz.TenantAdmissionFacts{}, err
	}
	close(reader.observed)
	return biz.TenantAdmissionFacts{}, reader.failure
}

type admissionRPCWaitingFacts struct {
	delegate biz.AdmissionFactsReader
	entered  chan struct{}
	exited   chan error
}

func (reader admissionRPCWaitingFacts) ReadAdmissionFacts(ctx context.Context, tenant, release, digest string) (biz.TenantAdmissionFacts, error) {
	if _, err := reader.delegate.ReadAdmissionFacts(ctx, tenant, release, digest); err != nil {
		return biz.TenantAdmissionFacts{}, err
	}
	close(reader.entered)
	<-ctx.Done()
	err := ctx.Err()
	reader.exited <- err
	return biz.TenantAdmissionFacts{}, err
}
