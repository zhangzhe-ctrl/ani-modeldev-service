package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

var originalSyncIDs = []int64{3, 5, 7, 9, 10, 13, 15, 16, 20, 37, 39, 41, 49, 50, 51, 53, 56, 58, 59, 61, 63, 66, 67}

type OriginalSyncIdentity struct {
	ID, TenantID                                                                      int64
	OperationID, ResourceTenantID, OwnerService, ResourceID, PayloadSHA256, RowSHA256 string
}
type OriginalSyncManifest struct {
	Schema, GovernanceInventorySHA256 string
	Rows                              []OriginalSyncIdentity
}

func loadOriginalSync(path, expected string, s *OriginalScope) error {
	raw, err := readBounded(path, 1<<20, false)
	if err != nil || expected == "" || bytesSHA(raw) != expected {
		return fmt.Errorf("root-reviewed original usage-sync manifest byte hash required")
	}
	var manifest OriginalSyncManifest
	if strictJSON(raw, &manifest) != nil || manifest.Schema != "ani.modeldev.original23-usage-sync-manifest.v1" || manifest.GovernanceInventorySHA256 != originalGovInventorySHA || len(manifest.Rows) != 23 {
		return fmt.Errorf("fixed original23 usage-sync manifest required")
	}
	ids := []int64{}
	operations := map[string]bool{}
	for _, r := range manifest.Rows {
		original := find(originalRows(s.OriginalOperations), "operation_id", r.OperationID)
		if original == nil || r.TenantID != s.GovernanceTenantID || r.ResourceTenantID != s.TenantID || r.OwnerService != "ani-modeldev-service" || text(original, "resource_id") != r.ResourceID || operations[r.OperationID] || !validSHA(r.PayloadSHA256) || !validSHA(r.RowSHA256) {
			return fmt.Errorf("original usage-sync identity differs")
		}
		operations[r.OperationID] = true
		ids = append(ids, r.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if digest(ids) != digest(originalSyncIDs) {
		return fmt.Errorf("original usage-sync PK set differs")
	}
	s.OriginalSync = manifest.Rows
	s.SyncInventorySHA256 = expected
	return nil
}
func validSHA(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'f' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func originalSyncKeyAllowed(s OriginalScope, key string) bool {
	for _, id := range s.SyncIDs {
		if key == "id="+strconv.FormatInt(id, 10) {
			return true
		}
	}
	return false
}
func qualifyOriginalSync(v databaseView, s OriginalScope, deleted bool) []string {
	blockers := []string{}
	if len(s.SyncIDs) == 0 {
		return blockers
	} // Isolated fixtures have no original sync rows.
	if s.SyncInventorySHA256 == "" || len(s.OriginalSync) != len(s.SyncIDs) {
		return []string{"GOV_ORIGINAL_SYNC_MANIFEST_UNKNOWN"}
	}
	rows := table(v, "sys_gpu_usage_sync").rows
	if deleted {
		if len(rows) != 0 {
			return []string{"GOV_ORIGINAL_SYNC_NOT_DELETED"}
		}
		return blockers
	}
	summaries := TableSummary{}
	for _, t := range v.Snapshot.Tables {
		if t.Table == "sys_gpu_usage_sync" {
			summaries = t
		}
	}
	for _, original := range s.OriginalSync {
		id := strconv.FormatInt(original.ID, 10)
		r := find(rows, "id", id)
		rowSHA := ""
		for _, summary := range summaries.Rows {
			if summary.Key == "id="+id {
				rowSHA = summary.SHA256
			}
		}
		if r == nil || integer(r, "tenant_id") != original.TenantID || text(r, "operation_id") != original.OperationID || text(r, "resource_tenant_id") != original.ResourceTenantID || text(r, "owner_service") != original.OwnerService || text(r, "resource_id") != original.ResourceID || text(r, "payload_hash") != original.PayloadSHA256 || rowSHA != original.RowSHA256 {
			blockers = append(blockers, "GOV_ORIGINAL_SYNC_IDENTITY_OR_BYTES_DIFFERS:"+id)
			continue
		}
		// The original ACK transition retains lease_owner but clears lease_until;
		// a stale worker label is no authority to replay an ACKed revision.
		if text(r, "state") != "ENDED" || integer(r, "revision") != 2 || integer(r, "acked_revision") != 2 || boolean(r, "retry_blocked") || !null(r, "lease_until") || !null(r, "next_attempt_at") || !null(r, "last_error_code") || !null(r, "deleted_at") {
			blockers = append(blockers, "GOV_ORIGINAL_SYNC_NOT_ACKED_ENDED:"+id)
		}
		var payload row
		if json.Unmarshal([]byte(text(r, "payload_json")), &payload) != nil || len(payload) == 0 {
			blockers = append(blockers, "GOV_ORIGINAL_SYNC_PAYLOAD_INVALID:"+id)
		}
	}
	return blockers
}
