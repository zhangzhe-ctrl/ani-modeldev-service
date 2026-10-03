package objectstore

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

type DownloadSigner struct {
	client *s3.Client
	connectionID string
}

var _ biz.ArtifactSigner = (*DownloadSigner)(nil)

func NewDownloadSigner(client *s3.Client, connectionID string) *DownloadSigner {
	return &DownloadSigner{client: client, connectionID: connectionID}
}

func (s *DownloadSigner) SignDownload(context.Context, cpup01.StorageScope, cpup01.FixedObjectRef) (biz.DownloadGrant, error) {
	return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
}
