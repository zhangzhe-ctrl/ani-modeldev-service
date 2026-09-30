package resolutiontest

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/catalogue"
)

func TestResolveAdmissionRequiresCompatibleExplicitManagedFacts(t *testing.T) {
	fixture := prepareResolution(t)
	for _, test := range []struct {
		name string
		change func(*biz.TenantAdmissionFacts)
		want error
	}{
		{"Runtime name", func(f *biz.TenantAdmissionFacts) { f.Runtime.Name = "other-runtime" }, biz.ErrAdmissionEnvironmentNotReady},
		{"Runtime content", func(f *biz.TenantAdmissionFacts) { f.Runtime.ContentSHA256 = strings.Repeat("a", 64) }, biz.ErrAdmissionEnvironmentNotReady},
		{"Runtime target", func(f *biz.TenantAdmissionFacts) { f.Runtime.TargetJobs = []string{"other-trainer"} }, biz.ErrAdmissionEnvironmentNotReady},
		{"workspace storage", func(f *biz.TenantAdmissionFacts) { f.Workspace.StorageClass = "other-storage" }, biz.ErrAdmissionEnvironmentNotReady},
		{"workspace mapping", func(f *biz.TenantAdmissionFacts) { f.Workspace.InputSubpath = "other-input" }, biz.ErrAdmissionEnvironmentNotReady},
		{"input connection", func(f *biz.TenantAdmissionFacts) { f.InputScope.StorageConnectionID = "other-connection" }, biz.ErrAdmissionEnvironmentNotReady},
		{"input bucket", func(f *biz.TenantAdmissionFacts) { f.InputScope.Bucket = "other-bucket" }, biz.ErrAdmissionEnvironmentNotReady},
		{"prefix boundary", func(f *biz.TenantAdmissionFacts) { f.InputScope.ApprovedPrefix = "tenant/in" }, biz.ErrAdmissionEnvironmentNotReady},
		{"relative input", func(f *biz.TenantAdmissionFacts) { f.InputFilePath = "relative/data.csv" }, biz.ErrAdmissionEnvironmentNotReady},
		{"traversal input", func(f *biz.TenantAdmissionFacts) { f.InputFilePath = "/prepared-fixture/../data.csv" }, biz.ErrAdmissionEnvironmentNotReady},
		{"wildcard output", func(f *biz.TenantAdmissionFacts) { f.OutputDirectoryPath = "/trained-*" }, biz.ErrAdmissionEnvironmentNotReady},
		{"same input and output", func(f *biz.TenantAdmissionFacts) { f.OutputDirectoryPath = f.InputFilePath }, biz.ErrAdmissionEnvironmentNotReady},
		{"input below output", func(f *biz.TenantAdmissionFacts) { f.OutputDirectoryPath = "/prepared-fixture" }, biz.ErrAdmissionEnvironmentNotReady},
		{"output below input", func(f *biz.TenantAdmissionFacts) { f.OutputDirectoryPath = f.InputFilePath + "/output" }, biz.ErrAdmissionEnvironmentNotReady},
		{"unknown environment", func(f *biz.TenantAdmissionFacts) { f.Environment.ClusterID = "UNKNOWN" }, biz.ErrInvalidAdmission},
		{"missing namespace identity", func(f *biz.TenantAdmissionFacts) { f.Environment.NamespaceUID = "" }, biz.ErrInvalidAdmission},
		{"shared step and training identity", func(f *biz.TenantAdmissionFacts) { f.Environment.Identities.TrainerServiceAccount = f.Environment.Identities.KFPStepServiceAccount }, biz.ErrInvalidAdmission},
		{"missing publication authority", func(f *biz.TenantAdmissionFacts) { f.PublicationScope.CredentialReference = "" }, biz.ErrInvalidAdmission},
	} {
		t.Run(test.name, func(t *testing.T) {
			facts := fixture.facts
			test.change(&facts)
			got, err := fixture.resolver.Resolve(fixture.ctx, fixture.selection, facts)
			if !errors.Is(err, test.want) || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
				t.Fatalf("incompatible facts produced a candidate or wrong domain error: %v", err)
			}
		})
	}
	// Similar path text is not containment: only whole slash-delimited paths
	// matter, and neither the resolver nor the argv invokes a shell.
	facts := fixture.facts
	facts.OutputDirectoryPath = "/prepared-fixture-output"
	got, err := fixture.resolver.Resolve(fixture.ctx, fixture.selection, facts)
	if err != nil || got.Snapshot.Program.ResolvedArgs[3] != facts.OutputDirectoryPath {
		t.Fatalf("separate explicit output path was rejected or changed: %v", err)
	}
	requireNoResolvedExecution(t, fixture)
}

func TestResolveAdmissionUsesFixedMicrosecondTimeAndCanonicalIdentity(t *testing.T) {
	fixture := prepareResolution(t)
	original, err := fixture.resolver.Resolve(fixture.ctx, fixture.selection, fixture.facts)
	if err != nil {
		t.Fatal(err)
	}
	selection := fixture.selection
	selection.AcceptedAt = time.Date(2026, 9, 30, 20, 3, 0, 123000, time.FixedZone("fixture-UTC+8", 8*60*60))
	selection.Intent.DatasetVersionID = strings.ToUpper(selection.Intent.DatasetVersionID)
	got, err := fixture.resolver.Resolve(fixture.ctx, selection, fixture.facts)
	if err != nil || !reflect.DeepEqual(got, original) || !got.Snapshot.DeadlineAt.Equal(time.Date(2026, 9, 30, 12, 33, 0, 123000, time.UTC)) {
		t.Fatalf("equivalent time/identity changed the frozen configuration: %v", err)
	}
	for _, test := range []struct {
		name string
		accepted time.Time
	}{
		{"missing time", time.Time{}},
		{"submicrosecond time", fixture.selection.AcceptedAt.Add(time.Nanosecond)},
		{"unsupported year", time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"deadline exceeds supported year", time.Date(9999, 12, 31, 23, 50, 0, 0, time.UTC)},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection := fixture.selection
			selection.AcceptedAt = test.accepted
			got, err := fixture.resolver.Resolve(fixture.ctx, selection, fixture.facts)
			if !errors.Is(err, biz.ErrInvalidAdmission) || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
				t.Fatalf("invalid time was rounded, replaced or accepted: %v", err)
			}
		})
	}
	requireNoResolvedExecution(t, fixture)
}

func TestResolveAdmissionCancellationNeverReturnsCandidate(t *testing.T) {
	fixture := prepareResolution(t)
	canceled, cancel := context.WithCancel(fixture.ctx)
	cancel()
	expired, stopExpired := context.WithDeadline(fixture.ctx, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	defer stopExpired()
	for _, test := range []struct {
		name string
		ctx context.Context
		want error
	}{
		{"already canceled", canceled, context.Canceled},
		{"already expired", expired, context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := fixture.resolver.Resolve(test.ctx, fixture.selection, fixture.facts)
			if !errors.Is(err, test.want) || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
				t.Fatalf("stopped context returned a candidate or lost its cause: %v", err)
			}
		})
	}
	t.Run("canceled after actual Release read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(fixture.ctx)
		defer cancel()
		reader := cancelAfterReleaseRead{delegate: fixture.releaseReader, cancel: cancel}
		resolver := biz.NewAdmissionResolver(reader, fixture.inputReader)
		got, err := resolver.Resolve(ctx, fixture.selection, fixture.facts)
		if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
			t.Fatalf("cancellation between real reads was hidden as an infrastructure failure: %v", err)
		}
	})
	requireNoResolvedExecution(t, fixture)
}

// The actual file reader still supplies the document. Only cancellation timing
// is controlled, without sleep or replacing a dependency with fake success.
type cancelAfterReleaseRead struct {
	delegate biz.AdmissionReleaseReader
	cancel context.CancelFunc
}

func (reader cancelAfterReleaseRead) ReadRelease(ctx context.Context, id, digest string) (cpup01.ReleaseDocument, error) {
	document, err := reader.delegate.ReadRelease(ctx, id, digest)
	if err == nil {
		reader.cancel()
	}
	return document, err
}

func TestResolveAdmissionHidesUnavailableCataloguePath(t *testing.T) {
	fixture := prepareResolution(t)
	directory := filepath.Join(t.TempDir(), "unavailable-managed-reference")
	resolver := biz.NewAdmissionResolver(catalogue.NewReader(directory), fixture.inputReader)
	got, err := resolver.Resolve(fixture.ctx, fixture.selection, fixture.facts)
	if err != biz.ErrPersistence || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
		t.Fatalf("unavailable catalogue did not return its finite safe error: %v", err)
	}
	requireNoResolvedExecution(t, fixture)
}
