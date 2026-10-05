package catalogue

import (
	"context"
	"errors"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

func (reader *Reader) ImportRelease(ctx context.Context, raw []byte, digest string) (cpup01.ReleaseDocument, bool, error) {
	result, err := ImportRelease(ctx, reader.directory, raw, digest)
	if errors.Is(err, ErrReleaseConflict) {
		return cpup01.ReleaseDocument{}, false, biz.ErrAdmissionConflict
	}
	if err != nil {
		return cpup01.ReleaseDocument{}, false, biz.ErrPersistence
	}
	release, err := reader.ReadRelease(ctx, result.ReleaseID, digest)
	return release, result.Created, err
}
