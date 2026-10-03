package execution

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

var _ biz.ArtifactQueryRepository = (*Repository)(nil)

func (r *Repository) GetQueryRecord(context.Context, string, string) (biz.QueryRecord, error) {
	return biz.QueryRecord{}, biz.ErrDownloadUnavailable
}

func (r *Repository) FindPublishedArtifact(context.Context, string, string) (biz.QueryRecord, biz.PublishedRuntimeFile, error) {
	return biz.QueryRecord{}, biz.PublishedRuntimeFile{}, biz.ErrArtifactNotFound
}
