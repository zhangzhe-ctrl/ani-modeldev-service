package catalogue

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"golang.org/x/sys/unix"
)

var ErrReleaseConflict = errors.New("RELEASE_CONFLICT")

// ImportResult records a file installation, not an enabled or verified Release.
type ImportResult struct {
	ReleaseID string
	Digest    string
	Created   bool
}

// ImportRelease installs canonical bytes under their immutable Release ID.
// A trusted T02 caller must first authorize the operation and verify the real
// environment/material evidence. This adapter does not grant those facts.
func ImportRelease(ctx context.Context, directory string, raw []byte, expectedDigest string) (ImportResult, error) {
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}
	if len(raw) == 0 || len(raw) > cpup01.MaxReleaseBytes || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return ImportResult{}, cpup01.ErrInvalidArgument
	}
	// The bytes checked by the contract are the same owned bytes written below.
	raw = bytes.Clone(raw)
	document, digest, err := cpup01.ParseRelease(raw)
	if err != nil || digest != expectedDigest {
		return ImportResult{}, cpup01.ErrInvalidArgument
	}
	root, err := unix.Open(directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ImportResult{}, ErrCatalogueUnavailable
	}
	defer unix.Close(root)
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}
	temporaryName := ".release-" + rand.Text() + ".tmp"
	fd, err := unix.Openat(root, temporaryName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ImportResult{}, ErrCatalogueUnavailable
	}
	temporaryPresent := true
	defer func() {
		if temporaryPresent {
			_ = unix.Unlinkat(root, temporaryName, 0)
		}
	}()
	file := os.NewFile(uintptr(fd), temporaryName)
	defer file.Close()
	written, err := file.Write(raw)
	if err != nil || written != len(raw) {
		return ImportResult{}, ErrCatalogueUnavailable
	}
	if file.Chmod(0444) != nil || file.Sync() != nil || file.Close() != nil {
		return ImportResult{}, ErrCatalogueUnavailable
	}
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}
	// Never replace an existing ID, including on filesystems lacking this flag.
	if err := unix.Renameat2(root, temporaryName, root, document.ReleaseID+".json", unix.RENAME_NOREPLACE); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return ImportResult{}, ErrReleaseConflict
		}
		return ImportResult{}, ErrCatalogueUnavailable
	}
	temporaryPresent = false
	// A failure here leaves the installed ID intact. The caller must retain the
	// original ID/digest and reconcile by retrying, never delete or choose a new ID.
	if unix.Fsync(root) != nil {
		return ImportResult{}, ErrCatalogueUnavailable
	}
	if err := ctx.Err(); err != nil {
		return ImportResult{}, err
	}
	return ImportResult{ReleaseID: document.ReleaseID, Digest: digest, Created: true}, nil
}
