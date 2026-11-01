package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

const originalInventorySHA = "aa5420bd1c2ad02f373509d625a99080b4c6721050de1bcdb3496ad1d15a44d2"
const originalGovInventorySHA = "66a3e9b4eb4edc931e8bc452d1c48e671d97e3412c820c992b6f28044cf54406"
const legacyTenant = "64c5c35a-f554-432c-b4bc-7b4f36f08420"

type row map[string]json.RawMessage

func text(r row, key string) string {
	var s string
	if json.Unmarshal(r[key], &s) == nil {
		return s
	}
	return string(r[key])
}
func integer(r row, key string) int64 { var n int64; _ = json.Unmarshal(r[key], &n); return n }
func null(r row, key string) bool     { return len(r[key]) == 0 || string(r[key]) == "null" }
func boolean(r row, key string) bool  { return string(r[key]) == "true" }
func readBounded(path string, limit int64, private bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit || private && info.Mode().Perm() != 0600 {
		return nil, fmt.Errorf("bounded regular input required; private DSN file must be mode0600")
	}
	return os.ReadFile(path)
}
func loadOriginal(mdPath, govPath string) (OriginalScope, error) {
	s := OriginalScope{TenantID: legacyTenant, GovernanceTenantID: 2, InventorySHA256: originalInventorySHA, GovernanceInventorySHA256: originalGovInventorySHA, SyncIDs: append([]int64{}, originalSyncIDs...)}
	md, err := readBounded(mdPath, 16<<20, false)
	if err != nil {
		return s, err
	}
	gov, err := readBounded(govPath, 4<<20, false)
	if err != nil {
		return s, err
	}
	if bytesSHA(md) != originalInventorySHA || bytesSHA(gov) != originalGovInventorySHA {
		return s, fmt.Errorf("only the fixed original task09 inventory byte hashes are accepted")
	}
	var m struct {
		Executions, Inputs, Dispatches, Runs, Authorities []row
		SchemaColumns                                     []column     `json:"schema_columns"`
		ForeignKeys                                       []foreignKey `json:"foreign_keys"`
		GovColumns                                        []column     `json:"gov_schema_columns"`
		GovKeys                                           []foreignKey `json:"gov_foreign_keys"`
	}
	var g struct{ Acceptances, Operations, Charges, Receipts []row }
	if json.Unmarshal(md, &m) != nil || json.Unmarshal(gov, &g) != nil {
		return s, fmt.Errorf("invalid fixed inventory")
	}
	if len(m.Executions) != 50 || len(m.Inputs) != 10 || len(g.Acceptances) != 54 || len(g.Operations) != 23 || len(g.Charges) != 23 || len(g.Receipts) != 23 {
		return s, fmt.Errorf("fixed original inventory row counts differ")
	}
	s.OriginalMDColumns = m.SchemaColumns
	s.OriginalGovColumns = m.GovColumns
	s.OriginalMDForeignKeys = m.ForeignKeys
	s.OriginalGovForeignKeys = m.GovKeys
	for _, r := range m.Executions {
		if text(r, "tenant_id") != legacyTenant {
			return s, fmt.Errorf("original MD tenant differs")
		}
		s.Executions = append(s.Executions, ExecutionIdentity{legacyTenant, text(r, "execution_id"), text(r, "operation_id"), text(r, "spec_hash")})
	}
	for _, r := range m.Inputs {
		if text(r, "tenant_id") != legacyTenant {
			return s, fmt.Errorf("original input tenant differs")
		}
		s.InputIDs = append(s.InputIDs, text(r, "input_version_id"))
	}
	s.OriginalAcceptances = g.Acceptances
	s.OriginalDispatches = m.Dispatches
	s.OriginalRuns = m.Runs
	s.OriginalAuthorities = m.Authorities
	s.OriginalInputs = m.Inputs
	for _, r := range g.Acceptances {
		if integer(r, "tenant_id") != 2 || text(r, "resource_tenant_id") != legacyTenant {
			return s, fmt.Errorf("original Governance mapping differs")
		}
		s.AcceptanceIDs = append(s.AcceptanceIDs, integer(r, "id"))
		if text(r, "dispatch_state") == "UNKNOWN" {
			s.UnknownExecutionIDs = append(s.UnknownExecutionIDs, text(r, "execution_id"))
		}
	}
	if len(s.UnknownExecutionIDs) != 4 {
		return s, fmt.Errorf("fixed four unknown acceptances differ")
	}
	for _, id := range s.UnknownExecutionIDs {
		w, ok := originalFour[id]
		original := find(s.OriginalAcceptances, "execution_id", id)
		if !ok || original == nil || integer(original, "id") != w.AcceptanceID || text(original, "operation_id") != w.OperationID || text(original, "execution_spec_hash") != w.SpecHash {
			return s, fmt.Errorf("only the four original reliably retired commands may extend the fixed fifty")
		}
		s.Executions = append(s.Executions, w.ExecutionIdentity)
	}
	if len(s.Executions) != 54 {
		return s, fmt.Errorf("fixed original fifty plus four aggregate scope differs")
	}
	for _, r := range g.Operations {
		if integer(r, "tenant_id") != 2 || text(r, "owner_service") != "ani-modeldev-service" || text(r, "action") != "modeldev.execution.create" {
			return s, fmt.Errorf("original Governance operation owner/action differs")
		}
		s.OperationIDs = append(s.OperationIDs, integer(r, "id"))
		raw, _ := json.Marshal(r)
		s.OriginalOperations = append(s.OriginalOperations, raw)
	}
	for _, r := range g.Charges {
		s.ChargeIDs = append(s.ChargeIDs, integer(r, "id"))
		raw, _ := json.Marshal(r)
		s.OriginalCharges = append(s.OriginalCharges, raw)
	}
	for _, r := range g.Receipts {
		s.ReceiptIDs = append(s.ReceiptIDs, text(r, "receipt_id"))
		raw, _ := json.Marshal(r)
		s.OriginalReceipts = append(s.OriginalReceipts, raw)
	}
	sort.Slice(s.Executions, func(i, j int) bool { return s.Executions[i].ExecutionID < s.Executions[j].ExecutionID })
	sort.Strings(s.InputIDs)
	sort.Strings(s.ReceiptIDs)
	return s, nil
}
