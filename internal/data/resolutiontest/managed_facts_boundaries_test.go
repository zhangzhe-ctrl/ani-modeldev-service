package resolutiontest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/admissionfacts"
)

func TestManagedFactsLoadRejectsMissingRequiredFields(t *testing.T) {
	fixture := prepareResolution(t)
	raw := managedFixtureBytes(t, fixture)
	// Re-pin every edited actual file: these failures must concern required
	// fields, not a stale byte hash. Missing fixed facts are the expected RED;
	// already-checked schema/key/evidence fields remain regression coverage.
	for _, field := range []string{
		"schema_version", "resource_tenant_id", "release_id", "release_digest",
		"environment", "input_scope", "publication_scope", "runtime", "workspace",
		"input_file_path", "output_directory_path", "environment_evidence", "application_evidence",
		"environment.namespace_uid", "environment.binding_digest", "environment.identities",
		"environment.identities.trainer_service_account", "runtime.name", "runtime.content_sha256", "runtime.target_jobs",
		"input_scope.storage_connection_id", "input_scope.approved_prefix", "publication_scope.credential_reference",
		"workspace.capacity_bytes", "workspace.input_subpath", "environment_evidence.sha256",
	} {
		t.Run(field, func(t *testing.T) {
			source := pinManagedFactsBytes(t, editManagedFactsField(t, raw, strings.Split(field, "."), nil))
			requireManagedFactsLoadFailure(t, fixture.ctx, []admissionfacts.FileSource{source}, admissionfacts.ErrInvalidFacts)
		})
	}
}

func TestManagedFactsLoadRejectsAmbiguousAndUntypedFileBytes(t *testing.T) {
	fixture := prepareResolution(t)
	raw := managedFixtureBytes(t, fixture)
	replace := func(old, replacement string) []byte {
		if !bytes.Contains(raw, []byte(old)) {
			t.Fatalf("MANAGED_FACTS_PREFLIGHT: mutation target absent; behavior NOT_RUN")
		}
		return bytes.Replace(raw, []byte(old), []byte(replacement), 1)
	}
	for _, test := range []struct {
		name string
		raw []byte
	}{
		{"unknown root field", replace(`{"schema_version":`, `{"current":true,"schema_version":`)},
		{"unknown nested field", replace(`"runtime":{`, `"runtime":{"verified":true,`)},
		{"duplicate root field", replace(`{"schema_version":`, `{"release_digest":"` + fixture.selection.Release.ReleaseDigest + `","schema_version":`)},
		{"duplicate nested field", replace(`"name":"cpu-module-fixture"`, `"name":"cpu-module-fixture","name":"cpu-module-fixture"`)},
		{"field case alias", replace(`"runtime":`, `"Runtime":`)},
		{"nested case alias", replace(`"target_jobs":`, `"Target_Jobs":`)},
		{"null facts", editManagedFactsField(t, raw, []string{"environment"}, json.RawMessage(`null`))},
		{"null nested string", editManagedFactsField(t, raw, []string{"input_scope", "credential_reference"}, json.RawMessage(`null`))},
		{"wrong typed capacity", editManagedFactsField(t, raw, []string{"workspace", "capacity_bytes"}, json.RawMessage(`1073741824`))},
		{"wrong typed targets", editManagedFactsField(t, raw, []string{"runtime", "target_jobs"}, json.RawMessage(`"trainer"`))},
		{"trailing object", append(bytes.Clone(raw), []byte(`{}`)...)},
		{"invalid UTF8", replace("module-fixture-cluster", "module-\xff")},
		{"unpaired UTF16", replace("module-fixture-cluster", `module-\ud800`)},
		{"unsupported schema", editManagedFactsField(t, raw, []string{"schema_version"}, json.RawMessage(`"unknown.v2"`))},
		{"zero tenant", editManagedFactsField(t, raw, []string{"resource_tenant_id"}, json.RawMessage(`"00000000-0000-0000-0000-000000000000"`))},
		{"evidence is credential URL", editManagedFactsField(t, raw, []string{"application_evidence", "reference"}, json.RawMessage(`"https://example.test/?token=private-fixture-token"`))},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := pinManagedFactsBytes(t, test.raw)
			requireManagedFactsLoadFailure(t, fixture.ctx, []admissionfacts.FileSource{source}, admissionfacts.ErrInvalidFacts)
		})
	}
}

func TestManagedFactsLoadPinsActualBytesAndBoundsAllSources(t *testing.T) {
	fixture := prepareResolution(t)
	raw := managedFixtureBytes(t, fixture)
	original := pinManagedFactsBytes(t, raw)
	changed := pinManagedFactsBytes(t, append(bytes.Clone(raw), '\n'))
	if changed.SHA256 == original.SHA256 {
		t.Fatal("MANAGED_FACTS_PREFLIGHT: changed raw bytes must change external SHA; behavior NOT_RUN")
	}
	stalePin := changed
	stalePin.SHA256 = original.SHA256
	requireManagedFactsLoadFailure(t, fixture.ctx, []admissionfacts.FileSource{stalePin}, admissionfacts.ErrInvalidFacts)
	reader, err := admissionfacts.Load(fixture.ctx, []admissionfacts.FileSource{changed})
	if err != nil {
		t.Fatalf("equivalent JSON with its own exact byte pin was rejected: %v", err)
	}
	got, err := reader.ReadAdmissionFacts(fixture.ctx, fixture.selection.TenantID, fixture.selection.Release.ReleaseID, fixture.selection.Release.ReleaseDigest)
	if err != nil || !reflect.DeepEqual(got, fixture.facts) {
		t.Fatalf("non-semantic whitespace changed the fixed facts: %v", err)
	}
	// Both filenames exist, have independently fixed bytes, and select the same
	// key. The entire load must fail instead of choosing the last/default file.
	requireManagedFactsLoadFailure(t, fixture.ctx, []admissionfacts.FileSource{original, changed}, admissionfacts.ErrInvalidFacts)
	requireManagedFactsLoadFailure(t, fixture.ctx, nil, admissionfacts.ErrInvalidFacts)
	requireManagedFactsLoadFailure(t, fixture.ctx, []admissionfacts.FileSource{pinManagedFactsBytes(t, nil)}, admissionfacts.ErrInvalidFacts)
	maximum := append(bytes.Clone(raw), bytes.Repeat([]byte{' '}, 64*1024-len(raw))...)
	if _, err := admissionfacts.Load(fixture.ctx, []admissionfacts.FileSource{pinManagedFactsBytes(t, maximum)}); err != nil {
		t.Fatalf("exact 64 KiB valid file was rejected: %v", err)
	}
	requireManagedFactsLoadFailure(t, fixture.ctx, []admissionfacts.FileSource{pinManagedFactsBytes(t, append(maximum, ' '))}, admissionfacts.ErrInvalidFacts)
	// Distinct real files with distinct keys make this a source-count limit
	// assertion; duplicated keys must not accidentally make the test pass.
	sources := make([]admissionfacts.FileSource, 65)
	for index := range sources {
		tenant := json.RawMessage(fmt.Sprintf(`"11111111-2222-4333-8444-%012d"`, index+1))
		sources[index] = pinManagedFactsBytes(t, editManagedFactsField(t, raw, []string{"resource_tenant_id"}, tenant))
	}
	if _, err := admissionfacts.Load(fixture.ctx, sources[:64]); err != nil {
		t.Fatalf("64 explicitly selected fixed sources were rejected: %v", err)
	}
	requireManagedFactsLoadFailure(t, fixture.ctx, sources, admissionfacts.ErrInvalidFacts)
}

func TestManagedFactsLoadRejectsUnsafeActualFileTypesAndPaths(t *testing.T) {
	fixture := prepareResolution(t)
	raw := managedFixtureBytes(t, fixture)
	for _, kind := range []string{"missing", "directory", "symlink", "hardlink", "fifo", "unclean path"} {
		t.Run(kind, func(t *testing.T) {
			source := pinManagedFactsBytes(t, raw)
			target := filepath.Join(t.TempDir(), "selected")
			want := admissionfacts.ErrInvalidFacts
			var err error
			switch kind {
			case "missing":
				want = admissionfacts.ErrFactsUnavailable
			case "directory":
				err = os.Mkdir(target, 0700)
			case "symlink":
				err = os.Symlink(source.Path, target)
				want = admissionfacts.ErrFactsUnavailable
			case "hardlink":
				err = os.Link(source.Path, target)
			case "fifo":
				err = syscall.Mkfifo(target, 0600)
			case "unclean path":
				target = filepath.Dir(source.Path) + "/./" + filepath.Base(source.Path)
			}
			if err != nil {
				t.Fatalf("MANAGED_FACTS_PREFLIGHT: actual file-type fixture failed; behavior NOT_RUN: %v", err)
			}
			source.Path = target
			requireManagedFactsLoadFailure(t, fixture.ctx, []admissionfacts.FileSource{source}, want)
		})
	}
}

func TestManagedFactsReaderUsesExactTenantReleaseKeysAndIndependentLoadedValues(t *testing.T) {
	fixture := prepareResolution(t)
	raw := managedFixtureBytes(t, fixture)
	const otherTenant = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	const otherRelease = "abcdefab-cdef-4abc-8def-abcdefabcdef"
	otherDigest := strings.Repeat("c", 64)
	tenantRaw := editManagedFactsField(t, raw, []string{"resource_tenant_id"}, json.RawMessage(`"`+otherTenant+`"`))
	tenantRaw = editManagedFactsField(t, tenantRaw, []string{"environment", "cluster_id"}, json.RawMessage(`"module-fixture-tenant-b"`))
	releaseRaw := editManagedFactsField(t, raw, []string{"release_id"}, json.RawMessage(`"`+otherRelease+`"`))
	releaseRaw = editManagedFactsField(t, releaseRaw, []string{"release_digest"}, json.RawMessage(`"`+otherDigest+`"`))
	releaseRaw = editManagedFactsField(t, releaseRaw, []string{"output_directory_path"}, json.RawMessage(`"/other-release-output"`))
	sources := []admissionfacts.FileSource{pinManagedFactsBytes(t, raw), pinManagedFactsBytes(t, tenantRaw), pinManagedFactsBytes(t, releaseRaw)}
	reader, err := admissionfacts.Load(fixture.ctx, sources)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		tenant string
		release string
		digest string
		want error
	}{
		{"other tenant", strings.ToUpper(otherTenant), fixture.selection.Release.ReleaseID, fixture.selection.Release.ReleaseDigest, nil},
		{"other release", fixture.selection.TenantID, strings.ToUpper(otherRelease), otherDigest, nil},
		{"unbound tenant", "99999999-9999-4999-8999-999999999999", fixture.selection.Release.ReleaseID, fixture.selection.Release.ReleaseDigest, biz.ErrAdmissionEnvironmentNotReady},
		{"wrong digest", fixture.selection.TenantID, otherRelease, fixture.selection.Release.ReleaseDigest, biz.ErrAdmissionEnvironmentNotReady},
		{"unbound release", otherTenant, otherRelease, otherDigest, biz.ErrAdmissionEnvironmentNotReady},
		{"malformed tenant", "not-a-tenant", otherRelease, otherDigest, biz.ErrInvalidAdmission},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := reader.ReadAdmissionFacts(fixture.ctx, test.tenant, test.release, test.digest)
			if !errors.Is(err, test.want) || (test.want != nil && !reflect.DeepEqual(got, biz.TenantAdmissionFacts{})) {
				t.Fatalf("wrong key selected a different tenant/Release or leaked partial facts: %v", err)
			}
			if test.name == "other tenant" && (got.TenantID != otherTenant || got.Environment.ClusterID != "module-fixture-tenant-b") {
				t.Fatal("tenant lookup did not select its own exact loaded value")
			}
			if test.name == "other release" && got.OutputDirectoryPath != "/other-release-output" {
				t.Fatal("explicit Release lookup selected a different file or default")
			}
		})
	}
	// Loading fixes the bytes once. Later disk/config changes cannot mutate this
	// reader; an independent new load must still check the original external pin.
	if err := os.WriteFile(sources[0].Path, []byte(`{}`), 0600); err != nil {
		t.Fatalf("MANAGED_FACTS_PREFLIGHT: fixture mutation failed; behavior NOT_RUN: %v", err)
	}
	requireManagedFactsLoadFailure(t, fixture.ctx, sources[:1], admissionfacts.ErrInvalidFacts)
	sources[0].SHA256 = "caller-change"
	got, err := reader.ReadAdmissionFacts(fixture.ctx, fixture.selection.TenantID, fixture.selection.Release.ReleaseID, fixture.selection.Release.ReleaseDigest)
	if err != nil || !reflect.DeepEqual(got, fixture.facts) {
		t.Fatalf("loaded value followed a mutable source instead of its pinned bytes: %v", err)
	}
	got.Runtime.TargetJobs[0] = "caller-change"
	again, err := reader.ReadAdmissionFacts(fixture.ctx, fixture.selection.TenantID, fixture.selection.Release.ReleaseID, fixture.selection.Release.ReleaseDigest)
	if err != nil || !reflect.DeepEqual(again, fixture.facts) {
		t.Fatalf("returned slice changed the immutable loaded facts: %v", err)
	}
}

func TestManagedFactsCancellationReturnsNoReaderFactsOrCandidate(t *testing.T) {
	fixture := prepareResolution(t)
	source := writeManagedFactsFixture(t, fixture)
	reader, err := admissionfacts.Load(fixture.ctx, []admissionfacts.FileSource{source})
	if err != nil {
		t.Fatal(err)
	}
	resolver := biz.NewManagedAdmissionResolver(fixture.releaseReader, fixture.inputReader, reader)
	canceled, cancel := context.WithCancel(fixture.ctx)
	cancel()
	expired, finish := context.WithDeadline(fixture.ctx, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	defer finish()
	for _, test := range []struct {
		name string
		ctx context.Context
		want error
	}{{"canceled", canceled, context.Canceled}, {"expired", expired, context.DeadlineExceeded}} {
		t.Run(test.name, func(t *testing.T) {
			requireManagedFactsLoadFailure(t, test.ctx, []admissionfacts.FileSource{source}, test.want)
			facts, err := reader.ReadAdmissionFacts(test.ctx, fixture.selection.TenantID, fixture.selection.Release.ReleaseID, fixture.selection.Release.ReleaseDigest)
			if !errors.Is(err, test.want) || !reflect.DeepEqual(facts, biz.TenantAdmissionFacts{}) {
				t.Fatalf("canceled lookup returned facts or hid cancellation: %v", err)
			}
			got, err := resolver.ResolveManaged(test.ctx, fixture.selection)
			if !errors.Is(err, test.want) || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
				t.Fatalf("canceled managed resolution returned a candidate or hid cancellation: %v", err)
			}
		})
	}
	requireNoResolvedExecution(t, fixture)
}

func managedFixtureBytes(t *testing.T, fixture resolutionFixture) []byte {
	t.Helper()
	source := writeManagedFactsFixture(t, fixture)
	raw, err := os.ReadFile(source.Path)
	if err != nil {
		t.Fatalf("MANAGED_FACTS_PREFLIGHT: fixture read failed; behavior NOT_RUN: %v", err)
	}
	return raw
}

func pinManagedFactsBytes(t *testing.T, raw []byte) admissionfacts.FileSource {
	t.Helper()
	file := filepath.Join(t.TempDir(), "selected-facts.json")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatalf("MANAGED_FACTS_PREFLIGHT: fixture write failed; behavior NOT_RUN: %v", err)
	}
	digest := sha256.Sum256(raw)
	return admissionfacts.FileSource{Path: file, SHA256: hex.EncodeToString(digest[:])}
}

func editManagedFactsField(t *testing.T, raw []byte, fields []string, value json.RawMessage) []byte {
	t.Helper()
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || len(fields) == 0 || object[fields[0]] == nil {
		t.Fatal("MANAGED_FACTS_PREFLIGHT: mutation path absent; behavior NOT_RUN")
	}
	if len(fields) > 1 {
		object[fields[0]] = editManagedFactsField(t, object[fields[0]], fields[1:], value)
	} else if value == nil {
		delete(object, fields[0])
	} else {
		object[fields[0]] = value
	}
	changed, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("MANAGED_FACTS_PREFLIGHT: fixture edit encoding failed; behavior NOT_RUN: %v", err)
	}
	return changed
}

func requireManagedFactsLoadFailure(t *testing.T, ctx context.Context, sources []admissionfacts.FileSource, want error) {
	t.Helper()
	type result struct {
		reader *admissionfacts.Reader
		err error
	}
	done := make(chan result, 1)
	go func() {
		reader, err := admissionfacts.Load(ctx, sources)
		done <- result{reader: reader, err: err}
	}()
	select {
	case got := <-done:
		if !errors.Is(got.err, want) || got.reader != nil {
			t.Fatalf("unsafe/incomplete file set must fail atomically with no reader: got %v; want %v", got.err, want)
		}
		for _, source := range sources {
			if strings.Contains(got.err.Error(), source.Path) || strings.Contains(got.err.Error(), "private-fixture-token") {
				t.Fatal("load failure exposed a filesystem path or raw fixture credential")
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("bounded file loading blocked on an unsafe source")
	}
}
