package main

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestProductionInputQueryUsesCurrentDelegation(t *testing.T) {
	fixture := prepareConfiguredAdmission(t)
	connection, _ := startConfiguredAdmissionApp(t, fixture)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := modeldevv1.NewModelDevQueryServiceClient(connection)
	rpc := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-ani-tenant-id", fixture.tenantID, "x-ani-actor", "governance:user:42", "x-ani-request-id", uuid.NewString(), "x-ani-data-scope", "tenant-all", "x-ani-authorized-method", modeldevv1.ModelDevQueryService_GetInputVersion_FullMethodName))
	input, err := client.GetInputVersion(rpc, &modeldevv1.GetInputVersionRequest{InputVersionId: fixture.ready.Import.InputVersionID})
	if err != nil || input.GetInputVersion().GetState() != modeldevv1.InputState_INPUT_STATE_READY {
		t.Fatalf("PRODUCTION_INPUT_QUERY: durable READY unavailable through current delegated query: %v", err)
	}
	if _, err := client.GetInputVersion(ctx, &modeldevv1.GetInputVersionRequest{InputVersionId: fixture.ready.Import.InputVersionID}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthorized query = %v", err)
	}
	other := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-ani-tenant-id", uuid.NewString(), "x-ani-actor", "governance:user:42", "x-ani-request-id", uuid.NewString(), "x-ani-data-scope", "tenant-all", "x-ani-authorized-method", modeldevv1.ModelDevQueryService_GetInputVersion_FullMethodName))
	if _, err := client.GetInputVersion(other, &modeldevv1.GetInputVersionRequest{InputVersionId: fixture.ready.Import.InputVersionID}); status.Code(err) != codes.NotFound {
		t.Fatalf("cross-tenant query = %v", err)
	}
	presets := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-ani-tenant-id", fixture.tenantID, "x-ani-actor", "governance:user:42", "x-ani-request-id", uuid.NewString(), "x-ani-data-scope", "tenant-all", "x-ani-authorized-method", modeldevv1.ModelDevQueryService_ListPresets_FullMethodName))
	list, err := client.ListPresets(presets, &modeldevv1.ListPresetsRequest{})
	if err != nil || len(list.GetPresets()) != 1 || list.Presets[0].ActiveReleaseId != "" {
		t.Fatalf("preset query must expose approved catalogue without a second active pointer: %v", err)
	}
	selection := fixture.request.Release
	management := modeldevv1.NewModelDevManagementServiceClient(connection)
	validate := metadata.NewOutgoingContext(ctx, metadata.Pairs("x-ani-tenant-id", fixture.tenantID, "x-ani-actor", "governance:user:42", "x-ani-request-id", uuid.NewString(), "x-ani-data-scope", "tenant-all", "x-ani-authorized-method", modeldevv1.ModelDevManagementService_ValidateRelease_FullMethodName))
	checked, err := management.ValidateRelease(validate, &modeldevv1.ValidateReleaseRequest{PresetId: list.Presets[0].PresetId, ReleaseId: selection.ReleaseId, ReleaseDigest: selection.ReleaseDigest})
	if err != nil || checked.GetReleaseDigest() != selection.ReleaseDigest {
		t.Fatalf("imported immutable Release validation = %v", err)
	}
	if _, err := management.ValidateRelease(validate, &modeldevv1.ValidateReleaseRequest{PresetId: uuid.NewString(), ReleaseId: selection.ReleaseId, ReleaseDigest: selection.ReleaseDigest}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("wrong preset enabled: %v", err)
	}
	if _, err := management.ValidateRelease(rpc, &modeldevv1.ValidateReleaseRequest{PresetId: list.Presets[0].PresetId, ReleaseId: selection.ReleaseId, ReleaseDigest: selection.ReleaseDigest}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("query delegation authorized management: %v", err)
	}
}
