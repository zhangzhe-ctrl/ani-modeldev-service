package objectstore

import (
	"context"
	"encoding/hex"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

type DownloadSigner struct {
	client       *s3.Client
	connectionID string
}

var _ biz.ArtifactSigner = (*DownloadSigner)(nil)

func NewDownloadSigner(client *s3.Client, connectionID string) *DownloadSigner {
	return &DownloadSigner{client: client, connectionID: connectionID}
}

// SignDownload signs one owner-selected, already published object version. It
// cannot select a different endpoint, bucket, key or version from user input.
func (s *DownloadSigner) SignDownload(ctx context.Context, scope cpup01.StorageScope, object cpup01.FixedObjectRef) (biz.DownloadGrant, error) {
	if ctx == nil || s == nil || s.client == nil || s.connectionID == "" || scope.StorageConnectionID != s.connectionID || object.StorageConnectionID != s.connectionID || object.Bucket != scope.Bucket || !bucketPattern.MatchString(scope.Bucket) || !biz.ValidStorageKey(scope.ApprovedPrefix) || !biz.ValidStorageKey(object.Key) || !strings.HasPrefix(object.Key, scope.ApprovedPrefix+"/") {
		return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
	}
	if err := ctx.Err(); err != nil {
		return biz.DownloadGrant{}, err
	}
	if object.VersionID == nil || object.ImmutableCopy != nil || !biz.ValidStorageReference(*object.VersionID) || strings.EqualFold(*object.VersionID, "null") || object.SizeBytes <= 0 || len(object.SHA256) != 64 || strings.ToLower(object.SHA256) != object.SHA256 {
		return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
	}
	if _, err := hex.DecodeString(object.SHA256); err != nil {
		return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
	}
	options := s.client.Options()
	if options.Credentials == nil {
		return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
	}
	if options.BaseEndpoint != nil {
		endpoint, err := url.Parse(*options.BaseEndpoint)
		if err != nil || !secureDownloadURL(endpoint) || endpoint.RawQuery != "" {
			return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
		}
	}
	credentials, err := options.Credentials.Retrieve(ctx)
	if err != nil || credentials.AccessKeyID == "" || credentials.SecretAccessKey == "" || credentials.CanExpire && !credentials.Expires.After(time.Now().Add(time.Minute)) {
		return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
	}
	// Freeze the retrieved credentials for this request, so a refresh cannot
	// change the lifetime between the expiry check and the actual signature.
	signer := s3.NewPresignClient(s.client, s3.WithPresignClientFromClientOptions(func(options *s3.Options) {
		options.Credentials = aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return credentials, nil
		})
	}))
	response, err := signer.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(object.Bucket), Key: aws.String(object.Key), VersionId: object.VersionID}, s3.WithPresignExpires(time.Minute))
	if err != nil || response == nil || response.Method != "GET" {
		return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
	}
	location, err := url.Parse(response.URL)
	if err != nil || !secureDownloadURL(location) {
		return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
	}
	query := location.Query()
	signedAt, err := time.Parse("20060102T150405Z", query.Get("X-Amz-Date"))
	if err != nil || query.Get("X-Amz-Expires") != "60" || query.Get("versionId") != *object.VersionID || query.Get("X-Amz-Signature") == "" {
		return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
	}
	expiresAt := signedAt.Add(time.Minute)
	if !expiresAt.After(time.Now()) || credentials.CanExpire && !credentials.Expires.After(expiresAt) {
		return biz.DownloadGrant{}, biz.ErrDownloadUnavailable
	}
	return biz.DownloadGrant{URL: response.URL, ExpiresAt: expiresAt.UTC()}, nil
}

func secureDownloadURL(location *url.URL) bool {
	return location != nil && location.Scheme == "https" && location.Hostname() != "" && location.User == nil && location.Fragment == "" && !location.ForceQuery
}
