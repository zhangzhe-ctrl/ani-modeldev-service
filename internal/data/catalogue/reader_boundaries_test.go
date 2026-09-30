package catalogue_test

import (
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

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/catalogue"
)

func TestParseReleaseBindsTheIndependentCanonicalFixtureDigest(t *testing.T) {
	// Independently computed with Fedora Python hashlib from fixed f16bcdeb.
	const digest = "388c7687614c92a2b50a14c8ee29df04f4d8934cf1dbf88d7b31258efc9f3dae"
	document, gotDigest, err := cpup01.ParseRelease([]byte(releaseFixtureV1))
	if err != nil || gotDigest != digest || !reflect.DeepEqual(document, releaseFixtureDocument()) {
		t.Fatalf("canonical Release or actual-byte digest changed: digest %q, err %v", gotDigest, err)
	}
}

func TestReleaseRejectsAmbiguousIncompleteAndUnsupportedDocuments(t *testing.T) {
	mutate := func(change func(*cpup01.ReleaseDocument)) []byte {
		document := releaseFixtureDocument()
		change(&document)
		raw, err := json.Marshal(document)
		if err != nil {
			t.Fatalf("fixture encoding failed; behavior NOT_RUN: %v", err)
		}
		return raw
	}
	cases := []struct {
		name string
		raw  []byte
	}{
		{"unknown field", []byte(strings.Replace(releaseFixtureV1, `"kind":`, `"ready":true,"kind":`, 1))},
		{"default pointer", []byte(strings.Replace(releaseFixtureV1, `"kind":`, `"current":true,"kind":`, 1))},
		{"duplicate field", []byte(strings.Replace(releaseFixtureV1, `"kind":`, `"kind":"GENERAL_TRAINING","kind":`, 1))},
		{"duplicate nested field", []byte(strings.Replace(releaseFixtureV1, `"nodes":1`, `"nodes":1,"nodes":1`, 1))},
		{"case folded field", []byte(strings.Replace(releaseFixtureV1, `"kind":`, `"Kind":`, 1))},
		{"null optional argument", []byte(strings.Replace(releaseFixtureV1, `{"literal":"--data"}`, `{"literal":"--data","source":null}`, 1))},
		{"omitted false", []byte(strings.Replace(releaseFixtureV1, `,"create_tar_bundle":false`, "", 1))},
		{"noncanonical integer", []byte(strings.Replace(releaseFixtureV1, `"1000"`, `"01000"`, 1))},
		{"noncanonical parameter", []byte(strings.Replace(releaseFixtureV1, `"0.01"`, `"0.0100"`, 1))},
		{"trailing newline", []byte(releaseFixtureV1 + "\n")},
		{"trailing object", []byte(releaseFixtureV1 + "{}")},
		{"leading whitespace", []byte(" " + releaseFixtureV1)},
		{"UTF8 BOM", append([]byte{0xef, 0xbb, 0xbf}, []byte(releaseFixtureV1)...)},
		{"invalid UTF8", []byte(strings.Replace(releaseFixtureV1, "cpu-module-fixture", "cpu-\xff", 1))},
		{"unpaired UTF16 escape", []byte(strings.Replace(releaseFixtureV1, "cpu-module-fixture", `cpu-\ud800`, 1))},
		{"unknown PipelineVersion", mutate(func(d *cpup01.ReleaseDocument) { d.PipelineVersionID = "UNKNOWN" })},
		{"zero release ID", mutate(func(d *cpup01.ReleaseDocument) { d.ReleaseID = "00000000-0000-0000-0000-000000000000" })},
		{"uppercase UUID", mutate(func(d *cpup01.ReleaseDocument) { d.PipelineVersionID = "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA" })},
		{"missing IR digest", mutate(func(d *cpup01.ReleaseDocument) { d.PipelineIRSHA256 = "" })},
		{"missing Runtime digest", mutate(func(d *cpup01.ReleaseDocument) { d.Runtime.ContentSHA256 = "" })},
		{"duplicate Runtime targets", mutate(func(d *cpup01.ReleaseDocument) { d.Runtime.TargetJobs = []string{"trainer", "trainer"} })},
		{"unordered Runtime targets", mutate(func(d *cpup01.ReleaseDocument) { d.Runtime.TargetJobs = []string{"worker", "trainer"} })},
		{"floating image", mutate(func(d *cpup01.ReleaseDocument) { d.Program.ImageDigest = "registry.example.test/cpu-mlp:latest" })},
		{"shell command", mutate(func(d *cpup01.ReleaseDocument) { d.Program.Command = []string{"sh", "-c", "train"} })},
		{"arbitrary argument source", mutate(func(d *cpup01.ReleaseDocument) { d.Program.ArgsTemplate[1].Source = "USER_SCRIPT" })},
		{"mixed argument forms", mutate(func(d *cpup01.ReleaseDocument) { d.Program.ArgsTemplate[1].Literal = "/arbitrary" })},
		{"unsupported parameter schema", mutate(func(d *cpup01.ReleaseDocument) { d.Program.ParameterContractVersion = "unknown" })},
		{"missing default parameter", mutate(func(d *cpup01.ReleaseDocument) { d.Program.DefaultParameters = d.Program.DefaultParameters[:2] })},
		{"duplicate default parameter", mutate(func(d *cpup01.ReleaseDocument) { d.Program.DefaultParameters[1] = d.Program.DefaultParameters[0] })},
		{"learning rate outside contract", mutate(func(d *cpup01.ReleaseDocument) { d.Program.DefaultParameters[2].Value = "0.11" })},
		{"multiple nodes", mutate(func(d *cpup01.ReleaseDocument) { d.Resources.Nodes = 2 })},
		{"CPU limit below request", mutate(func(d *cpup01.ReleaseDocument) { d.Resources.LimitMillicpu = 999 })},
		{"unbound storage class", mutate(func(d *cpup01.ReleaseDocument) { d.Workspace.StorageClass = "NOT_READY" })},
		{"overlapping workspace", mutate(func(d *cpup01.ReleaseDocument) { d.Workspace.TrainingSubpath = "input/training" })},
		{"wrong output role", mutate(func(d *cpup01.ReleaseDocument) { d.OutputContract.RequiredFiles[0].Role = "CHECKPOINT" })},
		{"unbounded timeout", mutate(func(d *cpup01.ReleaseDocument) { d.ExecutionTimeoutSeconds = 0 })},
		{"oversize otherwise canonical document", mutate(func(d *cpup01.ReleaseDocument) {
			d.Runtime.TargetJobs = make([]string, 10000)
			for i := range d.Runtime.TargetJobs {
				d.Runtime.TargetJobs[i] = fmt.Sprintf("job-%05d", i)
			}
		})},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			document, digest, err := cpup01.ParseRelease(test.raw)
			if !errors.Is(err, cpup01.ErrInvalidArgument) || digest != "" || !reflect.DeepEqual(document, cpup01.ReleaseDocument{}) {
				t.Fatalf("invalid document acquired a usable contract: digest %q, err %v", digest, err)
			}
			directory := t.TempDir()
			id := releaseFixtureDocument().ReleaseID
			if err := os.WriteFile(filepath.Join(directory, id+".json"), test.raw, 0600); err != nil {
				t.Fatal(err)
			}
			actualHash := sha256.Sum256(test.raw)
			loaded, err := catalogue.ReadRelease(context.Background(), directory, id, hex.EncodeToString(actualHash[:]))
			if !errors.Is(err, catalogue.ErrInvalidRelease) || !reflect.DeepEqual(loaded, cpup01.ReleaseDocument{}) {
				t.Fatalf("reader accepted invalid bytes despite a matching actual digest: %v", err)
			}
		})
	}
}

func TestReadReleaseRejectsChangedBytesAndWrongDocumentIdentity(t *testing.T) {
	id := releaseFixtureDocument().ReleaseID
	directory := t.TempDir()
	name := filepath.Join(directory, id+".json")
	originalHash := sha256.Sum256([]byte(releaseFixtureV1))
	for _, test := range []struct {
		name            string
		raw             string
		useActualDigest bool
	}{
		{"same ID changed content", strings.Replace(releaseFixtureV1, "55555555-5555-4555-8555-555555555555", "77777777-7777-4777-8777-777777777777", 1), false},
		{"different document ID", strings.Replace(releaseFixtureV1, id, "22222222-2222-4222-8222-222222222222", 1), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(name, []byte(test.raw), 0600); err != nil {
				t.Fatal(err)
			}
			digest := originalHash
			if test.useActualDigest {
				digest = sha256.Sum256([]byte(test.raw))
			}
			document, err := catalogue.ReadRelease(context.Background(), directory, id, hex.EncodeToString(digest[:]))
			if !errors.Is(err, catalogue.ErrInvalidRelease) || !reflect.DeepEqual(document, cpup01.ReleaseDocument{}) {
				t.Fatalf("changed Release was accepted: %v", err)
			}
		})
	}
}

func TestReadReleaseRejectsUnsafeFilesAndInvalidSelection(t *testing.T) {
	id := releaseFixtureDocument().ReleaseID
	digest := sha256.Sum256([]byte(releaseFixtureV1))
	digestText := hex.EncodeToString(digest[:])
	for _, kind := range []string{"symlink", "hardlink", "directory", "fifo", "empty", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			name := filepath.Join(directory, id+".json")
			switch kind {
			case "symlink", "hardlink":
				target := filepath.Join(t.TempDir(), "protected.json")
				if err := os.WriteFile(target, []byte(releaseFixtureV1), 0600); err != nil {
					t.Fatal(err)
				}
				var err error
				if kind == "symlink" {
					err = os.Symlink(target, name)
				} else {
					err = os.Link(target, name)
				}
				if err != nil {
					t.Fatalf("filesystem fixture failed; behavior NOT_RUN: %v", err)
				}
			case "directory":
				if err := os.Mkdir(name, 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := syscall.Mkfifo(name, 0600); err != nil {
					t.Fatalf("FIFO fixture failed; behavior NOT_RUN: %v", err)
				}
			case "empty", "oversize":
				var raw []byte
				if kind == "oversize" {
					raw = []byte(strings.Repeat("x", cpup01.MaxReleaseBytes+1))
				}
				if err := os.WriteFile(name, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			document, err := catalogue.ReadRelease(context.Background(), directory, id, digestText)
			if err == nil || !reflect.DeepEqual(document, cpup01.ReleaseDocument{}) {
				t.Fatalf("unsafe %s returned a usable Release", kind)
			}
		})
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, id+".json"), []byte(releaseFixtureV1), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, directory, id, digest string }{
		{"relative directory", "relative", id, digestText},
		{"unclean directory", directory + "/../" + filepath.Base(directory), id, digestText},
		{"path traversal", directory, "../" + id, digestText},
		{"URN UUID", directory, "urn:uuid:" + id, digestText},
		{"zero UUID", directory, "00000000-0000-0000-0000-000000000000", digestText},
		{"empty digest", directory, id, ""},
		{"uppercase digest", directory, id, strings.ToUpper(digestText)},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := catalogue.ReadRelease(context.Background(), test.directory, test.id, test.digest)
			if !errors.Is(err, cpup01.ErrInvalidArgument) || !reflect.DeepEqual(document, cpup01.ReleaseDocument{}) {
				t.Fatalf("invalid selection returned a Release: %v", err)
			}
		})
	}
	missing := "22222222-2222-4222-8222-222222222222"
	if _, err := catalogue.ReadRelease(context.Background(), directory, missing, digestText); !errors.Is(err, catalogue.ErrReleaseNotFound) {
		t.Fatalf("missing explicit version did not report not found: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := catalogue.ReadRelease(ctx, directory, id, digestText); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was lost: %v", err)
	}
}
