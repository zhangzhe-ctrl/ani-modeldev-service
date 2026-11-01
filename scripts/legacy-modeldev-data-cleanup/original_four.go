package main

import (
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

const cleanupPlanSchema = "ani.modeldev.legacy-db-cleanup.plan.v3"

type originalFourIdentity struct {
	AcceptanceID int64
	ExecutionIdentity
	IntentSHA256, Actor, AcceptedAt string
}

// The four original UNKNOWN commands were fenced, reliably redelivered with
// their original bytes/actor/time, and closed by the original NoDispatch path.
// This finite maintenance revision includes those four newly durable aggregates;
// it is not permission to adopt any other new execution or alter a runtime fact.
var originalFour = map[string]originalFourIdentity{
	"27cf73a3-4107-4c07-86f7-862ce9fb413d": {55, ExecutionIdentity{legacyTenant, "27cf73a3-4107-4c07-86f7-862ce9fb413d", "13bd316b-0b85-4c8e-a005-698eb9ffdd5e", "cb9831ca80e7df95cf1ca3746db4ff53b623b51671efcf34230275235e0c1690"}, "451f319fadcf72f4be5641971f8c930d7fedab27c4af70f529e801e8844cf0ce", "governance:user:3", "2026-10-29T23:10:32.619256+08:00"},
	"c9b470e0-d9cb-4989-a7f2-d1fbfbe72ab9": {57, ExecutionIdentity{legacyTenant, "c9b470e0-d9cb-4989-a7f2-d1fbfbe72ab9", "cd2ee5e3-e4fb-4515-99f8-33b3da892a8f", "9883ac49d909b2809e78002db7d47d564caa88f84bd29ba14d6248a34e8bb096"}, "fda56fe14be954c582addb294805dc0c0afe639c085defdd45aa4428dcc621f0", "governance:user:3", "2026-10-30T00:06:03.806936+08:00"},
	"91e351e7-102e-4807-b0d2-9236c1426da9": {59, ExecutionIdentity{legacyTenant, "91e351e7-102e-4807-b0d2-9236c1426da9", "ff5fc1f4-d9a8-4c6a-8c47-5b1d4ef06f05", "e75a332cef7521f278df8fc78df0b3f869d8499f96d188fd3b8a915fe02ec829"}, "732ea6ba966201699f3ad707f50dbb9151bcf568c23d856beeb6902f3166eb6f", "governance:user:3", "2026-10-30T08:00:22.738236+08:00"},
	"f148135c-640f-47ed-b9b1-4b039d4709d1": {60, ExecutionIdentity{legacyTenant, "f148135c-640f-47ed-b9b1-4b039d4709d1", "25b88cf7-bccb-4fac-8c9a-6b3ff91477ee", "85fa8793e5ff44c8c611316129f344a36124e027f03cc71bf797d89fa1d40a45"}, "d0f78b8d1ef87c029313b06b0c0d193bddb3b968198744e05d37ed79d63e47c7", "governance:user:3", "2026-10-30T08:08:04.77998+08:00"},
}

func qualifyOriginalFourAdmission(r row, execution biz.Execution) bool {
	w, ok := originalFour[execution.ExecutionID]
	if !ok || execution.TenantID != w.TenantID || execution.OperationID != w.OperationID || execution.SpecHash != w.SpecHash || execution.IntentHash != w.IntentSHA256 || execution.Actor != w.Actor || execution.Snapshot.ComputeOrigin.DeviceMode != "cpu" {
		return false
	}
	at, err := time.Parse(time.RFC3339Nano, w.AcceptedAt)
	if err != nil || !execution.AcceptedAt.Equal(at) {
		return false
	}
	intent, err := hex.DecodeString(strings.TrimPrefix(text(r, "intent_canonical"), "\\x"))
	if err != nil || bytesSHA(intent) != w.IntentSHA256 {
		return false
	}
	snapshot, err := hex.DecodeString(strings.TrimPrefix(text(r, "snapshot_canonical"), "\\x"))
	return err == nil && bytesSHA(snapshot) == w.SpecHash
}

func qualifyOriginalFourRuntime(w ExecutionIdentity, state biz.ExecutionRuntime, v databaseView) bool {
	expected, ok := originalFour[w.ExecutionID]
	if !ok || expected.ExecutionIdentity != w || state.ClosedAt == nil || state.CloseGeneration != 1 || state.CloseReason != string(biz.CloseReasonUserStop) || state.CloseEvidence == nil || !state.CloseEvidence.NoDispatch || state.CloseReviewReason != "" || state.LegacyRetirement != nil || state.CloseAuthority != nil || state.RuntimeTemplate != nil || state.Workspace != nil || state.Training != nil || state.TrainingHandle != nil || state.Observation != nil || state.Publication != nil || state.StepExitCleanup != nil || state.ComputeCleanup != nil || state.GPURelease != nil {
		return false
	}
	for _, name := range []string{"modeldev_pipeline_dispatches", "modeldev_pipeline_confirmed_runs", "modeldev_run_authorities"} {
		for _, r := range table(v, name).rows {
			if text(r, "tenant_id") == w.TenantID && text(r, "execution_id") == w.ExecutionID {
				return false
			}
		}
	}
	return true
}

func qualifyOriginalFourGov(r row) bool {
	w, ok := originalFour[text(r, "execution_id")]
	if !ok || integer(r, "id") != w.AcceptanceID || integer(r, "tenant_id") != 2 || text(r, "resource_tenant_id") != legacyTenant || text(r, "dispatch_state") != "ACKED" || text(r, "close_dispatch_state") != "ACKED" || boolean(r, "retry_blocked") || boolean(r, "close_retry_blocked") || integer(r, "stop_intent_generation") != 1 {
		return false
	}
	admissionRow := row{}
	for k, v := range r {
		admissionRow[k] = v
	}
	admissionRow["tenant_id"] = r["resource_tenant_id"]
	admissionRow["spec_hash"] = r["execution_spec_hash"]
	a, e := admitted(admissionRow)
	if e != nil || !qualifyOriginalFourAdmission(admissionRow, a) {
		return false
	}
	decode := func(key string, out any) bool {
		b, e := hex.DecodeString(strings.TrimPrefix(text(r, key), "\\x"))
		return e == nil && strictJSON(b, out) == nil
	}
	var close struct {
		OperationID       string `json:"operation_id"`
		ExecutionID       string `json:"execution_id"`
		ExecutionSpecHash string `json:"execution_spec_hash"`
		CloseGeneration   uint64 `json:"close_generation"`
		CloseState        string `json:"close_state"`
		Replayed          bool   `json:"replayed"`
	}
	if !decode("close_receipt_canonical", &close) || close.OperationID != w.OperationID || close.ExecutionID != w.ExecutionID || close.ExecutionSpecHash != w.SpecHash || close.CloseGeneration != 1 || (close.CloseState != "CLOSING" && close.CloseState != "CLOSED") {
		return false
	}
	// The persisted original Governance v1 receipt is checked only here, for
	// these four immutable commands. It is never a runtime compatibility reader.
	var owner struct {
		Schema   string `json:"schema"`
		Identity struct {
			OperationID       string `json:"operation_id"`
			ExecutionID       string `json:"execution_id"`
			ExecutionSpecHash string `json:"execution_spec_hash"`
		} `json:"identity"`
		States struct {
			ComputeState  string `json:"compute_state"`
			DeliveryState string `json:"delivery_state"`
			ResourceState string `json:"resource_state"`
			CloseState    string `json:"close_state"`
		} `json:"states"`
		Revision string `json:"revision"`
		Replayed bool   `json:"replayed"`
	}
	if !decode("owner_receipt_canonical", &owner) {
		return false
	}
	revision, e := strconv.ParseUint(owner.Revision, 10, 64)
	return e == nil && revision > 0 && owner.Schema == "ani.governance.modeldev-owner-receipt.v1" && owner.Identity.OperationID == w.OperationID && owner.Identity.ExecutionID == w.ExecutionID && owner.Identity.ExecutionSpecHash == w.SpecHash && owner.States.ResourceState == "NOT_APPLICABLE" && modeldevv1.ComputeState_value["COMPUTE_STATE_"+owner.States.ComputeState] != 0 && modeldevv1.DeliveryState_value["DELIVERY_STATE_"+owner.States.DeliveryState] != 0 && (owner.States.CloseState == "CLOSING" || owner.States.CloseState == "CLOSED")
}
