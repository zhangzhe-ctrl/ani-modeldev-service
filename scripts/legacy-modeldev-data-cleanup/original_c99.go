package main

import (
	"encoding/hex"
	"sort"
	"time"
)

const originalC99Commit = "c99c9f1081c754f7e38ceccd056db9a032ca3c27"

var originalC99Identities = map[string]ExecutionIdentity{
	"111952ee-9d72-46d9-9981-c2a1ead73289": {"64c5c35a-f554-432c-b4bc-7b4f36f08420", "111952ee-9d72-46d9-9981-c2a1ead73289", "8cb22cc3-04cf-4be8-b781-6c4781ef445f", "d0a5d82fc032f3d6b5571a0c13817611efdb8f361520684bce98d93f33c01fbf"},
	"4534a7de-480c-4a8f-ae76-e647cf7c158c": {"64c5c35a-f554-432c-b4bc-7b4f36f08420", "4534a7de-480c-4a8f-ae76-e647cf7c158c", "644dfe94-5311-4cc5-9a33-c6296de0fecb", "637373fc4213532e0724392564d9c2fb437575b5593eb88760f60b303ba93624"},
	"5953c2e1-107e-4d46-af6d-4c718c7fb10c": {"64c5c35a-f554-432c-b4bc-7b4f36f08420", "5953c2e1-107e-4d46-af6d-4c718c7fb10c", "badf5481-6202-4619-8173-ece940ff972c", "0208188cf2da722447cf84fbaa1459142a1fc4300f57360700971ec4c1d31208"},
	"5f0b866e-31d1-4b44-8a7c-5cc900767e00": {"64c5c35a-f554-432c-b4bc-7b4f36f08420", "5f0b866e-31d1-4b44-8a7c-5cc900767e00", "52ab36df-8e61-41cf-9519-b76aee82ebbd", "bd1fe7a7524ca89f61702eeb28a4ed3b74a8e39b0da30512dfeb3b80013eb12c"},
	"c0fa3739-d279-4bb3-85be-0f74b8d4773a": {"64c5c35a-f554-432c-b4bc-7b4f36f08420", "c0fa3739-d279-4bb3-85be-0f74b8d4773a", "fda33eaa-0997-4655-b53c-c32d6652a44e", "9251e87631c02b1b0f33803644240f1a0c5c3bdcf8e50602e0df3ed82cad5906"},
	"e94344af-a825-4ba2-9fa1-838638cc4d5f": {"64c5c35a-f554-432c-b4bc-7b4f36f08420", "e94344af-a825-4ba2-9fa1-838638cc4d5f", "8c42a0b2-e603-4e85-bdd1-30f649d7c6cc", "a7706257d2a8afca898447799b7ac35ba6db1bb6bb1f8ceb9c81dbbd7e641639"},
	"ec57bafc-56bd-4f7e-8f65-45527aa4d0e3": {"64c5c35a-f554-432c-b4bc-7b4f36f08420", "ec57bafc-56bd-4f7e-8f65-45527aa4d0e3", "b448bc04-fe3f-4322-9c16-f08e7c8090db", "2472153d80484cd6ee777afa9aebdc5f1b5275d861ef294b6e1fca3bb822938d"},
}

type originalC99Record struct {
	ExecutionID, OperationID, SpecHash, Predicate, Guard string
	Rows                                                 map[string][]string
	OwnerRevision, CloseGeneration                       uint64
	ClosedAt                                             *time.Time
	EventSHA256, ACKSHA256                               string
}
type originalC99Qualification struct {
	Schema, CodecCommit, QualifierBinarySHA256 string
	ObservedAt, ExpiresAt                      time.Time
	Database                                   DatabaseIdentity
	Records                                    []originalC99Record
}
type originalC99Build struct {
	Schema, CodecCommit, BinarySHA256, SourceSHA256, GoVersion string
	ExitCode                                                   int
}

func originalC99SHA(s string) bool { b, e := hex.DecodeString(s); return e == nil && len(b) == 32 }

// This consumes an explicit root-approved native historical qualification.
// It does not decode a historical command through the current runtime codec.
// Every original database row must still be byte-identical under apply locks.
func (c *CleanupCommand) qualifyOriginalC99(w ExecutionIdentity, v databaseView) string {
	expected, ok := originalC99Identities[w.ExecutionID]
	if !ok || w != expected {
		return "MD_ORIGINAL_C99_FIXED_IDENTITY_DIFFERS"
	}
	if c.Root == nil || c.Root.OriginalC99Qualification == nil || c.Root.OriginalC99Build == nil {
		return "MD_ORIGINAL_C99_QUALIFICATION_UNKNOWN"
	}
	p, b := *c.Root.OriginalC99Qualification, *c.Root.OriginalC99Build
	if p.Kind != "ORIGINAL_C99_NATIVE_QUALIFICATION" || b.Kind != "ORIGINAL_C99_BUILD_RECEIPT" || !proofValid(p) || !proofValid(b) {
		return "MD_ORIGINAL_C99_ROOT_PROOF_INVALID"
	}
	pr, e := readBounded(p.Path, 8<<20, false)
	if e != nil || bytesSHA(pr) != p.SHA256 {
		return "MD_ORIGINAL_C99_ROOT_PROOF_INVALID"
	}
	br, e := readBounded(b.Path, 8<<20, false)
	if e != nil || bytesSHA(br) != b.SHA256 {
		return "MD_ORIGINAL_C99_ROOT_PROOF_INVALID"
	}
	var q originalC99Qualification
	var build originalC99Build
	if strictJSON(pr, &q) != nil || strictJSON(br, &build) != nil {
		return "MD_ORIGINAL_C99_PROOF_SHAPE_INVALID"
	}
	now := time.Now().UTC()
	if q.Schema != "ani.modeldev.original-c99-db-qualification.v1" || q.CodecCommit != originalC99Commit || q.ObservedAt.IsZero() || q.ObservedAt.After(now) || !now.Before(q.ExpiresAt) || q.ExpiresAt.After(q.ObservedAt.Add(5*time.Minute)) || c.Root.ExpiresAt.After(q.ExpiresAt) || !c.readQualificationIdentityMatches(q.Database, v.Snapshot.Identity, "modeldev") || len(q.Records) != 7 {
		return "MD_ORIGINAL_C99_FRESH_DATABASE_PROOF_REQUIRED"
	}
	if build.Schema != "ani.modeldev.original-c99-qualifier-build.v1" || build.CodecCommit != originalC99Commit || build.ExitCode != 0 || !originalC99SHA(build.BinarySHA256) || build.BinarySHA256 != q.QualifierBinarySHA256 || !originalC99SHA(build.SourceSHA256) || build.GoVersion != "go1.26.10" {
		return "MD_ORIGINAL_C99_BUILD_IDENTITY_DIFFERS"
	}
	seen := map[string]bool{}
	var selected *originalC99Record
	for i := range q.Records {
		r := &q.Records[i]
		id, ok := originalC99Identities[r.ExecutionID]
		if !ok || seen[r.ExecutionID] || r.OperationID != id.OperationID || r.SpecHash != id.SpecHash {
			return "MD_ORIGINAL_C99_PROOF_SCOPE_DIFFERS"
		}
		seen[r.ExecutionID] = true
		if r.ExecutionID == w.ExecutionID {
			selected = r
		}
	}
	if selected == nil || selected.Predicate != "ORIGINAL_C99_NATIVE_CLOSED_CLEANUP_ACK" || selected.Guard != "" || selected.ClosedAt == nil || selected.OwnerRevision == 0 || selected.CloseGeneration == 0 {
		return "MD_ORIGINAL_C99_NATIVE_QUALIFICATION_BLOCKED"
	}
	tables := []string{"modeldev_executions", "modeldev_execution_identities", "modeldev_execution_runtimes", "modeldev_pipeline_dispatches", "modeldev_pipeline_confirmed_runs", "modeldev_run_authorities", "modeldev_close_intents", "modeldev_execution_cleanup_audits"}
	if len(selected.Rows) != len(tables) {
		return "MD_ORIGINAL_C99_AGGREGATE_SCOPE_DIFFERS"
	}
	for _, name := range tables {
		actual := []string{}
		for _, r := range table(v, name).rows {
			if text(r, "execution_id") == w.ExecutionID && text(r, "tenant_id") == w.TenantID {
				actual = append(actual, digest(r))
			}
		}
		sort.Strings(actual)
		expected, ok := selected.Rows[name]
		if !ok || digest(actual) != digest(expected) {
			return "MD_ORIGINAL_C99_AGGREGATE_ROW_DRIFT"
		}
	}
	return ""
}
