package objectstore

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// Verifier uses an owner-configured S3 client. Endpoints and credentials never
// come from a component-supplied object reference.
type Verifier struct {
	client       *s3.Client
	connectionID string
}

func NewVerifier(client *s3.Client, connectionID string) *Verifier {
	return &Verifier{client: client, connectionID: connectionID}
}

// Verify reads an exact object version under the owner's approved storage scope.
// The first slice requires a real VersionID; mutable unversioned keys are not
// sufficient to turn a point-in-time read into an immutable artifact reference.
func (v *Verifier) Verify(context.Context, cpup01.StorageScope, cpup01.FixedObjectRef) (biz.VerifiedObject, error) {
	return biz.VerifiedObject{}, biz.ErrObjectVerification
}
