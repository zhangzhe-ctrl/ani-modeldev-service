package catalogue_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/catalogue"
)

func TestImportDifferentContentCannotReplaceAnExistingReleaseID(t *testing.T) {
	directory := t.TempDir()
	original, err := catalogue.ImportRelease(context.Background(), directory, conformance.ReleaseCanonicalV1(), conformance.ReleaseSHA256V1)
	if err != nil {
		t.Fatal(err)
	}
	changed := changedReleaseBytes()
	result, err := catalogue.ImportRelease(context.Background(), directory, changed, releaseBytesDigest(changed))
	if !errors.Is(err, catalogue.ErrReleaseConflict) || result != (catalogue.ImportResult{}) {
		t.Fatalf("same ID with different content was not rejected: %+v, %v", result, err)
	}
	retained, err := catalogue.ReadRelease(context.Background(), directory, original.ReleaseID, original.Digest)
	if err != nil || !reflect.DeepEqual(retained, releaseFixtureDocument()) {
		t.Fatalf("conflicting import replaced the original file: %+v, %v", retained, err)
	}
}

func TestConcurrentIdenticalImportsCreateOnlyOneFileAndReplayTheOthers(t *testing.T) {
	directory := t.TempDir()
	raw := conformance.ReleaseCanonicalV1()
	results := concurrentImports(directory, [][]byte{raw, raw, raw, raw, raw, raw})
	created := 0
	for _, result := range results {
		if result.err != nil || result.reply.ReleaseID != releaseFixtureDocument().ReleaseID || result.reply.Digest != conformance.ReleaseSHA256V1 {
			t.Fatalf("concurrent exact import failed: %+v, %v", result.reply, result.err)
		}
		if result.reply.Created {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created %d files for one immutable ID", created)
	}
	retained, err := catalogue.ReadRelease(context.Background(), directory, releaseFixtureDocument().ReleaseID, conformance.ReleaseSHA256V1)
	if err != nil || !reflect.DeepEqual(retained, releaseFixtureDocument()) {
		t.Fatalf("concurrent imports left no complete fixed file: %+v, %v", retained, err)
	}
}

func TestConcurrentConflictingImportsKeepExactlyOneImmutableWinner(t *testing.T) {
	directory := t.TempDir()
	original, changed := conformance.ReleaseCanonicalV1(), changedReleaseBytes()
	results := concurrentImports(directory, [][]byte{original, changed, original, changed})
	created, replayed, rejected := 0, 0, 0
	var winner catalogue.ImportResult
	for _, result := range results {
		if errors.Is(result.err, catalogue.ErrReleaseConflict) {
			if result.reply != (catalogue.ImportResult{}) {
				t.Fatal("conflicting import returned a usable receipt")
			}
			rejected++
			continue
		}
		if result.err != nil || result.reply.Digest != result.digest {
			t.Fatalf("unexpected concurrent import outcome: %+v, %v", result.reply, result.err)
		}
		if result.reply.Created {
			created++
			winner = result.reply
		} else {
			replayed++
		}
	}
	if created != 1 || replayed != 1 || rejected != 2 {
		t.Fatalf("invalid winner/replay/conflict counts: %d/%d/%d", created, replayed, rejected)
	}
	for _, result := range results {
		if result.err == nil && result.reply.Digest != winner.Digest {
			t.Fatal("two different contents were acknowledged under one ID")
		}
	}
	want := releaseFixtureDocument()
	if winner.Digest == releaseBytesDigest(changed) {
		want.PipelineVersionID = "77777777-7777-4777-8777-777777777777"
	}
	retained, err := catalogue.ReadRelease(context.Background(), directory, winner.ReleaseID, winner.Digest)
	if err != nil || !reflect.DeepEqual(retained, want) {
		t.Fatalf("winning content was not preserved: %+v, %v", retained, err)
	}
}

func TestInvalidOrCancelledImportLeavesNoRelease(t *testing.T) {
	for _, test := range []struct {
		name   string
		raw    []byte
		digest string
		cancel bool
	}{
		{"wrong expected digest", conformance.ReleaseCanonicalV1(), strings.Repeat("0", 64), false},
		{"noncanonical file", []byte(releaseFixtureV1 + "\n"), releaseBytesDigest([]byte(releaseFixtureV1 + "\n")), false},
		{"unknown version", []byte(strings.Replace(releaseFixtureV1, "55555555-5555-4555-8555-555555555555", "UNKNOWN", 1)), "", false},
		{"cancelled", conformance.ReleaseCanonicalV1(), conformance.ReleaseSHA256V1, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if test.cancel {
				cancel()
			}
			digest := test.digest
			if digest == "" {
				digest = releaseBytesDigest(test.raw)
			}
			result, err := catalogue.ImportRelease(ctx, directory, test.raw, digest)
			if err == nil || result != (catalogue.ImportResult{}) {
				t.Fatalf("invalid or cancelled import succeeded: %+v, %v", result, err)
			}
			if test.cancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation was lost: %v", err)
			}
			if _, err := catalogue.ReadRelease(context.Background(), directory, releaseFixtureDocument().ReleaseID, conformance.ReleaseSHA256V1); !errors.Is(err, catalogue.ErrReleaseNotFound) {
				t.Fatalf("rejected import left an installed Release: %v", err)
			}
		})
	}
}

func TestImportCannotReplayUnsafeOrCorruptedExistingFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "directory", "fifo", "corrupted"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			name := filepath.Join(directory, releaseFixtureDocument().ReleaseID+".json")
			var protected string
			switch kind {
			case "symlink", "hardlink":
				protected = filepath.Join(t.TempDir(), "protected.json")
				if err := os.WriteFile(protected, conformance.ReleaseCanonicalV1(), 0600); err != nil {
					t.Fatal(err)
				}
				var err error
				if kind == "symlink" {
					err = os.Symlink(protected, name)
				} else {
					err = os.Link(protected, name)
				}
				if err != nil {
					t.Fatalf("filesystem preflight failed; behavior NOT_RUN: %v", err)
				}
			case "directory":
				if err := os.Mkdir(name, 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := syscall.Mkfifo(name, 0600); err != nil {
					t.Fatalf("FIFO preflight failed; behavior NOT_RUN: %v", err)
				}
			case "corrupted":
				if err := os.WriteFile(name, []byte("corrupted original bytes\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			result, err := catalogue.ImportRelease(context.Background(), directory, conformance.ReleaseCanonicalV1(), conformance.ReleaseSHA256V1)
			if err == nil || result != (catalogue.ImportResult{}) {
				t.Fatalf("unsafe existing file was accepted as replay: %+v, %v", result, err)
			}
			if _, err := catalogue.ReadRelease(context.Background(), directory, releaseFixtureDocument().ReleaseID, conformance.ReleaseSHA256V1); err == nil {
				t.Fatal("unsafe existing file was replaced with an accepted Release")
			}
			if protected != "" {
				actual, err := os.ReadFile(protected)
				if err != nil || string(actual) != releaseFixtureV1 {
					t.Fatalf("protected link target changed: %v", err)
				}
			}
			if kind == "corrupted" {
				actual, err := os.ReadFile(name)
				if err != nil || string(actual) != "corrupted original bytes\n" {
					t.Fatalf("corrupted original was overwritten: %v", err)
				}
			}
		})
	}
}

func TestImportRejectsSymlinkCatalogueRoot(t *testing.T) {
	directory := t.TempDir()
	alias := filepath.Join(t.TempDir(), "catalogue")
	if err := os.Symlink(directory, alias); err != nil {
		t.Fatal(err)
	}
	result, err := catalogue.ImportRelease(context.Background(), alias, conformance.ReleaseCanonicalV1(), conformance.ReleaseSHA256V1)
	if err == nil || result != (catalogue.ImportResult{}) {
		t.Fatalf("symlink root accepted: %+v, %v", result, err)
	}
	if _, err := catalogue.ReadRelease(context.Background(), directory, releaseFixtureDocument().ReleaseID, conformance.ReleaseSHA256V1); !errors.Is(err, catalogue.ErrReleaseNotFound) {
		t.Fatalf("symlink root received a Release: %v", err)
	}
}

type importAttempt struct {
	reply  catalogue.ImportResult
	err    error
	digest string
}

func concurrentImports(directory string, variants [][]byte) []importAttempt {
	start := make(chan struct{})
	completed := make(chan importAttempt, len(variants))
	for _, raw := range variants {
		go func(raw []byte) {
			<-start
			digest := releaseBytesDigest(raw)
			reply, err := catalogue.ImportRelease(context.Background(), directory, raw, digest)
			completed <- importAttempt{reply: reply, err: err, digest: digest}
		}(raw)
	}
	close(start)
	results := make([]importAttempt, 0, len(variants))
	for range variants {
		results = append(results, <-completed)
	}
	return results
}

func changedReleaseBytes() []byte {
	return []byte(strings.Replace(releaseFixtureV1, "55555555-5555-4555-8555-555555555555", "77777777-7777-4777-8777-777777777777", 1))
}

func releaseBytesDigest(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
