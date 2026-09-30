package input_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/objectstore"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestManagedInputImportFreezesBeforeReadingBytesAndCommitsReady(t *testing.T) {
	openPool := postgres.Prepare(t)
	repository, independentReader := input.New(openPool()), input.New(openPool())
	request, payload := managedImportFixture()
	var reads atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/"+request.Object.Bucket+"/"+request.Object.Key || r.URL.Query().Get("versionId") != *request.Object.VersionID {
			t.Error("import did not read the frozen object version")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		frozen, err := independentReader.Get(r.Context(), request.TenantID, request.InputVersionID)
		if err != nil || frozen.State != biz.InputStateValidating || frozen.Verification != nil || !reflect.DeepEqual(frozen.Import, request) {
			t.Error("object read occurred before the original request was committed")
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.Header().Set("x-amz-version-id", *request.Object.VersionID)
		w.Header().Set("ETag", `"metadata-is-not-byte-verification"`)
		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()
	verifier := managedImportVerifier(server, request.Scope.StorageConnectionID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready, err := biz.NewInputImporter(repository, verifier).ImportCSV(ctx, request)
	if err != nil {
		t.Fatalf("import using real PostgreSQL and the S3 SDK boundary: %v", err)
	}
	if reads.Load() != 1 || ready.State != biz.InputStateReady || !reflect.DeepEqual(ready.Import, request) || ready.Verification == nil || ready.Verification.ValidateFor(request) != nil {
		t.Fatalf("input became ready without the expected durable byte proof: %+v", ready)
	}
	recovered, err := input.New(openPool()).Get(ctx, request.TenantID, request.InputVersionID)
	if err != nil || !reflect.DeepEqual(recovered, ready) {
		t.Fatalf("READY was returned before its observation became durable: %+v, %v", recovered, err)
	}
}

func TestManagedInputImportRejectsBadRemoteProofWithoutExposingReady(t *testing.T) {
	for _, name := range []string{"different bytes", "different version", "invalid CSV shape"} {
		t.Run(name, func(t *testing.T) {
			openPool := postgres.Prepare(t)
			request, payload := managedImportFixture()
			version := *request.Object.VersionID
			switch name {
			case "different bytes":
				payload = strings.Replace(payload, "0.25", "0.26", 1)
			case "different version":
				version = "unexpected-source-version"
			case "invalid CSV shape":
				payload = strings.Replace(payload, "x0,", "wrong_feature,", 1)
				request.Object.SizeBytes = int64(len(payload))
				hash := sha256.Sum256([]byte(payload))
				request.Object.SHA256 = hex.EncodeToString(hash[:])
			}
			var reads atomic.Int64
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reads.Add(1)
				w.Header().Set("x-amz-version-id", version)
				_, _ = io.WriteString(w, payload)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got, err := biz.NewInputImporter(input.New(openPool()), managedImportVerifier(server, request.Scope.StorageConnectionID)).ImportCSV(ctx, request)
			if !errors.Is(err, biz.ErrInputVerification) || reads.Load() != 1 || got.State == biz.InputStateReady || got.Verification != nil {
				t.Fatalf("invalid remote proof exposed a ready input: %+v, %v", got, err)
			}
			stored, err := input.New(openPool()).Get(ctx, request.TenantID, request.InputVersionID)
			if err != nil || stored.State == biz.InputStateReady || stored.Verification != nil || !reflect.DeepEqual(stored.Import, request) {
				t.Fatalf("failed verification lost or promoted the frozen request: %+v, %v", stored, err)
			}
		})
	}
}

func TestManagedInputImportReplayAndConflictDoNotReadAnotherObject(t *testing.T) {
	openPool := postgres.Prepare(t)
	request, payload := managedImportFixture()
	var reads atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reads.Add(1)
		w.Header().Set("x-amz-version-id", *request.Object.VersionID)
		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()
	verifier := managedImportVerifier(server, request.Scope.StorageConnectionID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	writer := openPool()
	first, err := biz.NewInputImporter(input.New(writer), verifier).ImportCSV(ctx, request)
	if err != nil || first.State != biz.InputStateReady {
		t.Fatalf("initial verified import: %+v, %v", first, err)
	}
	writer.Close()
	reader := input.New(openPool())
	importer := biz.NewInputImporter(reader, verifier)
	replayed, err := importer.ImportCSV(ctx, request)
	if err != nil || !reflect.DeepEqual(replayed, first) || reads.Load() != 1 {
		t.Fatalf("replay changed the fixed version, proof, or repeated verification: %+v, %v", replayed, err)
	}
	conflicting := request
	conflicting.Object.Key = "tenant/input/replacement.csv"
	got, err := importer.ImportCSV(ctx, conflicting)
	if !errors.Is(err, biz.ErrInputConflict) || !reflect.DeepEqual(got, biz.InputVersion{}) || reads.Load() != 1 {
		t.Fatalf("conflicting retry reached another object or overwrote the import: %+v, %v", got, err)
	}
	requireStoredInputVersion(t, ctx, reader, first)
}

// The HTTPS server supplies module fixture bytes. It is not a real managed S3
// deployment, administrator identity, product endpoint or target-cluster proof.
func managedImportFixture() (biz.InputImport, string) {
	var payload strings.Builder
	for column := 0; column < 16; column++ {
		fmt.Fprintf(&payload, "x%d,", column)
	}
	payload.WriteString("label\n")
	for sample := 0; sample < 1024; sample++ {
		payload.WriteString(strings.Repeat("0.25,", 16))
		fmt.Fprintf(&payload, "%d\n", sample%2)
	}
	request := importFixture()
	request.RequestedAt = time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	request.Object.SizeBytes = int64(payload.Len())
	hash := sha256.Sum256([]byte(payload.String()))
	request.Object.SHA256 = hex.EncodeToString(hash[:])
	return request, payload.String()
}

func managedImportVerifier(server *httptest.Server, connectionID string) *objectstore.Verifier {
	client := s3.New(s3.Options{
		Region: "fixture-region", BaseEndpoint: aws.String(server.URL), UsePathStyle: true,
		Credentials: aws.AnonymousCredentials{}, HTTPClient: server.Client(), RetryMaxAttempts: 1,
	})
	return objectstore.NewVerifier(client, connectionID, 32*1024*1024)
}
