package workspace

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// PrepareInput is called by the registered prepare component on its own mounted
// execution PVC. The directory is component configuration, never an RPC field.
func PrepareInput(ctx context.Context, client *s3.Client, execution biz.Execution, binding biz.WorkspaceBinding, directory string) (biz.WorkspaceBinding, error) {
	return biz.WorkspaceBinding{}, errors.New("PIPELINE_IO_NOT_IMPLEMENTED")
}

// UploadOutput sends independently collected actual files and the manifest.
// This returns a candidate; only a later step may ask the owner to verify and
// publish after the uploader container has actually terminated successfully.
func UploadOutput(ctx context.Context, client *s3.Client, execution biz.Execution, directory string, output biz.CollectedOutput) (biz.RuntimePublication, error) {
	return biz.RuntimePublication{}, errors.New("PIPELINE_IO_NOT_IMPLEMENTED")
}
