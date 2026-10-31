package main

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

type govReleaseProof struct {
	ObservedAt                                      time.Time
	DatabaseName, DatabaseUser, TransactionReadOnly string
	ActiveCreateLeases                              int64
	Receipt                                         struct{ ReceiptID, OwnerService, ReleaseEventID, PayloadSHA256, PayloadJSON string }
	Charges                                         []struct {
		ChargeID, OperationID, QuotaCode string
		OriginalUnits, ReleasedUnits     int64
	}
	Accounts []biz.LegacyRetirementAccountInvariant
}

// This is the pre-existing bounded maintenance DB read path, with a forced
// read-only transaction and exact receipt/create identity. No credential is
// copied out of the original PostgreSQL container's environment.
func readGovReleaseProof(ctx context.Context) (govReleaseProof, error) {
	var proof govReleaseProof
	sql := `BEGIN READ ONLY;
SELECT json_build_object(
 'ObservedAt',clock_timestamp(),'DatabaseName',current_database(),'DatabaseUser',current_user,'TransactionReadOnly',current_setting('transaction_read_only'),
 'ActiveCreateLeases',(SELECT count(*) FROM sys_modeldev_acceptances WHERE action='modeldev.execution.create' AND lease_until>clock_timestamp()),
 'Receipt',(SELECT row_to_json(r) FROM (SELECT receipt_id AS "ReceiptID",owner_service AS "OwnerService",release_event_id AS "ReleaseEventID",payload_hash AS "PayloadSHA256",payload_json AS "PayloadJSON" FROM sys_quota_release_receipts WHERE tenant_id=2 AND receipt_id='23600748-d80d-5ceb-be35-664a995bb7db' AND release_event_id='54c1fb78-da48-4586-990e-333ed8090aef')r),
 'Charges',(SELECT coalesce(json_agg(c),'[]') FROM (SELECT charge_id AS "ChargeID",operation_id AS "OperationID",quota_code AS "QuotaCode",original_units AS "OriginalUnits",released_units AS "ReleasedUnits" FROM sys_quota_charges WHERE tenant_id=2 AND operation_id='1bc185ad-0977-4b3f-bb1c-404bc37e94fa' ORDER BY id)c),
 'Accounts',(SELECT coalesce(json_agg(a),'[]') FROM (SELECT id AS "AccountID",quota_code AS "QuotaCode",occupied_units AS "OccupiedUnits",(SELECT coalesce(sum(c.original_units-c.released_units),0) FROM sys_quota_charges c WHERE c.tenant_id=2 AND c.quota_code=q.quota_code AND c.deleted_at IS NULL) AS "AllOwnerChargeUnits" FROM sys_quota_accounts q WHERE tenant_id=2 AND id IN(2,6) AND deleted_at IS NULL ORDER BY id)a));
COMMIT;`
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(bounded, "kubectl", "--request-timeout=20s", "-n", "ani-platform", "exec", "-i", "postgresql-0", "--", "sh", "-c", `PGPASSWORD="${POSTGRES_PASSWORD:-${POSTGRES_POSTGRES_PASSWORD:-}}" PGOPTIONS="-c default_transaction_read_only=on -c statement_timeout=10000" exec psql -U postgres -d ani_governance -qAt -v ON_ERROR_STOP=1`)
	command.Stdin = strings.NewReader(sql)
	raw, err := command.Output()
	if err != nil || len(raw) > 16384 || json.Unmarshal(raw, &proof) != nil || !proof.valid() {
		return proof, errors.New("MAINTENANCE_ORIGINAL_GOV_RECEIPT_OR_ACCOUNT_NOT_PROVED")
	}
	return proof, nil
}
func (p govReleaseProof) valid() bool {
	if p.ObservedAt.IsZero() || p.ObservedAt.After(time.Now().UTC()) || p.DatabaseName != "ani_governance" || p.DatabaseUser != "postgres" || p.TransactionReadOnly != "on" || p.ActiveCreateLeases != 0 || p.Receipt.ReceiptID != "23600748-d80d-5ceb-be35-664a995bb7db" || p.Receipt.OwnerService != "ani-modeldev-service" || p.Receipt.ReleaseEventID != "54c1fb78-da48-4586-990e-333ed8090aef" || p.Receipt.PayloadSHA256 != "3e63b8ddc427e1228169804c6a6d28c34899318224257119b0c788b522ca7cad" || digest([]byte(p.Receipt.PayloadJSON)) != p.Receipt.PayloadSHA256 || len(p.Charges) != 1 || len(p.Accounts) != 2 {
		return false
	}
	charge := p.Charges[0]
	if charge.ChargeID != "1045b409-962a-4211-aee7-30319bffe459" || charge.OperationID != "1bc185ad-0977-4b3f-bb1c-404bc37e94fa" || charge.QuotaCode != "gpu.shared_memory_mib" || charge.OriginalUnits != 24570 || charge.ReleasedUnits != charge.OriginalUnits {
		return false
	}
	for i, a := range p.Accounts {
		if a.OccupiedUnits < 0 || a.OccupiedUnits != a.AllOwnerChargeUnits {
			return false
		}
		if i == 0 && (a.AccountID != 2 || a.QuotaCode != "gpu.physical.count") || i == 1 && (a.AccountID != 6 || a.QuotaCode != "gpu.shared_memory_mib") {
			return false
		}
	}
	return true
}
func (p govReleaseProof) digest() string {
	p.ObservedAt = time.Time{}
	raw, _ := json.Marshal(p)
	return digest(raw)
}
func priorGPUReceipt(before, after govReleaseProof, ack biz.GPUReleaseACK) (biz.LegacyRetirementPriorGPURelease, error) {
	var receipt biz.LegacyRetirementPriorGPURelease
	if !before.valid() || !after.valid() || before.digest() != after.digest() || !reflect.DeepEqual(before.Accounts, after.Accounts) || ack.Status != "ALREADY_APPLIED" {
		return receipt, errors.New("MAINTENANCE_ORIGINAL_GOV_ZERO_DELTA_NOT_PROVED")
	}
	event := biz.LegacyRetirementOriginalGPUEvent()
	if ack.ReleaseEventID != event.ReleaseEventID || ack.CreateOperationID != event.CreateOperationID || ack.ResourceID != event.ResourceID {
		return receipt, errors.New("MAINTENANCE_ORIGINAL_GOV_ACK_IDENTITY_CHANGED")
	}
	charge := after.Charges[0]
	receipt = biz.LegacyRetirementPriorGPURelease{ReceiptID: after.Receipt.ReceiptID, PayloadSHA256: after.Receipt.PayloadSHA256, Event: event, ACK: ack, ChargeID: charge.ChargeID, QuotaCode: charge.QuotaCode, OriginalUnits: charge.OriginalUnits, ReleasedUnits: charge.ReleasedUnits, BeforeAccounts: before.Accounts, AfterAccounts: after.Accounts, BeforeEvidenceSHA256: before.digest(), AfterEvidenceSHA256: after.digest(), ObservedAt: after.ObservedAt}
	return receipt, nil
}
