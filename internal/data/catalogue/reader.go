// Package catalogue reads explicitly selected immutable CPU-P01 Releases.
package catalogue

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var (
	ErrReleaseNotFound      = errors.New("RELEASE_NOT_FOUND")
	ErrInvalidRelease       = errors.New("INVALID_RELEASE")
	ErrCatalogueUnavailable = errors.New("CATALOGUE_UNAVAILABLE")
)

// ReadRelease reads only <releaseID>.json below a trusted configured directory.
// The caller supplies both the ID and digest selected by Governance. This
// adapter never chooses a current Release and never enables a loaded document.
func ReadRelease(ctx context.Context, directory, releaseID, expectedDigest string) (cpup01.ReleaseDocument, error) {
	if err := ctx.Err(); err != nil {
		return cpup01.ReleaseDocument{}, err
	}
	id, err := uuid.Parse(releaseID)
	if err != nil || id == uuid.Nil || id.String() != releaseID || len(expectedDigest) != 64 || strings.Trim(expectedDigest, "0123456789abcdef") != "" {
		return cpup01.ReleaseDocument{}, cpup01.ErrInvalidArgument
	}
	root, err := openCatalogue(ctx, directory)
	if err != nil {
		return cpup01.ReleaseDocument{}, err
	}
	defer syscall.Close(root)
	return readReleaseAt(ctx, root, releaseID, expectedDigest)
}

// openCatalogue applies the same directory boundary to startup and each read.
// It neither enumerates Releases nor retains a descriptor between requests.
func openCatalogue(ctx context.Context, directory string) (int, error) {
	if err := ctx.Err(); err != nil {
		return -1, err
	}
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return -1, cpup01.ErrInvalidArgument
	}
	root, err := syscall.Open(directory, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, ErrCatalogueUnavailable
	}
	return root, nil
}

// readReleaseAt keeps import replay validation on the same trusted directory
// descriptor used for the atomic install, even if its pathname is replaced.
func readReleaseAt(ctx context.Context, root int, releaseID, expectedDigest string) (cpup01.ReleaseDocument, error) {
	if err := ctx.Err(); err != nil {
		return cpup01.ReleaseDocument{}, err
	}
	name := releaseID + ".json"
	fd, err := syscall.Openat(root, name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return cpup01.ReleaseDocument{}, ErrReleaseNotFound
		}
		return cpup01.ReleaseDocument{}, ErrCatalogueUnavailable
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	var before syscall.Stat_t
	if syscall.Fstat(fd, &before) != nil || before.Mode&syscall.S_IFMT != syscall.S_IFREG || before.Nlink != 1 || before.Size <= 0 || before.Size > cpup01.MaxReleaseBytes {
		return cpup01.ReleaseDocument{}, ErrInvalidRelease
	}
	if err := ctx.Err(); err != nil {
		return cpup01.ReleaseDocument{}, err
	}
	raw, err := io.ReadAll(io.LimitReader(file, cpup01.MaxReleaseBytes+1))
	if err := ctx.Err(); err != nil {
		return cpup01.ReleaseDocument{}, err
	}
	if err != nil {
		return cpup01.ReleaseDocument{}, ErrCatalogueUnavailable
	}
	var after syscall.Stat_t
	if syscall.Fstat(fd, &after) != nil || int64(len(raw)) != before.Size || after.Size != before.Size || after.Nlink != 1 || after.Mtim != before.Mtim || after.Ctim != before.Ctim {
		return cpup01.ReleaseDocument{}, ErrInvalidRelease
	}
	document, digest, err := cpup01.ParseRelease(raw)
	if err != nil || digest != expectedDigest || document.ReleaseID != releaseID {
		return cpup01.ReleaseDocument{}, ErrInvalidRelease
	}
	return document, nil
}
