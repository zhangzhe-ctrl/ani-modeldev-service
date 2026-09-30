package catalogue

import (
	"context"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
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
	return ReadRelease(ctx, reader.directory, releaseID, expectedDigest)
}
