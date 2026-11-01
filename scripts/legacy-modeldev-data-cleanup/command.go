// Command legacy-modeldev-data-cleanup removes only the fixed reviewed legacy
// aggregates. It has no runtime service, migration or external deletion path.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ExecutionIdentity struct{ TenantID, ExecutionID, OperationID, SpecHash string }
type OriginalScope struct {
	TenantID                                                              string
	GovernanceTenantID                                                    int64
	Executions                                                            []ExecutionIdentity
	InputIDs, UnknownExecutionIDs                                         []string
	AcceptanceIDs, OperationIDs, ChargeIDs                                []int64
	ReceiptIDs                                                            []string
	OriginalOperations, OriginalCharges, OriginalReceipts                 []json.RawMessage
	OriginalAcceptances                                                   []row
	OriginalDispatches, OriginalRuns, OriginalAuthorities, OriginalInputs []row
	OriginalMDColumns, OriginalGovColumns                                 []column
	OriginalMDForeignKeys, OriginalGovForeignKeys                         []foreignKey
	InventorySHA256, GovernanceInventorySHA256                            string
}
type SourceProof struct{ Kind, Path, SHA256 string }
type ExternalDisposition struct {
	ExecutionID, OperationID, SpecHash, Category string
	Proofs                                       []SourceProof
}
type RootEvidence struct {
	Schema                                            string
	ObservedAt, ExpiresAt                             time.Time
	SSH, Context, APIServer, ClusterID, KubeSystemUID string
	InventorySHA256, GovernanceInventorySHA256        string
	Dispositions                                      []ExternalDisposition
	StopRecords                                       []StoppedSource
	Probe                                             HTTPProbe
	Databases                                         map[string]DatabaseIdentity
	OriginalC99Qualification, OriginalC99Build        *SourceProof
}
type StoppedSource struct {
	Component, SourceUID, SourceRevision, State string
	ObservedAt                                  time.Time
	Proof                                       SourceProof
}
type HTTPProbe struct {
	Method, Path, RequestID string
	Status                  int
	ObservedAt              time.Time
	Proof                   SourceProof
}
type DatabaseIdentity struct {
	Database, Schema, Role, ServerAddress string
	Port                                  int
	DatabaseOID                           uint32
}
type CleanupCommand struct {
	ModelDev, Governance                *pgxpool.Pool
	Original                            OriginalScope
	Root                                *RootEvidence
	RootSHA256, GovernanceReceiptSHA256 string
}
type RowSummary struct{ Key, SHA256 string }
type TableSummary struct {
	Table string
	Rows  []RowSummary
}
type DatabaseSnapshot struct {
	Identity       DatabaseIdentity
	CatalogSHA256  string
	Tables         []TableSummary
	AccountsSHA256 string
	Catalog        *actualCatalog
	Refusal        string
}
type CleanupPlan struct {
	Schema, Phase, InventorySHA256, GovernanceInventorySHA256, RootEvidenceSHA256, GovernanceDeletionSHA256 string
	ModelDev, Governance                                                                                    DatabaseSnapshot
	Blockers                                                                                                []string
	ManifestSHA256                                                                                          string
}
type DeletionReceipt struct {
	Schema                                                                  string
	Applied                                                                 bool
	Phase                                                                   string
	Plan                                                                    CleanupPlan
	Deleted                                                                 map[string]int64
	CompletedAt                                                             time.Time
	RetainedCharges, RetainedReleaseReceipts, AccountsBefore, AccountsAfter []row
}

func bytesSHA(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func rollbackCleanup(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func beginCleanup(ctx context.Context, pool *pgxpool.Pool, apply bool) (pgx.Tx, error) {
	options := pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}
	if apply {
		options = pgx.TxOptions{IsoLevel: pgx.ReadCommitted}
	}
	tx, err := pool.BeginTx(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("maintenance database connection unavailable")
	}
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='5s';SET LOCAL statement_timeout='20s';SET LOCAL timezone='UTC'`); err != nil {
		rollbackCleanup(tx)
		return nil, err
	}
	return tx, nil
}
func (c *CleanupCommand) Plan(ctx context.Context, phase string, prior *DeletionReceipt) (CleanupPlan, error) {
	md, err := beginCleanup(ctx, c.ModelDev, false)
	if err != nil {
		return CleanupPlan{}, err
	}
	defer rollbackCleanup(md)
	gov, err := beginCleanup(ctx, c.Governance, false)
	if err != nil {
		return CleanupPlan{}, err
	}
	defer rollbackCleanup(gov)
	p, _, _, err := c.planInTransactions(ctx, md, gov, phase, prior)
	return p, err
}
func (c *CleanupCommand) planInTransactions(ctx context.Context, md, gov pgx.Tx, phase string, prior *DeletionReceipt) (CleanupPlan, databaseView, databaseView, error) {
	p := CleanupPlan{Schema: cleanupPlanSchema, Phase: phase, InventorySHA256: c.Original.InventorySHA256, GovernanceInventorySHA256: c.Original.GovernanceInventorySHA256, RootEvidenceSHA256: c.RootSHA256, Blockers: []string{}}
	if phase != "governance" && phase != "modeldev" {
		return p, databaseView{}, databaseView{}, fmt.Errorf("phase must be governance or modeldev")
	}
	mv, me := snapshotDatabase(ctx, md, c.Original, true)
	gv, ge := snapshotDatabase(ctx, gov, c.Original, false)
	p.ModelDev = mv.Snapshot
	p.Governance = gv.Snapshot
	p.Blockers = append(p.Blockers, c.rootBlockers(mv.Snapshot.Identity, gv.Snapshot.Identity)...)
	if me != nil {
		p.Blockers = append(p.Blockers, "MODELDEV_SCHEMA_OR_ROW_SNAPSHOT_UNVERIFIED")
	} else {
		p.Blockers = append(p.Blockers, c.qualifyMD(ctx, md, mv)...)
	}
	if ge != nil {
		p.Blockers = append(p.Blockers, "GOVERNANCE_SCHEMA_OR_ROW_SNAPSHOT_UNVERIFIED")
	} else {
		p.Blockers = append(p.Blockers, qualifyGov(ctx, gov, gv, c.Original, phase == "modeldev")...)
	}
	if phase == "modeldev" {
		p.GovernanceDeletionSHA256 = c.GovernanceReceiptSHA256
		if c.GovernanceReceiptSHA256 == "" || validatePriorReceipt(prior, c.Original) != nil {
			p.Blockers = append(p.Blockers, "GOVERNANCE_ORIGINAL_DELETION_RECEIPT_UNVERIFIED")
		}
	}
	sort.Strings(p.Blockers)
	p.ManifestSHA256 = digest(p)
	return p, mv, gv, nil
}
func (c *CleanupCommand) Apply(ctx context.Context, reviewed CleanupPlan, prior *DeletionReceipt) (DeletionReceipt, error) {
	result := DeletionReceipt{Schema: "ani.modeldev.legacy-db-cleanup.receipt.v1", Phase: reviewed.Phase, Plan: reviewed}
	check := reviewed
	sha := check.ManifestSHA256
	check.ManifestSHA256 = ""
	if reviewed.Schema != cleanupPlanSchema || sha == "" || digest(check) != sha || len(reviewed.Blockers) != 0 {
		return result, fmt.Errorf("reviewed complete manifest without BLOCKED qualifications required")
	}
	md, err := beginCleanup(ctx, c.ModelDev, true)
	if err != nil {
		return result, err
	}
	defer rollbackCleanup(md)
	gov, err := beginCleanup(ctx, c.Governance, true)
	if err != nil {
		return result, err
	}
	defer rollbackCleanup(gov)
	if err = lockTables(ctx, md, true); err != nil {
		return result, fmt.Errorf("ModelDev in-flight identity/row lock not drained")
	}
	if err = lockTables(ctx, gov, false); err != nil {
		return result, fmt.Errorf("Governance in-flight ledger/lease lock not drained")
	}
	current, mv, gv, err := c.planInTransactions(ctx, md, gov, reviewed.Phase, prior)
	if err != nil {
		return result, err
	}
	if len(current.Blockers) != 0 || current.ManifestSHA256 != sha {
		return result, fmt.Errorf("under-lock manifest drift; md_catalog_changed=%t md_tables_changed=%t gov_changed=%t blockers=%v; no DELETE committed", current.ModelDev.CatalogSHA256 != reviewed.ModelDev.CatalogSHA256, digest(current.ModelDev.Tables) != digest(reviewed.ModelDev.Tables), digest(current.Governance) != digest(reviewed.Governance), current.Blockers)
	}
	target, view := gov, gv
	if reviewed.Phase == "modeldev" {
		target, view = md, mv
	}
	result.Deleted, err = deleteExactRows(ctx, target, view)
	if err != nil {
		return result, fmt.Errorf("exact DELETE count or FK/trigger failure; current database transaction rolled back")
	}
	if reviewed.Phase == "governance" {
		after, err := snapshotDatabase(ctx, gov, c.Original, false)
		if err != nil || after.Snapshot.AccountsSHA256 != gv.Snapshot.AccountsSHA256 || len(qualifyGov(ctx, gov, after, c.Original, true)) != 0 {
			return result, fmt.Errorf("post-delete preserved accounts or all-owner sum invariant differs; rollback")
		}
		result.RetainedCharges = table(gv, "sys_quota_charges").rows
		result.RetainedReleaseReceipts = table(gv, "sys_quota_release_receipts").rows
		result.AccountsBefore = gv.Accounts
		result.AccountsAfter = after.Accounts
	} else {
		after, err := snapshotDatabase(ctx, md, c.Original, true)
		if err != nil {
			return result, fmt.Errorf("post-delete ModelDev snapshot unavailable; rollback")
		}
		for _, table := range after.Tables {
			if len(table.rows) != 0 {
				return result, fmt.Errorf("old ModelDev target rows remain; rollback")
			}
		}
	}
	if c.Root == nil || !time.Now().UTC().Before(c.Root.ExpiresAt) {
		return result, fmt.Errorf("root external proof expired before commit; rollback")
	}
	if err = target.Commit(ctx); err != nil {
		return result, fmt.Errorf("commit response UNKNOWN; inspect original exact IDs before any retry")
	}
	result.Applied = true
	result.CompletedAt = time.Now().UTC()
	return result, nil
}
