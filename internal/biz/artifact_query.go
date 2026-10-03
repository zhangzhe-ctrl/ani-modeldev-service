package biz

import (
	"context"
	"errors"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var (
	ErrArtifactNotFound = errors.New("ARTIFACT_NOT_FOUND")
	ErrDownloadUnavailable = errors.New("DOWNLOAD_UNAVAILABLE")
)

type QueryRecord struct {
	Execution Execution
	Runtime ExecutionRuntime
}

type ArtifactQueryRepository interface {
	GetQueryRecord(context.Context, string, string) (QueryRecord, error)
	FindPublishedArtifact(context.Context, string, string) (QueryRecord, PublishedRuntimeFile, error)
}

type ArtifactSigner interface {
	SignDownload(context.Context, cpup01.StorageScope, cpup01.FixedObjectRef) (DownloadGrant, error)
}

type DownloadGrant struct {
	URL string
	ExpiresAt time.Time
}
