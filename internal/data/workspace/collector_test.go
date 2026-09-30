package workspace_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/workspace"
)

func TestCollectHashesActualRegisteredBytesWithoutTrustingCandidate(t *testing.T) {
	directory, execution, contents := preparedWorkspace(t)
	// Deliberately forged candidate metadata cannot supply a publication or
	// override hashes. These small byte fixtures test collection, not PyTorch.
	writeFile(t, filepath.Join(directory, "result.json"), []byte(`{"storage_state":"PUBLISHED","sha256":"forged"}`))
	got, err := workspace.Collect(context.Background(), directory, execution)
	if err != nil {
		t.Fatalf("collect valid workspace: %v", err)
	}
	if got.ExecutionID != execution.ExecutionID || got.ExecutionSpecHash != execution.SpecHash || got.StorageState != "WORKSPACE_ONLY" {
		t.Fatalf("collector lost the execution binding or claimed publication: %+v", got)
	}
	want := make([]biz.CollectedFile, 0, len(contents))
	for _, required := range execution.Snapshot.OutputContract.RequiredFiles {
		data := contents[required.RelativePath]
		digest := sha256.Sum256(data)
		want = append(want, biz.CollectedFile{RelativePath: required.RelativePath, Role: required.Role, SizeBytes: int64(len(data)), SHA256: hex.EncodeToString(digest[:])})
	}
	sort.Slice(want, func(i, j int) bool { return want[i].RelativePath < want[j].RelativePath })
	if !reflect.DeepEqual(got.Files, want) {
		t.Fatalf("actual byte inventory mismatch: got=%+v want=%+v", got.Files, want)
	}
	var manifest struct {
		ExecutionID       string              `json:"execution_id"`
		ExecutionSpecHash string              `json:"execution_spec_hash"`
		StorageState      string              `json:"storage_state"`
		Files             []biz.CollectedFile `json:"files"`
	}
	if err := json.Unmarshal(got.Manifest, &manifest); err != nil {
		t.Fatalf("collector must return a usable manifest: %v", err)
	}
	if manifest.ExecutionID != execution.ExecutionID || manifest.ExecutionSpecHash != execution.SpecHash || manifest.StorageState != "WORKSPACE_ONLY" || !reflect.DeepEqual(manifest.Files, want) {
		t.Fatal("collector manifest did not bind the bytes it actually read")
	}
	manifestDigest := sha256.Sum256(got.Manifest)
	if got.ManifestSHA256 != hex.EncodeToString(manifestDigest[:]) {
		t.Fatal("collector manifest SHA mismatch")
	}
}

func TestCollectRejectsMissingUnsafeOrUnboundedOutput(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, string, *biz.Execution)
	}{
		{"missing checkpoint", func(t *testing.T, directory string, _ *biz.Execution) {
			mustRemove(t, filepath.Join(directory, "model.pt"))
		}},
		{"empty checkpoint", func(t *testing.T, directory string, _ *biz.Execution) {
			writeFile(t, filepath.Join(directory, "model.pt"), nil)
		}},
		{"directory instead of checkpoint", func(t *testing.T, directory string, _ *biz.Execution) {
			path := filepath.Join(directory, "model.pt")
			mustRemove(t, path)
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink checkpoint", func(t *testing.T, directory string, _ *biz.Execution) {
			path := filepath.Join(directory, "model.pt")
			mustRemove(t, path)
			outside := filepath.Join(t.TempDir(), "outside.pt")
			writeFile(t, outside, []byte("outside execution"))
			if err := os.Symlink(outside, path); err != nil {
				t.Fatal(err)
			}
		}},
		{"hardlink checkpoint", func(t *testing.T, directory string, _ *biz.Execution) {
			if err := os.Link(filepath.Join(directory, "model.pt"), filepath.Join(t.TempDir(), "alias.pt")); err != nil {
				t.Fatal(err)
			}
		}},
		{"per file limit", func(t *testing.T, _ string, execution *biz.Execution) {
			for i := range execution.Snapshot.OutputContract.RequiredFiles {
				execution.Snapshot.OutputContract.RequiredFiles[i].MaxSizeBytes = 1
			}
			refreshSpec(t, execution)
		}},
		{"total byte limit", func(t *testing.T, directory string, execution *biz.Execution) {
			execution.Snapshot.OutputContract.MaxTotalBytes = 32
			for i := range execution.Snapshot.OutputContract.RequiredFiles {
				execution.Snapshot.OutputContract.RequiredFiles[i].MaxSizeBytes = 32
				writeFile(t, filepath.Join(directory, execution.Snapshot.OutputContract.RequiredFiles[i].RelativePath), []byte(strings.Repeat("x", 16)))
			}
			refreshSpec(t, execution)
		}},
		{"tampered frozen hash", func(_ *testing.T, _ string, execution *biz.Execution) { execution.SpecHash = strings.Repeat("0", 64) }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			directory, execution, _ := preparedWorkspace(t)
			testCase.change(t, directory, &execution)
			got, err := workspace.Collect(context.Background(), directory, execution)
			if !errors.Is(err, biz.ErrInvalidWorkspaceOutput) || !reflect.DeepEqual(got, biz.CollectedOutput{}) {
				t.Fatalf("unsafe output must fail without partial file inventory: got=%+v err=%v", got, err)
			}
		})
	}
}

func TestCollectRejectsSymlinkDirectoryAndCancelledContext(t *testing.T) {
	directory, execution, _ := preparedWorkspace(t)
	alias := filepath.Join(t.TempDir(), "training")
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatal(err)
	}
	if got, err := workspace.Collect(context.Background(), alias, execution); !errors.Is(err, biz.ErrInvalidWorkspaceOutput) || len(got.Files) != 0 {
		t.Fatalf("symlink root: %+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := workspace.Collect(ctx, directory, execution); !errors.Is(err, context.Canceled) || len(got.Files) != 0 {
		t.Fatalf("cancelled read: %+v %v", got, err)
	}
}

func preparedWorkspace(t *testing.T) (string, biz.Execution, map[string][]byte) {
	t.Helper()
	snapshot := conformance.SnapshotV1()
	intent := cpup01.Intent{Name: "collector-actual-bytes", Kind: "GENERAL_TRAINING", PresetID: snapshot.Release.PresetID, DatasetVersionID: snapshot.Input.InputVersionID}
	_, intentHash, err := cpup01.CanonicalIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	execution := biz.Execution{Admission: biz.Admission{
		TenantID: "11111111-2222-4333-8444-555555555555", Actor: "governance:user:42",
		OperationID: "bbbbbbbb-cccc-4ddd-8eee-ffffffffffff", ExecutionID: "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
		Intent: intent, IntentHash: intentHash, Snapshot: snapshot, AcceptedAt: snapshot.DeadlineAt.Add(-time.Hour),
	}}
	refreshSpec(t, &execution)
	directory := t.TempDir()
	contents := map[string][]byte{"model.pt": []byte("checkpoint bytes\x00\xff"), "model_config.json": []byte(`{"architecture":"MLP"}`), "metrics.jsonl": []byte("{\"step\":48}\n"), "summary.json": []byte(`{"steps":48}`)}
	for name, data := range contents {
		writeFile(t, filepath.Join(directory, name), data)
	}
	return directory, execution, contents
}

func refreshSpec(t *testing.T, execution *biz.Execution) {
	t.Helper()
	var err error
	execution.SpecHash, err = execution.Snapshot.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := execution.CanonicalPayloads(); err != nil {
		t.Fatalf("invalid admission fixture: %v", err)
	}
}

func writeFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func mustRemove(t *testing.T, name string) {
	t.Helper()
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
}
