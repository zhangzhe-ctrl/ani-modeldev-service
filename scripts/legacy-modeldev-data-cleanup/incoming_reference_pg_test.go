package main

import (
	"context"
	"testing"
)

func TestExistingIncomingReferencesAreScopedAndRetained(t *testing.T) {
	f := newCleanupFixture(t)
	f.qualifyRoot(t)
	ctx := context.Background()
	if _, err := f.gov.Exec(ctx, `INSERT INTO sys_gpu_delete_acceptances VALUES(11,2,'other-owner-op','other-owner-op');INSERT INTO sys_gpu_usage_sync(id,tenant_id,operation_id,resource_tenant_id,owner_service,resource_id) SELECT 12,tenant_id,operation_id,resource_tenant_id,owner_service,resource_id FROM sys_quota_operations WHERE id=902`); err != nil {
		t.Fatal(err)
	}
	plan, err := f.command.Plan(ctx, "governance", nil)
	if err != nil || len(plan.Blockers) != 0 || len(plan.Governance.IncomingReferences) != 2 || plan.Governance.IncomingOperationScopeSHA256 == "" {
		t.Fatalf("exact original reference proof: %+v %v", plan.Blockers, err)
	}
	for _, check := range plan.Governance.IncomingReferences {
		if len(check.Rows) != 0 {
			t.Fatalf("foreign owner was included: %+v", check)
		}
	}
	receipt, err := f.command.Apply(ctx, plan, nil)
	if err != nil || !receipt.Applied {
		t.Fatalf("exact deletion with retained references: %v", err)
	}
	var retained int
	if err = f.gov.QueryRow(ctx, `SELECT (SELECT count(*) FROM sys_gpu_delete_acceptances WHERE id=11)+(SELECT count(*) FROM sys_gpu_usage_sync WHERE id=12)`).Scan(&retained); err != nil || retained != 2 {
		t.Fatalf("other owner references changed: %d %v", retained, err)
	}
	f.assertProtected(t)
}

func TestOriginalIncomingReferenceAndPlanDriftRefuseDeletion(t *testing.T) {
	for _, kind := range []string{"create_operation", "delete_operation", "usage_sync"} {
		t.Run(kind, func(t *testing.T) {
			f := newCleanupFixture(t)
			f.qualifyRoot(t)
			ctx := context.Background()
			plan, err := f.command.Plan(ctx, "governance", nil)
			if err != nil || len(plan.Blockers) != 0 {
				t.Fatalf("before reference: %+v %v", plan.Blockers, err)
			}
			statement := `INSERT INTO sys_gpu_delete_acceptances SELECT 21,tenant_id,operation_id,operation_id FROM sys_quota_operations WHERE id=901`
			if kind == "create_operation" {
				statement = `INSERT INTO sys_gpu_delete_acceptances SELECT 21,tenant_id,operation_id,'other-owner-op' FROM sys_quota_operations WHERE id=901`
			}
			if kind == "delete_operation" {
				statement = `INSERT INTO sys_gpu_delete_acceptances SELECT 21,tenant_id,'other-owner-op',operation_id FROM sys_quota_operations WHERE id=901`
			}
			if kind == "usage_sync" {
				statement = `INSERT INTO sys_gpu_usage_sync(id,tenant_id,operation_id,resource_tenant_id,owner_service,resource_id) SELECT 22,tenant_id,operation_id,resource_tenant_id,owner_service,resource_id FROM sys_quota_operations WHERE id=901`
			}
			if _, err = f.gov.Exec(ctx, statement); err != nil {
				t.Fatal(err)
			}
			changed, err := f.command.Plan(ctx, "governance", nil)
			if err != nil || !containsBlocker(changed, "GOV_INCOMING_OPERATION_REFERENCE:") {
				t.Fatalf("incoming original reference accepted: %+v %v", changed.Blockers, err)
			}
			refs := 0
			for _, check := range changed.Governance.IncomingReferences {
				refs += len(check.Rows)
			}
			if refs != 1 {
				t.Fatalf("reference not fixed by exact PK/hash: %+v", changed.Governance.IncomingReferences)
			}
			if _, err = f.command.Apply(ctx, plan, nil); err == nil {
				t.Fatal("new incoming reference bypassed reviewed plan")
			}
			f.requireOriginalCounts(t)
		})
	}
}
