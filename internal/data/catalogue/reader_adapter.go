package catalogue

import (
	"context"
	"errors"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// Reader keeps the managed catalogue directory out of admission requests.
// It delegates all fixed-ID, byte-digest and filesystem checks to ReadRelease.
type Reader struct {
	directory string
}

func NewReader(trustedDirectory string) *Reader {
	return &Reader{directory: trustedDirectory}
}

func (reader *Reader) ReadRelease(ctx context.Context, releaseID, expectedDigest string) (cpup01.ReleaseDocument, error) {
	document, err := ReadRelease(ctx, reader.directory, releaseID, expectedDigest)
	switch {
	case err == nil:
		return document, nil
	case errors.Is(err, context.Canceled):
		return cpup01.ReleaseDocument{}, context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return cpup01.ReleaseDocument{}, context.DeadlineExceeded
	case errors.Is(err, cpup01.ErrInvalidArgument):
		return cpup01.ReleaseDocument{}, biz.ErrInvalidAdmission
	case errors.Is(err, ErrReleaseNotFound), errors.Is(err, ErrInvalidRelease):
		return cpup01.ReleaseDocument{}, biz.ErrNoCompatibleRelease
	default:
		return cpup01.ReleaseDocument{}, biz.ErrPersistence
	}
}
