package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
)

func originalRows(raws []json.RawMessage) []row {
	out := []row{}
	for _, raw := range raws {
		var r row
		_ = json.Unmarshal(raw, &r)
		out = append(out, r)
	}
	return out
}
func find(rows []row, key, value string) row {
	for _, r := range rows {
		if text(r, key) == value {
			return r
		}
	}
	return nil
}
func fieldsMatch(current, original row, keys ...string) bool {
	if current == nil || original == nil {
		return false
	}
	for _, key := range keys {
		if text(current, key) != text(original, key) {
			return false
		}
	}
	return true
}
func admitted(r row) (biz.Execution, error) {
	a := biz.Admission{TenantID: text(r, "tenant_id"), ExecutionID: text(r, "execution_id"), OperationID: text(r, "operation_id"), Actor: text(r, "actor"), IntentHash: text(r, "intent_hash"), SpecHash: text(r, "spec_hash")}
	var err error
	a.AcceptedAt, err = time.Parse(time.RFC3339Nano, text(r, "accepted_at"))
	if err != nil {
		return biz.Execution{}, err
	}
	ir, err := hex.DecodeString(strings.TrimPrefix(text(r, "intent_canonical"), "\\x"))
	if err != nil {
		return biz.Execution{}, err
	}
	sr, err := hex.DecodeString(strings.TrimPrefix(text(r, "snapshot_canonical"), "\\x"))
	if err != nil {
		return biz.Execution{}, err
	}
	a.Intent, err = cpup01.DecodeCanonicalIntent(ir)
	if err != nil {
		return biz.Execution{}, err
	}
	a.Snapshot, err = cpup01.DecodeCanonicalSnapshot(sr)
	if err != nil {
		return biz.Execution{}, err
	}
	intent, snapshot, err := a.CanonicalPayloads()
	if err != nil || string(intent) != string(ir) || string(snapshot) != string(sr) {
		return biz.Execution{}, fmt.Errorf("original canonical admission invalid")
	}
	return biz.Execution{Admission: a}, nil
}
func (c *CleanupCommand) qualifyMD(ctx context.Context, tx pgx.Tx, v databaseView) []string {
	blockers := []string{}
	s := c.Original
	executions := table(v, "modeldev_executions").rows
	identities := table(v, "modeldev_execution_identities").rows
	runtimes := table(v, "modeldev_execution_runtimes").rows
	if len(executions) != len(s.Executions) || len(identities) != len(s.Executions) || len(runtimes) != len(s.Executions) {
		blockers = append(blockers, "MD_ORIGINAL_AGGREGATE_ROWCOUNT_DIFFERS")
	}
	for _, wanted := range s.Executions {
		er := find(executions, "execution_id", wanted.ExecutionID)
		identity := find(identities, "execution_id", wanted.ExecutionID)
		runtime := find(runtimes, "execution_id", wanted.ExecutionID)
		good := true
		for _, r := range []row{er, identity, runtime} {
			if r == nil || text(r, "tenant_id") != wanted.TenantID || text(r, "operation_id") != wanted.OperationID || text(r, "spec_hash") != wanted.SpecHash {
				good = false
			}
		}
		if !good {
			blockers = append(blockers, "MD_ORIGINAL_IDENTITY_DIFFERS:"+wanted.ExecutionID)
			continue
		}
		execution, err := admitted(er)
		if err != nil {
			blockers = append(blockers, "MD_ORIGINAL_CANONICAL_INVALID:"+wanted.ExecutionID)
			continue
		}
		var state biz.ExecutionRuntime
		_, retirement := biz.LookupLegacyRetirementScope(wanted.ExecutionID)
		if retirement {
			// This maintenance reader is finite to the original three and performs the
			// same complete legacy qualification as the retirement writer. Under apply's
			// table locks a separate read connection cannot observe a changing aggregate.
			observed, st, _, owner, fence, e := lifecycle.NewLegacyRetirement(c.ModelDev).GetLegacyRetirementRecord(ctx, wanted.TenantID, wanted.ExecutionID)
			if e != nil || observed.SpecHash != wanted.SpecHash || st.OwnerRevision != uint64(integer(identity, "owner_revision")) || fence != uint64(integer(identity, "close_generation")) || !biz.LegacyRetirementComputeCleanupApplied(observed, owner, st) {
				blockers = append(blockers, "MD_LEGACY_RETIREMENT_NOT_QUALIFIED:"+wanted.ExecutionID)
				continue
			}
			state = st
			if execution.Snapshot.ComputeOrigin.DeviceMode == "gpu" {
				prior := state.LegacyRetirement.PriorGPURelease
				if !biz.ValidLegacyRetirementPriorGPURelease(execution, owner, prior) || state.GPURelease != nil || !null(runtime, "gpu_release_sealed_at") || !null(runtime, "gpu_release_scope_sha256") || prior.ObservedAt.Before(state.ComputeCleanup.CompletedAt) {
					blockers = append(blockers, "MD_ORIGINAL_GPU_JOURNAL_NOT_QUALIFIED:"+wanted.ExecutionID)
				}
			} else if state.LegacyRetirement.PriorGPURelease != nil {
				blockers = append(blockers, "MD_CPU_HAS_FOREIGN_GPU_JOURNAL:"+wanted.ExecutionID)
			}
		} else {
			state, err = lifecycle.ReadInTransaction(ctx, tx, wanted.TenantID, wanted.ExecutionID)
			if err != nil {
				blockers = append(blockers, "MD_NORMAL_OWNER_READER_REJECTED:"+wanted.ExecutionID)
				continue
			}
			if state.LegacyRetirement != nil {
				blockers = append(blockers, "MD_UNAPPROVED_RETIREMENT_MARKER:"+wanted.ExecutionID)
				continue
			}
			noResources := state.Workspace == nil && state.Training == nil && state.TrainingHandle == nil && state.Observation == nil && state.Publication == nil
			if state.CloseEvidence != nil && !state.CloseEvidence.NoDispatch && !noResources {
				authority, readErr := submission.ReadAuthorityInTransaction(ctx, tx, wanted.TenantID, wanted.ExecutionID)
				owner := authority.RunAuthorityCandidate
				if readErr != nil && state.CloseAuthority != nil {
					owner = *state.CloseAuthority
					readErr = nil
				}
				if readErr != nil || state.StepExitCleanup == nil || state.StepExitCleanup.CompletedAt == nil || !biz.ComputeCleanupApplied(biz.OperationRecord{QueryRecord: biz.QueryRecord{Execution: execution, Runtime: state}, Authority: &owner}) {
					blockers = append(blockers, "MD_CLEANUP_NOT_APPLIED:"+wanted.ExecutionID)
				}
			}
			if execution.Snapshot.ComputeOrigin.DeviceMode == "gpu" {
				fact := state.GPURelease
				if fact == nil || fact.Outbox == nil || fact.Outbox.ACK == nil || fact.Outbox.DeliveredAt == nil {
					blockers = append(blockers, "MD_ORIGINAL_GPU_OUTBOX_ACK_MISSING:"+wanted.ExecutionID)
				} else {
					e, a := fact.Outbox.Event, *fact.Outbox.ACK
					if e.CreateOperationID != wanted.OperationID || e.ResourceID != wanted.ExecutionID || a.CreateOperationID != e.CreateOperationID || a.ResourceID != e.ResourceID || a.ReleaseEventID != e.ReleaseEventID || (a.Status != "APPLIED" && a.Status != "ALREADY_APPLIED") {
						blockers = append(blockers, "MD_GPU_ACK_IDENTITY_DIFFERS:"+wanted.ExecutionID)
					}
				}
			}
		}
		if state.ClosedAt == nil || null(runtime, "closed_at") || state.CloseGeneration == 0 || state.CloseReviewReason != "" || state.CloseEvidence == nil {
			blockers = append(blockers, "MD_EXECUTION_NOT_CLOSED:"+wanted.ExecutionID)
		}
	}
	for _, pair := range []struct {
		table    string
		original []row
		fields   []string
	}{{"modeldev_pipeline_dispatches", s.OriginalDispatches, []string{"tenant_id", "execution_id", "operation_id", "spec_hash", "attempt_id", "plan_hash"}}, {"modeldev_pipeline_confirmed_runs", s.OriginalRuns, []string{"tenant_id", "execution_id", "attempt_id", "plan_hash", "run_id"}}, {"modeldev_run_authorities", s.OriginalAuthorities, []string{"tenant_id", "execution_id", "operation_id", "spec_hash", "attempt_id", "plan_hash", "run_id", "namespace_name", "namespace_uid", "workflow_name", "workflow_uid"}}, {"modeldev_input_versions", s.OriginalInputs, []string{"tenant_id", "input_version_id", "request_id", "storage_connection_id", "bucket", "approved_prefix", "object_key", "object_version_id", "sha256"}}} {
		actual := table(v, pair.table).rows
		if len(actual) != len(pair.original) {
			blockers = append(blockers, "MD_ORIGINAL_CHILD_ROWCOUNT_DIFFERS:"+pair.table)
			continue
		}
		key := "execution_id"
		if pair.table == "modeldev_input_versions" {
			key = "input_version_id"
		}
		for _, original := range pair.original {
			if !fieldsMatch(find(actual, key, text(original, key)), original, pair.fields...) {
				blockers = append(blockers, "MD_ORIGINAL_CHILD_IDENTITY_DIFFERS:"+pair.table+":"+text(original, key))
			}
		}
	}
	schema := v.Snapshot.Identity.Schema
	executionIDs := []string{}
	for _, e := range s.Executions {
		executionIDs = append(executionIDs, e.ExecutionID)
	}
	var outside bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+quoted(schema, "modeldev_execution_identities")+" WHERE tenant_id=$1::uuid AND execution_id=ANY($2::uuid[]))", s.TenantID, s.UnknownExecutionIDs).Scan(&outside); err != nil || outside {
		blockers = append(blockers, "UNKNOWN_ACCEPTANCE_HAS_NEW_MD_ROWS_REQUIRES_NEW_MANIFEST")
	}
	// Input versions are metadata only. An execution outside this exact set may
	// still reference them through its frozen canonical bytes; any such hit blocks.
	rows, err := tx.Query(ctx, "SELECT snapshot_canonical FROM "+quoted(schema, "modeldev_executions")+" WHERE NOT (tenant_id=$1::uuid AND execution_id=ANY($2::uuid[]))", s.TenantID, executionIDs)
	if err != nil {
		blockers = append(blockers, "MD_EXTERNAL_INPUT_REFERENCES_UNVERIFIED")
	} else {
		n := 0
		for rows.Next() {
			n++
			var raw []byte
			if rows.Scan(&raw) != nil || n > 512 {
				blockers = append(blockers, "MD_EXTERNAL_INPUT_REFERENCES_UNVERIFIED")
				break
			}
			for _, input := range s.InputIDs {
				if strings.Contains(string(raw), input) {
					blockers = append(blockers, "MD_INPUT_REFERENCED_OUTSIDE_MANIFEST:"+input)
				}
			}
		}
		if rows.Err() != nil {
			blockers = append(blockers, "MD_EXTERNAL_INPUT_REFERENCES_UNVERIFIED")
		}
		rows.Close()
	}
	return blockers
}
func qualifyGov(ctx context.Context, tx pgx.Tx, v databaseView, s OriginalScope, deleted bool) []string {
	blockers := []string{}
	schema := v.Snapshot.Identity.Schema
	expected := map[string]int{"sys_modeldev_acceptances": len(s.AcceptanceIDs), "sys_quota_charges": len(s.ChargeIDs), "sys_quota_operations": len(s.OperationIDs), "sys_quota_release_receipts": len(s.ReceiptIDs)}
	for tableName, count := range expected {
		if deleted {
			count = 0
		}
		if len(table(v, tableName).rows) != count {
			blockers = append(blockers, "GOV_EXACT_ROWCOUNT_DIFFERS:"+tableName)
		}
	}
	operations := table(v, "sys_quota_operations").rows
	charges := table(v, "sys_quota_charges").rows
	receipts := table(v, "sys_quota_release_receipts").rows
	if !deleted {
		for _, original := range s.OriginalAcceptances {
			r := find(table(v, "sys_modeldev_acceptances").rows, "id", text(original, "id"))
			if !fieldsMatch(r, original, "tenant_id", "resource_tenant_id", "operation_id", "execution_id", "execution_spec_hash") {
				blockers = append(blockers, "GOV_ACCEPTANCE_IDENTITY_DIFFERS:"+text(original, "id"))
				continue
			}
			if !null(r, "lease_until") || !null(r, "close_lease_until") || (!null(r, "lease_owner") && text(r, "lease_owner") != "") || (!null(r, "close_lease_owner") && text(r, "close_lease_owner") != "") {
				blockers = append(blockers, "GOV_ACCEPTANCE_LEASE_PRESENT:"+text(r, "id"))
			}
			if text(r, "dispatch_state") != "ACKED" && text(original, "dispatch_state") != "UNKNOWN" {
				blockers = append(blockers, "GOV_ACCEPTANCE_DISPATCH_UNRESOLVED:"+text(r, "id"))
			}
		}
		for _, original := range originalRows(s.OriginalOperations) {
			r := find(operations, "id", text(original, "id"))
			if !fieldsMatch(r, original, "tenant_id", "operation_id", "resource_tenant_id", "resource_id", "create_operation_id", "owner_service", "action", "ack_json") || text(r, "dispatch_state") != "ACKED" || !null(r, "lease_until") || (!null(r, "lease_owner") && text(r, "lease_owner") != "") {
				blockers = append(blockers, "GOV_ORIGINAL_OPERATION_NOT_SETTLED:"+text(original, "id"))
			}
		}
		for _, original := range originalRows(s.OriginalCharges) {
			r := find(charges, "id", text(original, "id"))
			if !fieldsMatch(r, original, "tenant_id", "charge_id", "operation_id", "quota_code", "original_units") || integer(r, "released_units") != integer(r, "original_units") || integer(r, "original_units") <= 0 {
				blockers = append(blockers, "GOV_CHARGE_NOT_FULLY_RELEASED:"+text(original, "id"))
			}
		}
		for _, original := range originalRows(s.OriginalReceipts) {
			r := find(receipts, "receipt_id", text(original, "receipt_id"))
			if !fieldsMatch(r, original, "tenant_id", "receipt_id", "owner_service", "release_event_id", "payload_hash", "payload_json") || bytesSHA([]byte(text(r, "payload_json"))) != text(r, "payload_hash") {
				blockers = append(blockers, "GOV_ORIGINAL_OWNER_RELEASE_RECEIPT_DIFFERS:"+text(original, "receipt_id"))
				continue
			}
			var payload row
			if json.Unmarshal([]byte(text(r, "payload_json")), &payload) != nil || text(payload, "schema") != "resource-release-v1" || text(payload, "owner_service") != "ani-modeldev-service" || text(payload, "release_event_id") != text(r, "release_event_id") {
				blockers = append(blockers, "GOV_OWNER_RELEASE_PAYLOAD_INVALID:"+text(r, "receipt_id"))
				continue
			}
			op := find(operations, "operation_id", text(payload, "create_operation_id"))
			if op == nil || text(op, "resource_id") != text(payload, "resource_id") || text(op, "owner_service") != text(payload, "owner_service") {
				blockers = append(blockers, "GOV_OWNER_RELEASE_ORIGINAL_OPERATION_DIFFERS:"+text(r, "receipt_id"))
			}
		}
	}
	// Locking accounts and charges makes this all-owner invariant stable through
	// deletion. No account value is written, and other owners remain in the sum.
	for _, a := range v.Accounts {
		var units int64
		if err := tx.QueryRow(ctx, "SELECT coalesce(sum(original_units-released_units),0)::bigint FROM "+quoted(schema, "sys_quota_charges")+" WHERE tenant_id=$1 AND quota_code=$2 AND deleted_at IS NULL", s.GovernanceTenantID, text(a, "quota_code")).Scan(&units); err != nil || units != integer(a, "occupied_units") {
			blockers = append(blockers, "GOV_ACCOUNT_SUM_INVARIANT_DIFFERS:"+text(a, "id"))
		}
	}
	if len(v.Accounts) == 0 {
		blockers = append(blockers, "GOV_ACCOUNT_IDENTITIES_UNKNOWN")
	}
	ids := []string{}
	for _, r := range originalRows(s.OriginalOperations) {
		ids = append(ids, text(r, "operation_id"))
	}
	var unknown bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+quoted(schema, "sys_quota_operations")+" WHERE tenant_id=$1 AND create_operation_id=ANY($2::text[]) AND NOT (id=ANY($3::bigint[])))", s.GovernanceTenantID, ids, s.OperationIDs).Scan(&unknown); err != nil || unknown {
		blockers = append(blockers, "GOV_OPERATION_REFERENCED_OUTSIDE_MANIFEST")
	}
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+quoted(schema, "sys_quota_charges")+" WHERE tenant_id=$1 AND operation_id=ANY($2::text[]) AND NOT (id=ANY($3::bigint[])))", s.GovernanceTenantID, ids, s.ChargeIDs).Scan(&unknown); err != nil || unknown {
		blockers = append(blockers, "GOV_CHARGE_REFERENCED_OUTSIDE_MANIFEST")
	}
	// Acceptance and canonical release payload references are semantic rather
	// than foreign keys. They must also stay within the reviewed original set.
	acceptanceOps := append([]string{}, ids...)
	resources := []string{}
	for _, r := range s.OriginalAcceptances {
		acceptanceOps = append(acceptanceOps, text(r, "operation_id"))
		resources = append(resources, text(r, "execution_id"))
	}
	for _, r := range originalRows(s.OriginalOperations) {
		resources = append(resources, text(r, "resource_id"))
	}
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+quoted(schema, "sys_modeldev_acceptances")+" WHERE tenant_id=$1 AND (operation_id=ANY($2::text[]) OR execution_id=ANY($3::text[])) AND NOT (id=ANY($4::bigint[])))", s.GovernanceTenantID, acceptanceOps, resources, s.AcceptanceIDs).Scan(&unknown); err != nil || unknown {
		blockers = append(blockers, "GOV_ACCEPTANCE_REFERENCED_OUTSIDE_MANIFEST")
	}
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM "+quoted(schema, "sys_quota_release_receipts")+" WHERE tenant_id=$1 AND ((payload_json::jsonb->>'create_operation_id')=ANY($2::text[]) OR (payload_json::jsonb->>'resource_id')=ANY($3::text[])) AND NOT (receipt_id=ANY($4::text[])))", s.GovernanceTenantID, acceptanceOps, resources, s.ReceiptIDs).Scan(&unknown); err != nil || unknown {
		blockers = append(blockers, "GOV_RELEASE_RECEIPT_REFERENCED_OUTSIDE_MANIFEST")
	}
	return blockers
}
