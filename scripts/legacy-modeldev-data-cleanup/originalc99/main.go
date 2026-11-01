// Command originalc99 qualifies seven immutable historical aggregates using
// their original native module. It has no write mode or runtime consumer.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/lifecycle"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/submission"
)

const codecCommit = "c99c9f1081c754f7e38ceccd056db9a032ca3c27"
const originalTenant = "64c5c35a-f554-432c-b4bc-7b4f36f08420"

type originalIdentity struct{ ExecutionID, OperationID, SpecHash string }

// Actual identities are populated below from the fixed reviewed original set.
var originals = []originalIdentity{
	{"111952ee-9d72-46d9-9981-c2a1ead73289", "8cb22cc3-04cf-4be8-b781-6c4781ef445f", "d0a5d82fc032f3d6b5571a0c13817611efdb8f361520684bce98d93f33c01fbf"},
	{"4534a7de-480c-4a8f-ae76-e647cf7c158c", "644dfe94-5311-4cc5-9a33-c6296de0fecb", "637373fc4213532e0724392564d9c2fb437575b5593eb88760f60b303ba93624"},
	{"5953c2e1-107e-4d46-af6d-4c718c7fb10c", "badf5481-6202-4619-8173-ece940ff972c", "0208188cf2da722447cf84fbaa1459142a1fc4300f57360700971ec4c1d31208"},
	{"5f0b866e-31d1-4b44-8a7c-5cc900767e00", "52ab36df-8e61-41cf-9519-b76aee82ebbd", "bd1fe7a7524ca89f61702eeb28a4ed3b74a8e39b0da30512dfeb3b80013eb12c"},
	{"c0fa3739-d279-4bb3-85be-0f74b8d4773a", "fda33eaa-0997-4655-b53c-c32d6652a44e", "9251e87631c02b1b0f33803644240f1a0c5c3bdcf8e50602e0df3ed82cad5906"},
	{"e94344af-a825-4ba2-9fa1-838638cc4d5f", "8c42a0b2-e603-4e85-bdd1-30f649d7c6cc", "a7706257d2a8afca898447799b7ac35ba6db1bb6bb1f8ceb9c81dbbd7e641639"},
	{"ec57bafc-56bd-4f7e-8f65-45527aa4d0e3", "b448bc04-fe3f-4322-9c16-f08e7c8090db", "2472153d80484cd6ee777afa9aebdc5f1b5275d861ef294b6e1fca3bb822938d"},
}

// Hashes came from the fixed original bytes, not a current-codec conversion.
// SpecHash is also the SHA256 of each original snapshot_canonical payload.
var originalBytes = map[string]struct{ IntentSHA256, ActorAcceptedAtSHA256 string }{
	"111952ee-9d72-46d9-9981-c2a1ead73289": {"a3d07a262708dbdfb31c9cbc905034eebbe0a330e51bf677662274e9683907c8", "8beba25b05d4bbc436a8a8814637f14fe3145d353125ff501d4acef9fdf5efd9"},
	"4534a7de-480c-4a8f-ae76-e647cf7c158c": {"36090062cf8f4bba5fb91295451f004f7c4aca5955e5e83ae915baec858ecfe4", "c394787963588897d5854e583ae55a52afbea22cb88a06bbfa904e84c93ba3cd"},
	"5953c2e1-107e-4d46-af6d-4c718c7fb10c": {"bdab8a06e0ceb458b06febde6828c9e3cddd7cf5f275874c444e6ca2254b31f9", "258c7732b2c8ef56aafb3a6140b2d7ac6a245f4ece2ae49280c3402fbe18ab54"},
	"5f0b866e-31d1-4b44-8a7c-5cc900767e00": {"af8657ef4ac7fba245180bbb0fb682c42043dd2d8656f8912686f8c5e0924277", "a08370f8871ce34dd343b1a592be308661793f830b37800347734d548f18209e"},
	"c0fa3739-d279-4bb3-85be-0f74b8d4773a": {"f94ba5a636a7afe1d3f6a2af31fa7004bc5624ea95ead850c57f9af9d2a7f4da", "d6b7bdd28f25f6e543478b66c61fac5f223e49df583261396af1037808154f5d"},
	"e94344af-a825-4ba2-9fa1-838638cc4d5f": {"76930a4cc52a9b1a5d127189e3045caee413670aaccb8ec967bbca046287272e", "e89546305fec2471c0c4b3e8af5fd2dc74e38a3c63f2f9475e8ac6318fd0d963"},
	"ec57bafc-56bd-4f7e-8f65-45527aa4d0e3": {"b2c099ad82a2a1fb9702dd2507f066929aa0838a322b0819fda9e308c23fcbf8", "06c1386c060073b9d316b5c6611a9156e88d3e0bfd1497455132fc19b6d694dc"},
}
var tables = []string{"modeldev_executions", "modeldev_execution_identities", "modeldev_execution_runtimes", "modeldev_pipeline_dispatches", "modeldev_pipeline_confirmed_runs", "modeldev_run_authorities", "modeldev_close_intents", "modeldev_execution_cleanup_audits"}

type databaseIdentity struct {
	Database, Schema, Role, ServerAddress string
	Port                                  int
	DatabaseOID                           uint32
}
type record struct {
	ExecutionID, OperationID, SpecHash, Predicate, Guard string
	Rows                                                 map[string][]string
	OwnerRevision, CloseGeneration                       uint64
	ClosedAt                                             *time.Time
	EventSHA256, ACKSHA256                               string
}
type qualification struct {
	Schema, CodecCommit, QualifierBinarySHA256 string
	ObservedAt, ExpiresAt                      time.Time
	Database                                   databaseIdentity
	Records                                    []record
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func bytesDigest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func text(r map[string]json.RawMessage, k string) string {
	var s string
	_ = json.Unmarshal(r[k], &s)
	return s
}
func payload(r map[string]json.RawMessage, k string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(text(r, k), "\\x"))
}
func originalAdmission(r map[string]json.RawMessage, w originalIdentity) (biz.Execution, error) {
	invalid := errors.New("ORIGINAL_C99_CANONICAL_IDENTITY_INVALID")
	if text(r, "tenant_id") != originalTenant || text(r, "execution_id") != w.ExecutionID || text(r, "operation_id") != w.OperationID || text(r, "spec_hash") != w.SpecHash {
		return biz.Execution{}, invalid
	}
	ir, e := payload(r, "intent_canonical")
	if e != nil {
		return biz.Execution{}, invalid
	}
	sr, e := payload(r, "snapshot_canonical")
	if e != nil {
		return biz.Execution{}, invalid
	}
	immutable, ok := originalBytes[w.ExecutionID]
	if !ok || bytesDigest(ir) != immutable.IntentSHA256 || bytesDigest(sr) != w.SpecHash || digest(map[string]string{"actor": text(r, "actor"), "accepted_at": text(r, "accepted_at")}) != immutable.ActorAcceptedAtSHA256 {
		return biz.Execution{}, invalid
	}
	intent, e := cpup01.DecodeCanonicalIntent(ir)
	if e != nil {
		return biz.Execution{}, invalid
	}
	snapshot, e := cpup01.DecodeCanonicalSnapshot(sr)
	if e != nil {
		return biz.Execution{}, invalid
	}
	accepted, e := time.Parse(time.RFC3339Nano, text(r, "accepted_at"))
	if e != nil {
		return biz.Execution{}, invalid
	}
	a := biz.Admission{TenantID: originalTenant, ExecutionID: w.ExecutionID, OperationID: w.OperationID, SpecHash: w.SpecHash, Actor: text(r, "actor"), IntentHash: text(r, "intent_hash"), Intent: intent, Snapshot: snapshot, AcceptedAt: accepted}
	ci, cs, e := a.CanonicalPayloads()
	if e != nil || !bytes.Equal(ci, ir) || !bytes.Equal(cs, sr) {
		return biz.Execution{}, invalid
	}
	return biz.Execution{Admission: a}, nil
}
func readRows(ctx context.Context, tx pgx.Tx, schema, table, id string) ([]map[string]json.RawMessage, error) {
	rs, e := tx.Query(ctx, "SELECT to_jsonb(t) FROM "+pgx.Identifier{schema, table}.Sanitize()+" t WHERE tenant_id=$1::uuid AND execution_id=$2::uuid", originalTenant, id)
	if e != nil {
		return nil, e
	}
	defer rs.Close()
	out := []map[string]json.RawMessage{}
	for rs.Next() {
		var raw []byte
		if rs.Scan(&raw) != nil || len(raw) > 2<<20 || len(out) >= 64 {
			return nil, errors.New("ORIGINAL_C99_ROW_BOUND")
		}
		var r map[string]json.RawMessage
		if json.Unmarshal(raw, &r) != nil {
			return nil, errors.New("ORIGINAL_C99_ROW_INVALID")
		}
		out = append(out, r)
	}
	return out, rs.Err()
}
func qualify(ctx context.Context, pool *pgxpool.Pool) (qualification, error) {
	out := qualification{Schema: "ani.modeldev.original-c99-db-qualification.v1", CodecCommit: codecCommit, Records: []record{}}
	path, e := os.Executable()
	if e != nil {
		return out, errors.New("ORIGINAL_C99_BINARY_IDENTITY")
	}
	binary, e := os.ReadFile(path)
	if e != nil {
		return out, errors.New("ORIGINAL_C99_BINARY_IDENTITY")
	}
	sum := sha256.Sum256(binary)
	out.QualifierBinarySHA256 = hex.EncodeToString(sum[:])
	tx, e := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return out, errors.New("ORIGINAL_C99_READ_ONLY_BEGIN")
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SET LOCAL statement_timeout='15000';SET LOCAL timezone='UTC'`); e != nil {
		return out, e
	}
	e = tx.QueryRow(ctx, `SELECT current_database(),current_schema(),current_user,coalesce(host(inet_server_addr()),''),coalesce(inet_server_port(),0),(SELECT oid FROM pg_database WHERE datname=current_database()),clock_timestamp()`).Scan(&out.Database.Database, &out.Database.Schema, &out.Database.Role, &out.Database.ServerAddress, &out.Database.Port, &out.Database.DatabaseOID, &out.ObservedAt)
	if e != nil {
		return out, errors.New("ORIGINAL_C99_DATABASE_IDENTITY")
	}
	out.ExpiresAt = out.ObservedAt.Add(5 * time.Minute)
	for _, w := range originals {
		r := record{ExecutionID: w.ExecutionID, OperationID: w.OperationID, SpecHash: w.SpecHash, Rows: map[string][]string{}}
		all := map[string][]map[string]json.RawMessage{}
		for _, table := range tables {
			rows, e := readRows(ctx, tx, out.Database.Schema, table, w.ExecutionID)
			if e != nil {
				return out, errors.New("ORIGINAL_C99_EXACT_ROWS_UNAVAILABLE")
			}
			all[table] = rows
			r.Rows[table] = []string{}
			for _, v := range rows {
				r.Rows[table] = append(r.Rows[table], digest(v))
			}
			sort.Strings(r.Rows[table])
		}
		if len(all["modeldev_executions"]) != 1 || len(all["modeldev_execution_identities"]) != 1 || len(all["modeldev_execution_runtimes"]) != 1 {
			r.Guard = "ORIGINAL_AGGREGATE_ROWCOUNT"
			out.Records = append(out.Records, r)
			continue
		}
		execution, e := originalAdmission(all["modeldev_executions"][0], w)
		if e != nil {
			r.Guard = "ORIGINAL_C99_CANONICAL_IDENTITY_INVALID"
			out.Records = append(out.Records, r)
			continue
		}
		// Reject newer/foreign runtime fields, including all retirement markers.
		var facts biz.ExecutionRuntime
		d := json.NewDecoder(bytes.NewReader(all["modeldev_execution_runtimes"][0]["facts"]))
		d.DisallowUnknownFields()
		if d.Decode(&facts) != nil {
			r.Guard = "ORIGINAL_C99_RUNTIME_SHAPE"
			out.Records = append(out.Records, r)
			continue
		}
		state, e := lifecycle.ReadInTransaction(ctx, tx, originalTenant, w.ExecutionID)
		if e != nil {
			r.Guard = "ORIGINAL_C99_NORMAL_RUNTIME_READER"
			out.Records = append(out.Records, r)
			continue
		}
		if state.ClosedAt == nil || state.CloseGeneration == 0 || state.CloseReviewReason != "" || state.CloseEvidence == nil {
			r.Guard = "ORIGINAL_NOT_CLOSED"
			out.Records = append(out.Records, r)
			continue
		}
		noResources := state.Workspace == nil && state.Training == nil && state.TrainingHandle == nil && state.Observation == nil && state.Publication == nil
		if !state.CloseEvidence.NoDispatch && !noResources {
			a, e := submission.ReadAuthorityInTransaction(ctx, tx, originalTenant, w.ExecutionID)
			owner := a.RunAuthorityCandidate
			if e != nil && state.CloseAuthority != nil {
				owner, e = *state.CloseAuthority, nil
			}
			if e != nil || state.StepExitCleanup == nil || state.StepExitCleanup.CompletedAt == nil || !biz.ComputeCleanupApplied(biz.OperationRecord{QueryRecord: biz.QueryRecord{Execution: execution, Runtime: state}, Authority: &owner}) {
				r.Guard = "ORIGINAL_C99_CLEANUP_NOT_APPLIED"
				out.Records = append(out.Records, r)
				continue
			}
		}
		if execution.Snapshot.ComputeOrigin.DeviceMode == "gpu" {
			f := state.GPURelease
			if f == nil || f.Outbox == nil || f.Outbox.ACK == nil || f.Outbox.DeliveredAt == nil {
				r.Guard = "ORIGINAL_GPU_OUTBOX_ACK_MISSING"
				out.Records = append(out.Records, r)
				continue
			}
			event, ack := f.Outbox.Event, *f.Outbox.ACK
			if event.CreateOperationID != w.OperationID || event.ResourceID != w.ExecutionID || ack.CreateOperationID != event.CreateOperationID || ack.ResourceID != event.ResourceID || ack.ReleaseEventID != event.ReleaseEventID || (ack.Status != "APPLIED" && ack.Status != "ALREADY_APPLIED") {
				r.Guard = "ORIGINAL_GPU_ACK_IDENTITY_DIFFERS"
				out.Records = append(out.Records, r)
				continue
			}
			r.EventSHA256, r.ACKSHA256 = digest(event), digest(ack)
		}
		r.Predicate = "ORIGINAL_C99_NATIVE_CLOSED_CLEANUP_ACK"
		r.OwnerRevision, r.CloseGeneration, r.ClosedAt = state.OwnerRevision, state.CloseGeneration, state.ClosedAt
		out.Records = append(out.Records, r)
	}
	if e = tx.Commit(ctx); e != nil {
		return out, errors.New("ORIGINAL_C99_READ_ONLY_COMMIT")
	}
	return out, nil
}
func run() error {
	path := flag.String("dsn-file", "", "explicit protected original MD connection reference; reads only")
	flag.Parse()
	i, e := os.Lstat(*path)
	if e != nil || !i.Mode().IsRegular() || i.Mode().Perm() != 0600 || i.Size() > 1<<20 {
		return errors.New("PRIVATE_ORIGINAL_MD_DSN_REQUIRED")
	}
	raw, e := os.ReadFile(*path)
	if e != nil {
		return errors.New("PRIVATE_ORIGINAL_MD_DSN_UNAVAILABLE")
	}
	cfg, e := pgxpool.ParseConfig(strings.TrimSpace(string(raw)))
	if e != nil {
		return errors.New("ORIGINAL_C99_CONNECTION_INVALID")
	}
	cfg.MaxConns = 2
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return errors.New("ORIGINAL_C99_CONNECTION_UNAVAILABLE")
	}
	defer p.Close()
	out, e := qualify(ctx, p)
	if e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
