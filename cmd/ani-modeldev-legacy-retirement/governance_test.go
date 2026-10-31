package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func originalGovProof(t *testing.T) govReleaseProof {
	t.Helper()
	raw := `{"ObservedAt":"2026-10-31T18:00:00Z","DatabaseName":"ani_governance","DatabaseUser":"postgres","TransactionReadOnly":"on","ActiveCreateLeases":0,"Receipt":{"ReceiptID":"23600748-d80d-5ceb-be35-664a995bb7db","OwnerService":"ani-modeldev-service","ReleaseEventID":"54c1fb78-da48-4586-990e-333ed8090aef","PayloadSHA256":"3e63b8ddc427e1228169804c6a6d28c34899318224257119b0c788b522ca7cad"},"Charges":[{"ChargeID":"1045b409-962a-4211-aee7-30319bffe459","OperationID":"1bc185ad-0977-4b3f-bb1c-404bc37e94fa","QuotaCode":"gpu.shared_memory_mib","OriginalUnits":24570,"ReleasedUnits":24570}],"Accounts":[{"AccountID":2,"QuotaCode":"gpu.physical.count","OccupiedUnits":0,"AllOwnerChargeUnits":0},{"AccountID":6,"QuotaCode":"gpu.shared_memory_mib","OccupiedUnits":0,"AllOwnerChargeUnits":0}]}`
	var p govReleaseProof
	if json.Unmarshal([]byte(raw), &p) != nil {
		t.Fatal("external original receipt metadata fixture")
	}
	p.Receipt.PayloadJSON = `{"schema":"resource-release-v1","owner_service":"ani-modeldev-service","release_event_id":"54c1fb78-da48-4586-990e-333ed8090aef","create_operation_id":"1bc185ad-0977-4b3f-bb1c-404bc37e94fa","resource_id":"eca4d47b-5911-4f0f-8b45-89c1be775925","reason":"RESOURCE_RELEASED"}`
	if !p.valid() {
		t.Fatal("fixed original canonical Governance receipt rejected")
	}
	return p
}
func TestLegacyRetirementOriginalGovReceiptRequiresActualZeroDeltaACK(t *testing.T) {
	before := originalGovProof(t)
	after := originalGovProof(t)
	after.ObservedAt = time.Now().UTC()
	event := biz.LegacyRetirementOriginalGPUEvent()
	ack := biz.GPUReleaseACK{ReleaseEventID: event.ReleaseEventID, CreateOperationID: event.CreateOperationID, ResourceID: event.ResourceID, Status: "ALREADY_APPLIED"}
	receipt, err := priorGPUReceipt(before, after, ack)
	if err != nil || receipt.Event != event || receipt.ACK != ack {
		t.Fatal("original event was rewritten", err)
	}
	for name, change := range map[string]func(*govReleaseProof, *biz.GPUReleaseACK){"new-event": func(_ *govReleaseProof, a *biz.GPUReleaseACK) { a.ReleaseEventID = "new-event" }, "applied": func(_ *govReleaseProof, a *biz.GPUReleaseACK) { a.Status = "APPLIED" }, "remaining-units": func(p *govReleaseProof, _ *biz.GPUReleaseACK) { p.Charges[0].ReleasedUnits-- }, "account-imbalance": func(p *govReleaseProof, _ *biz.GPUReleaseACK) { p.Accounts[0].OccupiedUnits++ }, "changed-occupied-delta": func(p *govReleaseProof, _ *biz.GPUReleaseACK) {
		p.Accounts[0].OccupiedUnits++
		p.Accounts[0].AllOwnerChargeUnits++
	}, "new-create-lease": func(p *govReleaseProof, _ *biz.GPUReleaseACK) { p.ActiveCreateLeases = 1 }, "different-payload": func(p *govReleaseProof, _ *biz.GPUReleaseACK) { p.Receipt.PayloadJSON += " " }, "not-read-only": func(p *govReleaseProof, _ *biz.GPUReleaseACK) { p.TransactionReadOnly = "off" }} {
		t.Run(name, func(t *testing.T) {
			current := originalGovProof(t)
			candidate := ack
			change(&current, &candidate)
			if _, err := priorGPUReceipt(before, current, candidate); err == nil {
				t.Fatal("financial original-receipt guard widened")
			}
		})
	}
}
