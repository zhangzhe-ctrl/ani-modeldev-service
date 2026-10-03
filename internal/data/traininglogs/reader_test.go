package traininglogs_test

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/traininglogs"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

func TestTrainingLogsBoundedTailRejectsPodUIDReplacement(t *testing.T) {
	source := biz.TrainingLogSource{LogID: "11111111-2222-4333-8444-555555555555", Namespace: "training", NamespaceUID: "namespace-uid", PodName: "actual-training", PodUID: "original-pod-uid", OwnerUID: "original-job-uid", OwnerName: "actual-training-job", OwnerKind: "Job", OwnerAPIVersion: "batch/v1", ContainerName: "node"}
	var replace, logsRead, hang atomic.Bool
	peer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-log-owner" || r.Method != http.MethodGet {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/training":
			_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": source.Namespace, "uid": source.NamespaceUID}})
		case "/api/v1/namespaces/training/pods/actual-training":
			uid := source.PodUID
			if replace.Load() && logsRead.Load() {
				uid = "replacement-pod-uid"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": source.PodName, "namespace": source.Namespace, "uid": uid, "ownerReferences": []any{map[string]any{"apiVersion": source.OwnerAPIVersion, "kind": source.OwnerKind, "name": source.OwnerName, "uid": source.OwnerUID, "controller": true}}}, "spec": map[string]any{"containers": []any{map[string]any{"name": "node"}}}, "status": map[string]any{"containerStatuses": []any{map[string]any{"name": "node", "restartCount": 0}}}})
		case "/api/v1/namespaces/training/pods/actual-training/log":
			if hang.Load() {
				<-r.Context().Done()
				return
			}
			q := r.URL.Query()
			if q.Get("container") != "node" || q.Get("timestamps") != "true" || q.Get("follow") == "true" || q.Get("previous") == "true" {
				t.Error("unbounded or caller-selected training log source")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			lines, _ := strconv.Atoi(q.Get("tailLines"))
			limit, _ := strconv.Atoi(q.Get("limitBytes"))
			if lines < 1 || lines > 1001 || limit < 1 || limit > 65537 {
				t.Error("log limits missing")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			body := "2026-10-03T01:00:00Z optimizer step 1\n2026-10-03T01:00:01Z optimizer step 2\n2026-10-03T01:00:02Z optimizer step 3\n"
			if len(body) > limit {
				body = body[:limit]
			}
			logsRead.Store(true)
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(body))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer peer.Close()
	client, err := coreclient.NewForConfig(&rest.Config{Host: peer.URL, BearerToken: "synthetic-log-owner", TLSClientConfig: rest.TLSClientConfig{CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: peer.Certificate().Raw})}})
	if err != nil {
		t.Fatal(err)
	}
	reader := traininglogs.New(client)
	got, err := reader.ReadTrainingLogs(context.Background(), source, biz.TrainingLogOptions{TailLines: 2, MaxBytes: 256})
	if err != nil || len(got.Lines) != 2 || got.Lines[0].Text != "optimizer step 2" || got.Lines[1].Text != "optimizer step 3" || !got.Truncated {
		t.Fatalf("TRAINING_LOG_READER_NOT_IMPLEMENTED: bounded actual-container tail unavailable: %+v %v", got, err)
	}
	got, err = reader.ReadTrainingLogs(context.Background(), source, biz.TrainingLogOptions{TailLines: 1000, MaxBytes: 80})
	if err != nil || !got.Truncated || len(got.Lines) == 0 {
		t.Fatalf("byte-limited logs must preserve readable data and mark truncation: %+v %v", got, err)
	}
	size := 0
	for _, line := range got.Lines {
		size += len(line.Text)
		if strings.Contains(line.Text, "2026-") {
			t.Fatal("timestamp envelope leaked into training text")
		}
	}
	if size > 80 {
		t.Fatal("training log exceeded caller byte bound")
	}
	replace.Store(true)
	logsRead.Store(false)
	if _, err := reader.ReadTrainingLogs(context.Background(), source, biz.TrainingLogOptions{TailLines: 2, MaxBytes: 256}); !errors.Is(err, biz.ErrTrainingLogsUnavailable) {
		t.Fatalf("same-name replacement returned another Pod's logs: %v", err)
	}
	replace.Store(false)
	hang.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	began := time.Now()
	if _, err := reader.ReadTrainingLogs(ctx, source, biz.TrainingLogOptions{TailLines: 2, MaxBytes: 256}); err == nil || time.Since(began) > time.Second {
		t.Fatalf("training log request ignored its caller deadline: %v", err)
	}
}
