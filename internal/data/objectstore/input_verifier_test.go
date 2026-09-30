package objectstore_test

import (
	"context"
	"encoding/csv"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/objectstore"
)

func TestVerifyCSVReadsFixedVersionAndActualRegisteredShape(t *testing.T) {
	// These are module fixture bytes, not the managed production dataset.
	payload := inputCSVFixture(t)
	scope, object := objectFixture(payload)
	object.Key = "tenant/execution/input.csv"
	var gets atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gets.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/cpu-artifacts/tenant/execution/input.csv" || r.URL.Query().Get("versionId") != "version-1" {
			t.Errorf("unexpected fixed-input request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("x-amz-version-id", "version-1")
		w.Header().Set("ETag", `"not-the-content-digest"`)
		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()
	got, err := objectstore.NewVerifier(testS3Client(server), scope.StorageConnectionID, 32*1024*1024).VerifyCSV(context.Background(), scope, object)
	if err != nil {
		t.Fatalf("verify fixed CSV bytes: %v", err)
	}
	if gets.Load() != 1 || got.Object != object || got.SchemaVersion != "ani.cpu.csv.v1" || got.RowCount != 1024 || got.FeatureCount != 16 || got.VerifiedAt.IsZero() {
		t.Fatalf("missing observed immutable input facts: %+v", got)
	}
}

func inputCSVFixture(t *testing.T) string {
	t.Helper()
	var buffer strings.Builder
	writer := csv.NewWriter(&buffer)
	writer.UseCRLF = true
	header := make([]string, 17)
	row := make([]string, 17)
	for column := 0; column < 16; column++ {
		header[column] = "x" + strconv.Itoa(column)
		row[column] = "0.25"
	}
	header[16] = "label"
	if err := writer.Write(header); err != nil {
		t.Fatal(err)
	}
	for sample := 0; sample < 1024; sample++ {
		row[16] = strconv.Itoa(sample % 2)
		if err := writer.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatal(err)
	}
	return buffer.String()
}
