package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func bindFixtureSync(t *testing.T, f *cleanupFixture) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.gov.Exec(ctx, `INSERT INTO sys_gpu_usage_sync(id,tenant_id,operation_id,resource_tenant_id,owner_service,resource_id,revision,state,payload_json,payload_hash,acked_revision,lease_owner,lease_until,retry_blocked) SELECT 31,tenant_id,operation_id,resource_tenant_id,owner_service,resource_id,2,'ENDED','{"revision":2,"state":2}','original-payload-sha',2,'original-acked-worker',NULL,false FROM sys_quota_operations WHERE id=901`); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := f.gov.QueryRow(ctx, `SELECT to_jsonb(t) FROM sys_gpu_usage_sync t WHERE id=31`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var r row
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	f.command.Original.SyncIDs = []int64{31}
	f.command.Original.OriginalSync = []OriginalSyncIdentity{{ID: 31, TenantID: 2, OperationID: text(r, "operation_id"), ResourceTenantID: text(r, "resource_tenant_id"), OwnerService: text(r, "owner_service"), ResourceID: text(r, "resource_id"), PayloadSHA256: text(r, "payload_hash"), RowSHA256: bytesSHA(raw)}}
	f.command.Original.SyncInventorySHA256 = digest(f.command.Original.OriginalSync)
}
func TestAckedEndedOriginalSyncDeletesBeforeOperationAndPreservesOtherOwner(t *testing.T) {
	f := newCleanupFixture(t)
	f.qualifyRoot(t)
	bindFixtureSync(t, f)
	ctx := context.Background()
	if _, err := f.gov.Exec(ctx, `INSERT INTO sys_gpu_usage_sync(id,tenant_id,operation_id,resource_tenant_id,owner_service,resource_id) SELECT 32,tenant_id,operation_id,resource_tenant_id,owner_service,resource_id FROM sys_quota_operations WHERE id=902`); err != nil {
		t.Fatal(err)
	}
	plan, err := f.command.Plan(ctx, "governance", nil)
	if err != nil || len(plan.Blockers) != 0 {
		t.Fatalf("original ended ACK plan: %+v %v", plan.Blockers, err)
	}
	receipt, err := f.command.Apply(ctx, plan, nil)
	if err != nil || !receipt.Applied || receipt.Deleted["sys_gpu_usage_sync"] != 1 {
		t.Fatalf("exact sync-before-operation deletion: %+v %v", receipt.Deleted, err)
	}
	var count int
	if err = f.gov.QueryRow(ctx, `SELECT count(*) FROM sys_gpu_usage_sync WHERE id=32`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("other sync owner changed %d %v", count, err)
	}
	f.assertProtected(t)
	f.command.GovernanceReceiptSHA256 = digest(receipt)
	next, err := f.command.Plan(ctx, "modeldev", &receipt)
	if err != nil || len(next.Blockers) != 0 {
		t.Fatalf("sync deletion receipt lost: %+v %v", next.Blockers, err)
	}
}
func TestSyncUnackedLeasePayloadDriftAndMissingManifestRefuse(t *testing.T) {
	for _, kind := range []string{"unacked", "lease", "retry", "payload", "missing_manifest"} {
		t.Run(kind, func(t *testing.T) {
			f := newCleanupFixture(t)
			f.qualifyRoot(t)
			bindFixtureSync(t, f)
			ctx := context.Background()
			plan, err := f.command.Plan(ctx, "governance", nil)
			if err != nil || len(plan.Blockers) != 0 {
				t.Fatalf("before drift: %+v %v", plan.Blockers, err)
			}
			updates := map[string]string{"unacked": "acked_revision=1", "lease": "lease_until=clock_timestamp()+interval '30 seconds'", "retry": "retry_blocked=true", "payload": "payload_json='{}'"}
			if kind == "missing_manifest" {
				f.command.Original.SyncInventorySHA256 = ""
			} else if _, err = f.gov.Exec(ctx, `UPDATE sys_gpu_usage_sync SET `+updates[kind]+` WHERE id=31`); err != nil {
				t.Fatal(err)
			}
			changed, err := f.command.Plan(ctx, "governance", nil)
			if err != nil || (!containsBlocker(changed, "GOV_ORIGINAL_SYNC_") && !containsBlocker(changed, "GOV_INCOMING_")) {
				t.Fatalf("unsafe sync accepted: %+v %v", changed.Blockers, err)
			}
			if _, err = f.command.Apply(ctx, plan, nil); err == nil {
				t.Fatal("reviewed sync plan accepted after drift")
			}
			f.requireOriginalCounts(t)
		})
	}
}
func TestFixedOriginal23SyncManifestRejectsForeignPKOwnerAndHash(t *testing.T) {
	scope := OriginalScope{TenantID: legacyTenant, GovernanceTenantID: 2}
	manifest := OriginalSyncManifest{Schema: "ani.modeldev.original23-usage-sync-manifest.v1", GovernanceInventorySHA256: originalGovInventorySHA}
	for i, id := range originalSyncIDs {
		op := fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1)
		resource := fmt.Sprintf("00000000-0000-4000-8001-%012d", i+1)
		raw, _ := json.Marshal(map[string]any{"operation_id": op, "resource_id": resource})
		scope.OriginalOperations = append(scope.OriginalOperations, raw)
		manifest.Rows = append(manifest.Rows, OriginalSyncIdentity{ID: id, TenantID: 2, OperationID: op, ResourceTenantID: legacyTenant, OwnerService: "ani-modeldev-service", ResourceID: resource, PayloadSHA256: strings.Repeat("a", 64), RowSHA256: strings.Repeat("b", 64)})
	}
	for _, kind := range []string{"original", "foreign_pk", "owner", "row_hash", "file_hash"} {
		t.Run(kind, func(t *testing.T) {
			candidate := manifest
			candidate.Rows = append([]OriginalSyncIdentity{}, manifest.Rows...)
			if kind == "foreign_pk" {
				candidate.Rows[0].ID = 999
			}
			if kind == "owner" {
				candidate.Rows[0].OwnerService = "ani-inference-service"
			}
			if kind == "row_hash" {
				candidate.Rows[0].RowSHA256 = ""
			}
			raw, _ := json.Marshal(candidate)
			p := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(p, raw, 0600); err != nil {
				t.Fatal(err)
			}
			expected := bytesSHA(raw)
			if kind == "file_hash" {
				expected = strings.Repeat("0", 64)
			}
			copy := scope
			err := loadOriginalSync(p, expected, &copy)
			if kind == "original" && err != nil || kind != "original" && err == nil {
				t.Fatalf("fixed manifest %s: %v", kind, err)
			}
		})
	}
}
