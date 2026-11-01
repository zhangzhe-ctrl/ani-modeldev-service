package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func proofValid(p SourceProof) bool {
	if !filepath.IsAbs(p.Path) || p.SHA256 == "" {
		return false
	}
	raw, err := readBounded(p.Path, 8<<20, false)
	return err == nil && bytesSHA(raw) == p.SHA256
}
func (c *CleanupCommand) rootBlockers(mdSnapshot, govSnapshot DatabaseSnapshot) []string {
	md, gov := mdSnapshot.Identity, govSnapshot.Identity
	r := c.Root
	if r == nil || c.RootSHA256 == "" {
		return []string{"ROOT_EVIDENCE_UNKNOWN"}
	}
	out := []string{}
	now := time.Now().UTC()
	if len(r.StopRecords) != 3 || len(r.Dispositions) > 80 {
		return []string{"ROOT_EXTERNAL_PROOF_SET_OUT_OF_BOUND"}
	}
	cache := map[string]string{}
	var total int64
	verifyProof := func(p SourceProof) bool {
		if !filepath.IsAbs(p.Path) || p.SHA256 == "" {
			return false
		}
		if hash, ok := cache[p.Path]; ok {
			return hash == p.SHA256
		}
		info, err := os.Lstat(p.Path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 8<<20 || total+info.Size() > 64<<20 || len(cache) >= 256 {
			return false
		}
		total += info.Size()
		raw, err := readBounded(p.Path, 8<<20, false)
		if err != nil {
			return false
		}
		cache[p.Path] = bytesSHA(raw)
		return cache[p.Path] == p.SHA256
	}
	for _, proof := range []*SourceProof{r.OriginalC99Qualification, r.OriginalC99Build, r.OriginalF216Settlement} {
		if proof != nil && !verifyProof(*proof) {
			out = append(out, "ROOT_ORIGINAL_C99_SOURCE_PROOF_INVALID")
		}
	}
	if r.Schema != "ani.modeldev.legacy-cleanup-root-evidence.v1" || r.ObservedAt.IsZero() || r.ObservedAt.After(now) || !now.Before(r.ExpiresAt) || r.ExpiresAt.After(r.ObservedAt.Add(5*time.Minute)) {
		out = append(out, "ROOT_EVIDENCE_NOT_FRESH")
	}
	if r.SSH != "kubercloud@100.64.128.12" || r.Context != "kubernetes-admin@ani-kcn-full" || r.APIServer != "https://lb.kubesphere.local:6443" || r.ClusterID != "f39bf6e2-f3ed-4191-ada3-caeebf86c220" || r.KubeSystemUID != "3eeb5379-8748-4bf8-8f6d-d7a48ce3bc24" {
		out = append(out, "ROOT_FULL_CLUSTER_IDENTITY_DIFFERS")
	}
	if r.InventorySHA256 != c.Original.InventorySHA256 || r.GovernanceInventorySHA256 != c.Original.GovernanceInventorySHA256 {
		out = append(out, "ROOT_ORIGINAL_SCOPE_HASH_DIFFERS")
	}
	if r.Databases["modeldev"] != md || r.Databases["governance"] != gov {
		out = append(out, "ROOT_DATABASE_IDENTITIES_DIFFER")
	}

	if c.Original.InventorySHA256 == originalInventorySHA {
		if !deleteOwnerValid(r.DeleteOwners["modeldev"], mdSnapshot, "modeldev_v21_migrate") {
			out = append(out, "ROOT_ORIGINAL_MODELDEV_DELETE_OWNER_UNVERIFIED")
		}
		if !deleteOwnerValid(r.DeleteOwners["governance"], govSnapshot, "gov_migrate") {
			out = append(out, "ROOT_ORIGINAL_GOVERNANCE_DELETE_OWNER_UNVERIFIED")
		}
	}
	components := map[string]bool{}
	for _, stop := range r.StopRecords {
		if stop.SourceUID == "" || stop.SourceRevision == "" || stop.State != "STOPPED" || stop.ObservedAt.Before(r.ObservedAt) || stop.ObservedAt.After(now) || !verifyProof(stop.Proof) || components[stop.Component] {
			out = append(out, "ROOT_STOP_SOURCE_PROOF_INVALID")
			continue
		}
		components[stop.Component] = true
	}
	for _, required := range []string{"MODELDEV_WORKERS", "GOVERNANCE_CREATE_PUBLISHER", "LEGACY_REPLAY_AND_PUBLICATION"} {
		if !components[required] {
			out = append(out, "ROOT_STOP_SOURCE_MISSING:"+required)
		}
	}
	p := r.Probe
	if p.Method != "POST" || p.Path != "/api/v1/modeldev/executions" || p.Status != 503 || p.RequestID == "" || p.ObservedAt.Before(r.ObservedAt) || p.ObservedAt.After(now) || !verifyProof(p.Proof) {
		out = append(out, "ROOT_REAL_CREATE_503_PROOF_MISSING")
	}
	// One root-approved disposition per original identity. Files and byte hashes
	// prove exactly which external evidence was reviewed; this tool does not
	// manufacture Kubernetes/S3/KFP observations or infer truth from a boolean.
	expected := map[string]ExecutionIdentity{}
	for _, e := range c.Original.Executions {
		expected[e.ExecutionID] = e
	}
	for _, a := range c.Original.OriginalAcceptances {
		e := ExecutionIdentity{c.Original.TenantID, text(a, "execution_id"), text(a, "operation_id"), text(a, "execution_spec_hash")}
		if old, ok := expected[e.ExecutionID]; ok && old != e {
			out = append(out, "ORIGINAL_CROSS_DATABASE_IDENTITY_DIFFERS:"+e.ExecutionID)
		}
		expected[e.ExecutionID] = e
	}
	for _, o := range originalRows(c.Original.OriginalOperations) {
		id := text(o, "resource_id")
		if _, ok := expected[id]; !ok {
			expected[id] = ExecutionIdentity{c.Original.TenantID, id, text(o, "operation_id"), ""}
		}
	}
	seen := map[string]bool{}
	unknown := map[string]bool{}
	for _, id := range c.Original.UnknownExecutionIDs {
		unknown[id] = true
	}
	mdIDs := map[string]bool{}
	for _, e := range c.Original.Executions {
		mdIDs[e.ExecutionID] = true
	}
	for _, d := range r.Dispositions {
		want, ok := expected[d.ExecutionID]
		if !ok || seen[d.ExecutionID] || d.OperationID != want.OperationID || d.SpecHash != want.SpecHash {
			out = append(out, "ROOT_EXTERNAL_SCOPE_DIFFERS:"+d.ExecutionID)
			continue
		}
		seen[d.ExecutionID] = true
		category := "CLEANED_ORIGINAL_EXTERNAL_RESOURCES"
		if unknown[d.ExecutionID] {
			category = "OWNER_CLOSED_WITHOUT_SIDE_EFFECTS"
		} else if !mdIDs[d.ExecutionID] {
			category = "PRIOR_AUTHENTICATED_OWNER_RELEASE"
		}
		if d.Category != category || len(d.Proofs) == 0 || len(d.Proofs) > 8 {
			out = append(out, "ROOT_EXTERNAL_QUALIFICATION_MISSING:"+d.ExecutionID)
			continue
		}
		kinds := map[string]bool{}
		for _, proof := range d.Proofs {
			if !verifyProof(proof) {
				out = append(out, "ROOT_EXTERNAL_PROOF_HASH_DIFFERS:"+d.ExecutionID)
			}
			kinds[proof.Kind] = true
		}
		required := []string{"KUBERNETES_ORIGINAL_UIDS", "KFP_ORIGINAL_RUN_WORKFLOW", "S3_OWNED_OBJECT_VERSIONS"}
		if unknown[d.ExecutionID] {
			required = []string{"AUTHENTICATED_ORIGINAL_OWNER_CLOSED_NO_SIDE_EFFECTS"}
		}
		for _, kind := range required {
			if !kinds[kind] {
				out = append(out, "ROOT_EXTERNAL_PROOF_KIND_MISSING:"+d.ExecutionID+":"+kind)
			}
		}
	}
	for id := range expected {
		if !seen[id] {
			out = append(out, "ROOT_EXTERNAL_QUALIFICATION_UNKNOWN:"+id)
		}
	}
	sort.Strings(out)
	return out
}
func validatePriorReceipt(prior *DeletionReceipt, s OriginalScope) error {
	if prior == nil || !prior.Applied || prior.Phase != "governance" || prior.Schema != "ani.modeldev.legacy-db-cleanup.receipt.v1" || prior.CompletedAt.IsZero() || prior.CompletedAt.After(time.Now().UTC()) || prior.Plan.InventorySHA256 != s.InventorySHA256 || prior.Plan.GovernanceInventorySHA256 != s.GovernanceInventorySHA256 || prior.Plan.GovernanceSyncInventorySHA256 != s.SyncInventorySHA256 || len(prior.Plan.Blockers) != 0 {
		return fmt.Errorf("successful original Governance deletion receipt required")
	}
	plan := prior.Plan
	if plan.Schema != cleanupPlanSchema {
		return fmt.Errorf("current fixed54 deletion plan required")
	}
	sha := plan.ManifestSHA256
	plan.ManifestSHA256 = ""
	if sha == "" || digest(plan) != sha {
		return fmt.Errorf("original Governance plan digest differs")
	}
	expected := map[string]int64{"sys_modeldev_acceptances": int64(len(s.AcceptanceIDs)), "sys_quota_operations": int64(len(s.OperationIDs)), "sys_quota_charges": int64(len(s.ChargeIDs)), "sys_gpu_usage_sync": int64(len(s.SyncIDs)), "sys_quota_release_receipts": int64(len(s.ReceiptIDs))}
	if digest(prior.Deleted) != digest(expected) || len(prior.RetainedCharges) != len(s.ChargeIDs) || len(prior.RetainedReleaseReceipts) != len(s.ReceiptIDs) || digest(prior.AccountsBefore) != digest(prior.AccountsAfter) {
		return fmt.Errorf("original Governance deletion counts or retained ledger evidence differ")
	}
	for _, o := range originalRows(s.OriginalCharges) {
		r := find(prior.RetainedCharges, "id", text(o, "id"))
		if !fieldsMatch(r, o, "tenant_id", "charge_id", "operation_id", "quota_code", "original_units") || integer(r, "released_units") != integer(r, "original_units") {
			return fmt.Errorf("original Governance retained charge not fully released")
		}
	}
	for _, o := range originalRows(s.OriginalReceipts) {
		r := find(prior.RetainedReleaseReceipts, "receipt_id", text(o, "receipt_id"))
		if !fieldsMatch(r, o, "tenant_id", "receipt_id", "owner_service", "release_event_id", "payload_hash", "payload_json") || bytesSHA([]byte(text(r, "payload_json"))) != text(r, "payload_hash") {
			return fmt.Errorf("original Governance retained owner receipt differs")
		}
	}
	return nil
}
