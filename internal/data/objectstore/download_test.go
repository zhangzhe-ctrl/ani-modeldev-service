package objectstore_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/objectstore"
)

func TestDownloadSignerPinsVersionAndLifetime(t *testing.T) {
	scope := cpup01.StorageScope{StorageConnectionID: "approved-store", Bucket: "artifacts", ApprovedPrefix: "tenant/execution"}
	version := "immutable-version-1"
	object := cpup01.FixedObjectRef{StorageConnectionID: scope.StorageConnectionID, Bucket: scope.Bucket, Key: scope.ApprovedPrefix + "/model.pt", VersionID: &version, SizeBytes: 4903, SHA256: strings.Repeat("a", 64)}
	credentials := aws.Credentials{AccessKeyID: "synthetic-key", SecretAccessKey: "synthetic-secret", SessionToken: "synthetic-session", CanExpire: true, Expires: time.Now().Add(5 * time.Minute)}
	newSigner := func(endpoint string, credentials aws.Credentials) *objectstore.DownloadSigner {
		client := s3.New(s3.Options{Region: "us-east-1", BaseEndpoint: aws.String(endpoint), UsePathStyle: true, Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) { return credentials, nil })})
		return objectstore.NewDownloadSigner(client, scope.StorageConnectionID)
	}
	signer := newSigner("https://storage.example.test", credentials)
	grant, err := signer.SignDownload(context.Background(), scope, object)
	if err != nil {
		t.Fatalf("fixed-version download was not signed: %v", err)
	}
	location, err := url.Parse(grant.URL)
	if err != nil {
		t.Fatal("signer returned a malformed location")
	}
	query := location.Query()
	signedAt, err := time.Parse("20060102T150405Z", query.Get("X-Amz-Date"))
	if err != nil || location.Scheme != "https" || location.Host != "storage.example.test" || location.Path != "/artifacts/tenant/execution/model.pt" || query.Get("versionId") != version || query.Get("X-Amz-Expires") != "60" || query.Get("X-Amz-Signature") == "" || !grant.ExpiresAt.Equal(signedAt.Add(time.Minute)) {
		t.Fatal("download signature did not preserve the exact object version and 60-second lifetime")
	}
	expiring := credentials
	expiring.Expires = time.Now().Add(30 * time.Second)
	tests := []struct {
		name string
		signer *objectstore.DownloadSigner
		scope cpup01.StorageScope
		object cpup01.FixedObjectRef
	}{
		{name: "wrong scope", signer: signer, scope: cpup01.StorageScope{StorageConnectionID: scope.StorageConnectionID, Bucket: scope.Bucket, ApprovedPrefix: "other-tenant"}, object: object},
		{name: "unversioned object", signer: signer, scope: scope, object: cpup01.FixedObjectRef{StorageConnectionID: object.StorageConnectionID, Bucket: object.Bucket, Key: object.Key, SizeBytes: object.SizeBytes, SHA256: object.SHA256}},
		{name: "insecure endpoint", signer: newSigner("http://storage.example.test", credentials), scope: scope, object: object},
		{name: "credentials expire before grant", signer: newSigner("https://storage.example.test", expiring), scope: scope, object: object},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			grant, err := test.signer.SignDownload(context.Background(), test.scope, test.object)
			if !errors.Is(err, biz.ErrDownloadUnavailable) || grant.URL != "" || !grant.ExpiresAt.IsZero() {
				t.Fatalf("invalid download input returned a grant: %v", err)
			}
		})
	}
}
