package execution_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/execution"
)

func TestUserStopConflictingReplayPreservesOriginalFactsAndFence(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*biz.CloseIntent)
	}{
		{"different actor", func(c *biz.CloseIntent) { c.RequestedActor = "governance:user:43" }},
		{"different request time", func(c *biz.CloseIntent) { c.RequestedAt = c.RequestedAt.Add(time.Microsecond) }},
		{"different spec", func(c *biz.CloseIntent) { c.SpecHash = strings.Repeat("9", 64) }},
		{"same operation different execution", func(c *biz.CloseIntent) { c.ExecutionID = otherExecution }},
		{"same execution different operation", func(c *biz.CloseIntent) { c.OperationID = otherOperation }},
		{"other tenant reuses both identities", func(c *biz.CloseIntent) { c.TenantID = otherTenant }},
		{"other tenant reuses operation", func(c *biz.CloseIntent) { c.TenantID, c.ExecutionID = otherTenant, otherExecution }},
		{"other tenant reuses execution", func(c *biz.CloseIntent) { c.TenantID, c.OperationID = otherTenant, otherOperation }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repository := execution.New(preparePostgreSQL(t)())
			original := userStopIntent(t)
			candidate := original
			testCase.mutate(&candidate)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := repository.ApplyCloseIntent(ctx, original); err != nil {
				t.Fatalf("initial close: %v", err)
			}
			got, err := repository.ApplyCloseIntent(ctx, candidate)
			assertEmptyCloseFailure(t, got, err, biz.ErrAdmissionConflict)
			stored, err := repository.GetCloseIntent(ctx, original.TenantID, original.ExecutionID)
			if err != nil {
				t.Fatalf("GetCloseIntent after conflict: %v", err)
			}
			assertInitialCloseTombstone(t, stored, original)
			if candidate.TenantID != original.TenantID || candidate.ExecutionID != original.ExecutionID {
				absent, err := repository.GetCloseIntent(ctx, candidate.TenantID, candidate.ExecutionID)
				assertEmptyCloseFailure(t, absent, err, biz.ErrExecutionNotFound)
			}
			// A rejected attempt must not consume a hidden owner generation.
			next := original
			next.SourceGeneration++
			next.RequestedAt = next.RequestedAt.Add(time.Minute)
			second, err := repository.ApplyCloseIntent(ctx, next)
			if err != nil {
				t.Fatalf("next legitimate close after conflict: %v", err)
			}
			assertCloseTombstone(t, second, next, 2)
		})
	}
}

func TestGetCloseIntentHidesOtherTenantAndRejectsMalformedIdentities(t *testing.T) {
	repository := execution.New(preparePostgreSQL(t)())
	first, second := userStopIntent(t), userStopIntent(t)
	second.TenantID, second.OperationID, second.ExecutionID = otherTenant, otherOperation, otherExecution
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, intent := range []biz.CloseIntent{first, second} {
		if _, err := repository.ApplyCloseIntent(ctx, intent); err != nil {
			t.Fatalf("independent tenant close: %v", err)
		}
		stored, err := repository.GetCloseIntent(ctx, intent.TenantID, intent.ExecutionID)
		if err != nil {
			t.Fatalf("read own tenant close: %v", err)
		}
		assertInitialCloseTombstone(t, stored, intent)
	}
	for _, lookup := range [][2]string{{first.TenantID, second.ExecutionID}, {second.TenantID, first.ExecutionID}} {
		got, err := repository.GetCloseIntent(ctx, lookup[0], lookup[1])
		assertEmptyCloseFailure(t, got, err, biz.ErrExecutionNotFound)
	}
	for _, lookup := range [][2]string{
		{strings.ReplaceAll(first.TenantID, "-", "X"), first.ExecutionID},
		{first.TenantID, strings.ReplaceAll(first.ExecutionID, "-", "X")},
	} {
		got, err := repository.GetCloseIntent(ctx, lookup[0], lookup[1])
		assertEmptyCloseFailure(t, got, err, biz.ErrInvalidAdmission)
	}
}

func TestUserStopConcurrentDuplicatesAcrossSixPoolsAllocateOneFence(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	intent := userStopIntent(t)
	commands := make([]biz.CloseIntent, 6)
	for i := range commands {
		commands[i] = intent
	}
	for _, outcome := range raceCloseIntents(t, openRuntimePool, commands) {
		if outcome.err != nil {
			t.Errorf("concurrent close replay %d rejected: %v", outcome.index, outcome.err)
			continue
		}
		assertInitialCloseTombstone(t, outcome.record, intent)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	repository := execution.New(openRuntimePool())
	stored, err := repository.GetCloseIntent(ctx, intent.TenantID, intent.ExecutionID)
	if err != nil {
		t.Fatalf("GetCloseIntent after six-pool race: %v", err)
	}
	assertInitialCloseTombstone(t, stored, intent)
	next := intent
	next.SourceGeneration++
	next.RequestedAt = next.RequestedAt.Add(time.Minute)
	got, err := repository.ApplyCloseIntent(ctx, next)
	if err != nil {
		t.Fatalf("next distinct source after replay race: %v", err)
	}
	assertCloseTombstone(t, got, next, 2)
}

func TestUserStopConcurrentDifferentFactsHaveOneWinner(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	first, second := userStopIntent(t), userStopIntent(t)
	second.RequestedActor = "governance:user:43"
	commands := []biz.CloseIntent{first, second}
	winner, successes := -1, 0
	for _, outcome := range raceCloseIntents(t, openRuntimePool, commands) {
		if outcome.err == nil {
			winner, successes = outcome.index, successes+1
			assertInitialCloseTombstone(t, outcome.record, commands[outcome.index])
		} else {
			assertEmptyCloseFailure(t, outcome.record, outcome.err, biz.ErrAdmissionConflict)
		}
	}
	if successes != 1 {
		t.Fatalf("competing close facts produced %d winners, want one", successes)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stored, err := execution.New(openRuntimePool()).GetCloseIntent(ctx, first.TenantID, first.ExecutionID)
	if err != nil {
		t.Fatalf("GetCloseIntent after fact race: %v", err)
	}
	assertInitialCloseTombstone(t, stored, commands[winner])
}

func TestUserStopEarlierSourceReplayDoesNotReplaceLatestFence(t *testing.T) {
	openRuntimePool := preparePostgreSQL(t)
	repository := execution.New(openRuntimePool())
	first := userStopIntent(t)
	second := first
	second.SourceGeneration = 99
	second.RequestedAt = second.RequestedAt.Add(time.Minute)
	second.RequestedActor = "governance:user:43"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i, intent := range []biz.CloseIntent{first, second} {
		got, err := repository.ApplyCloseIntent(ctx, intent)
		if err != nil {
			t.Fatalf("close source %d: %v", intent.SourceGeneration, err)
		}
		assertCloseTombstone(t, got, intent, uint64(i+1))
	}
	repository = execution.New(openRuntimePool())
	replayed, err := repository.ApplyCloseIntent(ctx, first)
	if err != nil {
		t.Fatalf("earlier source replay: %v", err)
	}
	assertInitialCloseTombstone(t, replayed, first)
	latest, err := repository.GetCloseIntent(ctx, first.TenantID, first.ExecutionID)
	if err != nil {
		t.Fatalf("latest close after earlier replay: %v", err)
	}
	assertCloseTombstone(t, latest, second, 2)
}

func userStopIntent(t *testing.T) biz.CloseIntent {
	t.Helper()
	admission := validAdmission(t)
	return biz.CloseIntent{
		TenantID: admission.TenantID, OperationID: admission.OperationID, ExecutionID: admission.ExecutionID,
		SpecHash: admission.SpecHash, SourceGeneration: 41, Reason: biz.CloseReasonUserStop,
		RequestedAt: admission.AcceptedAt.Add(time.Minute), RequestedActor: "governance:user:42",
	}
}

func assertEmptyCloseFailure(t *testing.T, got biz.CloseRecord, err, want error) {
	t.Helper()
	if !errors.Is(err, want) || err.Error() != want.Error() || !reflect.DeepEqual(got, biz.CloseRecord{}) {
		t.Fatalf("close failure must be stable %s with an empty receipt; got error %v, nonempty=%t", want, err, !reflect.DeepEqual(got, biz.CloseRecord{}))
	}
}

type closeOutcome struct {
	index  int
	record biz.CloseRecord
	err    error
}

func raceCloseIntents(t *testing.T, openRuntimePool func() *pgxpool.Pool, commands []biz.CloseIntent) []closeOutcome {
	t.Helper()
	repositories := make([]*execution.Repository, len(commands))
	for i := range repositories {
		repositories[i] = execution.New(openRuntimePool())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan closeOutcome, len(commands))
	for i, command := range commands {
		go func(index int, intent biz.CloseIntent) {
			<-start
			record, err := repositories[index].ApplyCloseIntent(ctx, intent)
			results <- closeOutcome{index: index, record: record, err: err}
		}(i, command)
	}
	close(start)
	outcomes := make([]closeOutcome, 0, len(commands))
	for range commands {
		select {
		case result := <-results:
			outcomes = append(outcomes, result)
		case <-ctx.Done():
			t.Fatal("concurrent close attempts did not finish within the bounded context")
		}
	}
	return outcomes
}
