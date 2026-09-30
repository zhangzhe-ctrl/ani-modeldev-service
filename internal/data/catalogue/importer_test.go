package catalogue_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/conformance"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/catalogue"
)

func TestImportReleaseIsReadableByItsFixedIdentityAfterSuccess(t *testing.T) {
	directory:=t.TempDir()
	want:=releaseFixtureDocument()
	if _,err:=catalogue.ReadRelease(context.Background(),directory,want.ReleaseID,conformance.ReleaseSHA256V1);!errors.Is(err,catalogue.ErrReleaseNotFound) {
		t.Fatalf("CPU04_CATALOGUE_PREFLIGHT: fresh directory read must be not found; behavior NOT_RUN: %v",err)
	}
	result,err:=catalogue.ImportRelease(context.Background(),directory,conformance.ReleaseCanonicalV1(),conformance.ReleaseSHA256V1)
	if err!=nil {t.Fatalf("import canonical Release: %v",err)}
	if !result.Created || result.ReleaseID!=want.ReleaseID || result.Digest!=conformance.ReleaseSHA256V1 {
		t.Fatalf("import did not acknowledge the exact installed identity: %+v",result)
	}
	// This reopens the actual file through the public reader; it does not inspect
	// an import cache or reuse a returned document as the persistence oracle.
	loaded,err:=catalogue.ReadRelease(context.Background(),directory,result.ReleaseID,result.Digest)
	if err!=nil || !reflect.DeepEqual(loaded,want) {
		t.Fatalf("successful import was not independently readable: %+v, %v",loaded,err)
	}
}
