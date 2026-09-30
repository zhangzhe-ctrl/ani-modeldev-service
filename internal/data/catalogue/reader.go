// Package catalogue reads explicitly selected immutable CPU-P01 Releases.
package catalogue

import (
	"context"
	"errors"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

// ReadRelease reads only <releaseID>.json below a trusted configured directory.
// The caller supplies both the ID and digest selected by Governance. This
// adapter never chooses a current Release and never enables a loaded document.
func ReadRelease(ctx context.Context, directory, releaseID, expectedDigest string) (cpup01.ReleaseDocument, error) {
	return cpup01.ReleaseDocument{}, errors.New("RELEASE_NOT_IMPLEMENTED")
}
