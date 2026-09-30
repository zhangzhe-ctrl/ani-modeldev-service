package resolutiontest

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/catalogue"
)

func TestResolveAdmissionPreservesIntentPresenceWhileResolvingEquivalentDefaults(t *testing.T) {
	fixture := prepareResolution(t)
	empty := []cpup01.Parameter{}
	defaults := []cpup01.Parameter{
		{Name: "learning_rate", Type: "DECIMAL", Value: "0.0100"},
		{Name: "epochs", Type: "INTEGER", Value: "3"},
		{Name: "batch_size", Type: "INTEGER", Value: "64"},
	}
	image := fixture.release.Program.ImageVersionID
	variants := []struct {
		name string
		parameters *[]cpup01.Parameter
		image *string
	}{
		{"omitted", nil, nil},
		{"empty", &empty, nil},
		{"explicit defaults", &defaults, nil},
		{"explicit image", nil, &image},
	}
	var original biz.AdmissionResolution
	hashes := make(map[string]bool)
	for index, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			selection := fixture.selection
			selection.Intent.GeneralParameters, selection.Intent.ImageVersionID = variant.parameters, variant.image
			before, intentHash, err := cpup01.CanonicalIntent(selection.Intent)
			if err != nil || hashes[intentHash] {
				t.Fatalf("presence fixture must have a distinct valid intent hash: %v", err)
			}
			hashes[intentHash] = true
			got, err := fixture.resolver.Resolve(fixture.ctx, selection, fixture.facts)
			if err != nil {
				t.Fatal(err)
			}
			if index == 0 {
				original = got
			} else if !reflect.DeepEqual(got, original) {
				t.Fatal("equivalent explicit values changed the resolved configuration")
			}
			want := []cpup01.Parameter{{Name: "batch_size", Type: "INTEGER", Value: "64"}, {Name: "epochs", Type: "INTEGER", Value: "3"}, {Name: "learning_rate", Type: "DECIMAL", Value: "0.01"}}
			if !reflect.DeepEqual(got.Snapshot.Program.ResolvedParameters, want) || got.Snapshot.Program.ResolvedArgs[9] != "0.01" {
				t.Fatal("Release defaults were not resolved into the registered program")
			}
			after, afterHash, err := cpup01.CanonicalIntent(selection.Intent)
			if err != nil || afterHash != intentHash || !bytes.Equal(before, after) || selection.Intent.GeneralParameters != variant.parameters || selection.Intent.ImageVersionID != variant.image {
				t.Fatal("resolution replaced the original intent or its presence")
			}
		})
	}
	if len(empty) != 0 || defaults[0].Value != "0.0100" || defaults[0].Name != "learning_rate" {
		t.Fatal("resolution rewrote caller-owned parameter values or order")
	}
	requireNoResolvedExecution(t, fixture)
}

func TestResolveAdmissionRejectsInvalidOrIncompatibleSelections(t *testing.T) {
	fixture := prepareResolution(t)
	for _, test := range []struct {
		name string
		change func(*biz.AdmissionResolutionRequest)
		want error
	}{
		{"missing release", func(r *biz.AdmissionResolutionRequest) { r.Release.ReleaseID = "77777777-7777-4777-8777-777777777777" }, biz.ErrNoCompatibleRelease},
		{"wrong digest", func(r *biz.AdmissionResolutionRequest) { r.Release.ReleaseDigest = strings.Repeat("0", 64) }, biz.ErrNoCompatibleRelease},
		{"malformed digest", func(r *biz.AdmissionResolutionRequest) { r.Release.ReleaseDigest = "invalid" }, biz.ErrInvalidAdmission},
		{"zero release", func(r *biz.AdmissionResolutionRequest) { r.Release.ReleaseID = "00000000-0000-0000-0000-000000000000" }, biz.ErrInvalidAdmission},
		{"zero generation", func(r *biz.AdmissionResolutionRequest) { r.Release.BindingGeneration = 0 }, biz.ErrInvalidAdmission},
		{"wrong preset", func(r *biz.AdmissionResolutionRequest) { r.Intent.PresetID = "77777777-7777-4777-8777-777777777777" }, biz.ErrNoCompatibleRelease},
		{"unsupported kind", func(r *biz.AdmissionResolutionRequest) { r.Intent.Kind = "FINETUNING" }, biz.ErrInvalidAdmission},
		{"different image", func(r *biz.AdmissionResolutionRequest) { id := "77777777-7777-4777-8777-777777777777"; r.Intent.ImageVersionID = &id }, biz.ErrNoCompatibleRelease},
		{"unknown parameter", func(r *biz.AdmissionResolutionRequest) { values := []cpup01.Parameter{{Name: "script", Type: "STRING", Value: "unregistered"}}; r.Intent.GeneralParameters = &values }, biz.ErrInvalidAdmission},
		{"duplicate parameter", func(r *biz.AdmissionResolutionRequest) { values := []cpup01.Parameter{{Name: "epochs", Type: "INTEGER", Value: "3"}, {Name: "epochs", Type: "INTEGER", Value: "3"}}; r.Intent.GeneralParameters = &values }, biz.ErrInvalidAdmission},
		{"wrong parameter type", func(r *biz.AdmissionResolutionRequest) { values := []cpup01.Parameter{{Name: "learning_rate", Type: "INTEGER", Value: "0.01"}}; r.Intent.GeneralParameters = &values }, biz.ErrInvalidAdmission},
		{"out of range parameter", func(r *biz.AdmissionResolutionRequest) { values := []cpup01.Parameter{{Name: "learning_rate", Type: "DECIMAL", Value: "0.11"}}; r.Intent.GeneralParameters = &values }, biz.ErrInvalidAdmission},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection := fixture.selection
			test.change(&selection)
			got, err := fixture.resolver.Resolve(fixture.ctx, selection, fixture.facts)
			if !errors.Is(err, test.want) || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
				t.Fatalf("invalid selection produced a candidate or wrong domain error: %v", err)
			}
		})
	}
	requireNoResolvedExecution(t, fixture)
}

func TestResolveAdmissionRetainsExplicitOldSelectionAfterNewReleaseImport(t *testing.T) {
	fixture := prepareResolution(t)
	directory := t.TempDir()
	if _, err := catalogue.ImportRelease(fixture.ctx, directory, conformance.ReleaseCanonicalV1(), conformance.ReleaseSHA256V1); err != nil {
		t.Fatalf("RESOLUTION_PREFLIGHT: original catalogue import failed; behavior NOT_RUN: %v", err)
	}
	resolver := biz.NewAdmissionResolver(catalogue.NewReader(directory), fixture.inputReader)
	selection := fixture.selection
	selection.Intent.GeneralParameters = nil
	original, err := resolver.Resolve(fixture.ctx, selection, fixture.facts)
	if err != nil {
		t.Fatal(err)
	}
	// Change explicit immutable fixture fields, keeping canonical field order.
	// This is a test Release, never a production material or enabled pointer.
	raw := string(conformance.ReleaseCanonicalV1())
	raw = strings.Replace(raw, `"release_id":"11111111-1111-4111-8111-111111111111"`, `"release_id":"77777777-7777-4777-8777-777777777777"`, 1)
	raw = strings.Replace(raw, `"image_version_id":"66666666-6666-4666-8666-666666666666"`, `"image_version_id":"88888888-8888-4888-8888-888888888888"`, 1)
	raw = strings.Replace(raw, `"value":"0.01"`, `"value":"0.03"`, 1)
	newRelease, digest, err := cpup01.ParseRelease([]byte(raw))
	if err != nil {
		t.Fatalf("RESOLUTION_PREFLIGHT: new Release fixture invalid; behavior NOT_RUN: %v", err)
	}
	if _, err := catalogue.ImportRelease(fixture.ctx, directory, []byte(raw), digest); err != nil {
		t.Fatalf("RESOLUTION_PREFLIGHT: new Release import failed; behavior NOT_RUN: %v", err)
	}
	again, err := resolver.Resolve(fixture.ctx, selection, fixture.facts)
	if err != nil || !reflect.DeepEqual(again, original) {
		t.Fatalf("adding a Release changed the explicit original selection: %v", err)
	}
	selection.Release = biz.AdmissionReleaseSelection{ReleaseID: newRelease.ReleaseID, ReleaseDigest: digest, BindingGeneration: 8}
	changed, err := resolver.Resolve(fixture.ctx, selection, fixture.facts)
	if err != nil || changed.Snapshot.Release.ReleaseID != newRelease.ReleaseID || changed.Snapshot.Release.ReleaseDigest != digest || changed.Snapshot.Release.AcceptedBindingGeneration != 8 || changed.Snapshot.Program.ImageVersionID != newRelease.Program.ImageVersionID || changed.Snapshot.Program.ResolvedArgs[9] != "0.03" || changed.SpecHash == original.SpecHash {
		t.Fatalf("explicit new selection did not bind the new immutable facts: %v", err)
	}
	oldImage := fixture.release.Program.ImageVersionID
	selection.Intent.ImageVersionID = &oldImage
	got, err := resolver.Resolve(fixture.ctx, selection, fixture.facts)
	if !errors.Is(err, biz.ErrNoCompatibleRelease) || !reflect.DeepEqual(got, biz.AdmissionResolution{}) {
		t.Fatalf("new Release accepted an explicitly incompatible old image: %v", err)
	}
	requireNoResolvedExecution(t, fixture)
}
