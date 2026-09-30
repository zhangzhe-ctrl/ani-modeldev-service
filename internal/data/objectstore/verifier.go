package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
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
func (v *Verifier) Verify(ctx context.Context, scope cpup01.StorageScope, object cpup01.FixedObjectRef) (biz.VerifiedObject, error) {
	if err := ctx.Err(); err != nil { return biz.VerifiedObject{}, err }
	if v == nil || v.client == nil || v.connectionID == "" || scope.StorageConnectionID != v.connectionID || object.StorageConnectionID != v.connectionID || object.Bucket != scope.Bucket || !bucketPattern.MatchString(scope.Bucket) || !validKey(scope.ApprovedPrefix) || !validKey(object.Key) || !strings.HasPrefix(object.Key, scope.ApprovedPrefix+"/") {
		return biz.VerifiedObject{}, biz.ErrObjectVerification
	}
	if object.VersionID == nil || object.ImmutableCopy != nil || !validText(*object.VersionID) || strings.EqualFold(*object.VersionID, "null") || object.SizeBytes <= 0 || len(object.SHA256) != 64 || strings.ToLower(object.SHA256) != object.SHA256 {
		return biz.VerifiedObject{}, biz.ErrObjectVerification
	}
	if _, err := hex.DecodeString(object.SHA256); err != nil { return biz.VerifiedObject{}, biz.ErrObjectVerification }
	version := *object.VersionID
	response, err := v.client.GetObject(ctx, &s3.GetObjectInput{Bucket:aws.String(object.Bucket), Key:aws.String(object.Key), VersionId:&version})
	if err != nil {
		if ctx.Err() != nil { return biz.VerifiedObject{}, ctx.Err() }
		return biz.VerifiedObject{}, biz.ErrObjectVerification
	}
	if response == nil || response.Body == nil { return biz.VerifiedObject{}, biz.ErrObjectVerification }
	defer response.Body.Close()
	if response.VersionId == nil || *response.VersionId != version { return biz.VerifiedObject{}, biz.ErrObjectVerification }
	digest := sha256.New()
	size, err := io.Copy(digest, io.LimitReader(response.Body, object.SizeBytes))
	if err != nil {
		if ctx.Err() != nil { return biz.VerifiedObject{}, ctx.Err() }
		return biz.VerifiedObject{}, biz.ErrObjectVerification
	}
	var extra [1]byte
	n, extraErr := response.Body.Read(extra[:])
	if err := ctx.Err(); err != nil { return biz.VerifiedObject{}, err }
	if size != object.SizeBytes || n != 0 || extraErr != io.EOF || hex.EncodeToString(digest.Sum(nil)) != object.SHA256 {
		return biz.VerifiedObject{}, biz.ErrObjectVerification
	}
	object.VersionID = &version
	return biz.VerifiedObject{Object:object, VerifiedAt:time.Now().UTC()}, nil
}

var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func validKey(value string) bool {
	return validText(value) && len(value) <= 1024 && value != "." && !strings.HasPrefix(value,"/") && !strings.Contains(value,"\\") && path.Clean(value) == value
}

func validText(value string) bool {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.TrimSpace(value) != value { return false }
	for _, r := range value { if unicode.IsControl(r) { return false } }
	return true
}
