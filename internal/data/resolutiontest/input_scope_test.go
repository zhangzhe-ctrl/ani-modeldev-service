package resolutiontest

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
)

func TestResolveAdmissionScopesIdenticalInputIDsByTrustedTenant(t *testing.T) {
	fixture := prepareResolution(t)
	otherSelection, otherFacts := fixture.selection, fixture.facts
	otherSelection.TenantID = "99999999-9999-4999-8999-999999999999"
	otherFacts.TenantID = otherSelection.TenantID
	otherFacts.InputScope.ApprovedPrefix = "other/input"
	otherFacts.PublicationScope.ApprovedPrefix = "other/artifacts"
	otherFacts.Environment.NamespaceName = "module-fixture-other"
	otherFacts.Environment.Identities.TenantProxyIdentity = "fixture:other"

	got, err := fixture.resolver.Resolve(fixture.ctx, otherSelection, otherFacts)
	if !errors.Is(err, biz.ErrInputNotFound) || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
		t.Fatalf("another tenant's READY row became visible: %v", err)
	}
	got, err = fixture.resolver.Resolve(fixture.ctx, otherSelection, fixture.facts)
	if !errors.Is(err, biz.ErrAdmissionEnvironmentNotReady) || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
		t.Fatalf("facts from another tenant were accepted: %v", err)
	}

	// The same input ID in tenant B denotes its own immutable object. Both
	// records are real PG rows, so omitting tenant from either read is observable.
	otherImport := fixture.imported
	otherImport.TenantID = otherSelection.TenantID
	otherImport.RequestID = "88888888-8888-4888-8888-888888888888"
	otherImport.Scope = otherFacts.InputScope
	otherImport.Object.Key = "other/input/data.csv"
	otherVersion := "synthetic-other-tenant-version"
	otherImport.Object.VersionID = &otherVersion
	otherImport.Object.SHA256 = strings.Repeat("e", 64)
	repository := input.New(fixture.openPool())
	if _, err := repository.FreezeImport(fixture.ctx, otherImport); err != nil {
		t.Fatalf("RESOLUTION_PREFLIGHT: tenant B import failed; behavior NOT_RUN: %v", err)
	}
	proof := biz.VerifiedCSV{
		VerifiedObject: biz.VerifiedObject{Object: otherImport.Object, VerifiedAt: otherImport.RequestedAt.Add(time.Minute)},
		SchemaVersion: "ani.cpu.csv.v1", RowCount: 1024, FeatureCount: 16,
	}
	otherReady, err := repository.RecordVerifiedCSV(fixture.ctx, otherImport, proof)
	if err != nil {
		t.Fatalf("RESOLUTION_PREFLIGHT: tenant B READY failed; behavior NOT_RUN: %v", err)
	}
	other, err := fixture.resolver.Resolve(fixture.ctx, otherSelection, otherFacts)
	if err != nil || !reflect.DeepEqual(other.Snapshot.Input.Object, otherImport.Object) {
		t.Fatalf("tenant B did not resolve its own immutable object: %v", err)
	}
	original, err := fixture.resolver.Resolve(fixture.ctx, fixture.selection, fixture.facts)
	if err != nil || !reflect.DeepEqual(original.Snapshot.Input.Object, fixture.imported.Object) || original.SpecHash == other.SpecHash {
		t.Fatalf("tenant A resolution mixed in tenant B facts: %v", err)
	}
	for _, want := range []biz.InputVersion{fixture.ready, otherReady} {
		stored, err := input.New(fixture.openPool()).Get(fixture.ctx, want.Import.TenantID, want.Import.InputVersionID)
		if err != nil || !reflect.DeepEqual(stored, want) {
			t.Fatalf("resolution rewrote one tenant's input: %v", err)
		}
	}
	requireNoResolvedExecution(t, fixture)
}

func TestResolveAdmissionRejectsDurableInputsWithoutReadyProof(t *testing.T) {
	for _, test := range []struct {
		name string
		failure biz.InputFailureCode
		state biz.InputState
	}{
		{"validating", "", biz.InputStateValidating},
		{"source unavailable", biz.InputFailureSourceUnavailable, biz.InputStateValidating},
		{"content rejected", biz.InputFailureContentRejected, biz.InputStateRejected},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := prepareResolution(t)
			request := fixture.imported
			request.InputVersionID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
			request.RequestID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
			request.Object.Key = "tenant/input/not-ready.csv"
			repository := input.New(fixture.openPool())
			stored, err := repository.FreezeImport(fixture.ctx, request)
			if err != nil {
				t.Fatalf("RESOLUTION_PREFLIGHT: not-READY import failed; behavior NOT_RUN: %v", err)
			}
			if test.failure != "" {
				stored, err = repository.RecordValidationFailure(fixture.ctx, request, biz.InputValidationFailure{Code: test.failure, ObservedAt: request.RequestedAt.Add(time.Minute)})
				if err != nil {
					t.Fatalf("RESOLUTION_PREFLIGHT: not-READY state failed; behavior NOT_RUN: %v", err)
				}
			}
			if stored.State != test.state || stored.Verification != nil {
				t.Fatal("RESOLUTION_PREFLIGHT: not-READY fixture has wrong durable state; behavior NOT_RUN")
			}
			selection := fixture.selection
			selection.Intent.DatasetVersionID = request.InputVersionID
			got, err := fixture.resolver.Resolve(fixture.ctx, selection, fixture.facts)
			if !errors.Is(err, biz.ErrAdmissionInputNotReady) || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
				t.Fatalf("unverified input produced an execution candidate: %v", err)
			}
			again, err := input.New(fixture.openPool()).Get(fixture.ctx, request.TenantID, request.InputVersionID)
			if err != nil || !reflect.DeepEqual(again, stored) {
				t.Fatalf("rejected resolution changed the input state: %v", err)
			}
			requireNoResolvedExecution(t, fixture)
		})
	}
}

func requireNoResolvedExecution(t *testing.T, fixture resolutionFixture) {
	t.Helper()
	var count int
	if err := fixture.openPool().QueryRow(fixture.ctx, "SELECT count(*) FROM modeldev_executions").Scan(&count); err != nil || count != 0 {
		t.Fatalf("read-only resolution persisted an execution: count=%d, error=%v", count, err)
	}
}
