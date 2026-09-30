package input_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestFreezeImportExactRequestReplayAfterReconnectReturnsOriginalVersion(t *testing.T) {
	openPool := postgres.Prepare(t)
	writer := openPool()
	command := importFixture()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first, err := input.New(writer).FreezeImport(ctx, command)
	if err != nil { t.Fatalf("initial fixed import: %v", err) }
	writer.Close()
	retry := input.New(openPool())
	replayed, err := retry.FreezeImport(ctx, command)
	if err != nil || !reflect.DeepEqual(replayed, first) {
		t.Fatalf("same request failed to return original fixed import: %+v, %v", replayed, err)
	}
	stored, err := input.New(openPool()).Get(ctx, command.TenantID, command.InputVersionID)
	if err != nil || !reflect.DeepEqual(stored, first) {
		t.Fatalf("replay changed durable input facts: %+v, %v", stored, err)
	}
}
