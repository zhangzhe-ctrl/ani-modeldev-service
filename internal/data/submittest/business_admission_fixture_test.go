//go:build cpu_mainflow

package submittest_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/service"
)

func prepareMainFlowAdmission(t *testing.T, ctx context.Context, f *completeFixture, pool *pgxpool.Pool, store *s3.Client) (*service.Admission, biz.AdmissionReleaseSelection, cpup01.Intent) {
	t.Helper()
	release := f.request.Admission.Snapshot.Release
	return nil, biz.AdmissionReleaseSelection{ReleaseID: release.ReleaseID, ReleaseDigest: release.ReleaseDigest, BindingGeneration: 1}, f.request.Admission.Intent
}
