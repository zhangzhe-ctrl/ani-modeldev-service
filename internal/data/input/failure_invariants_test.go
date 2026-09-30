package input_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/input"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/testsupport/postgres"
)

func TestInputTerminalFactsSurviveLateValidationOutcomes(t *testing.T) {
	for _, state := range []biz.InputState{biz.InputStateReady, biz.InputStateRejected} {
		t.Run(string(state), func(t *testing.T) {
			openPool := postgres.Prepare(t)
			repo := input.New(openPool())
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			request := importFixture()
			if _, err := repo.FreezeImport(ctx, request); err != nil {
				t.Fatal(err)
			}
			var first biz.InputVersion
			var err error
			if state == biz.InputStateReady {
				first, err = repo.RecordVerifiedCSV(ctx, request, verificationObservation(request))
			} else {
				first, err = repo.RecordValidationFailure(ctx, request, biz.InputValidationFailure{Code: biz.InputFailureContentRejected, ObservedAt: request.RequestedAt.Add(time.Second)})
			}
			if err != nil {
				t.Fatal(err)
			}
			reader := input.New(openPool())
			for _, code := range []biz.InputFailureCode{biz.InputFailureContentRejected, biz.InputFailureSourceUnavailable} {
				late := biz.InputValidationFailure{Code: code, ObservedAt: request.RequestedAt.Add(time.Hour)}
				got, err := reader.RecordValidationFailure(ctx, request, late)
				if err != nil || !reflect.DeepEqual(got, first) {
					t.Fatalf("late failure replaced a terminal fact: %+v, %v", got, err)
				}
			}
			proof := verificationObservation(request)
			proof.VerifiedAt = proof.VerifiedAt.Add(time.Hour)
			got, err := reader.RecordVerifiedCSV(ctx, request, proof)
			if state == biz.InputStateRejected {
				if !errors.Is(err, biz.ErrInputVerification) {
					t.Fatalf("late proof promoted rejected content: %+v, %v", got, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, first) {
				t.Fatalf("late proof changed first terminal observation: %+v", got)
			}
			requireStoredInputVersion(t, ctx, reader, first)
		})
	}
}

func TestInputRetryableFailureKeepsLatestObservationAndSuccessfulRetryClearsIt(t *testing.T) {
	openPool := postgres.Prepare(t)
	repo := input.New(openPool())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request := importFixture()
	if _, err := repo.FreezeImport(ctx, request); err != nil {
		t.Fatal(err)
	}
	first := biz.InputValidationFailure{Code: biz.InputFailureSourceUnavailable, ObservedAt: request.RequestedAt.Add(time.Second)}
	if _, err := repo.RecordValidationFailure(ctx, request, first); err != nil {
		t.Fatal(err)
	}
	latest := first
	latest.ObservedAt = time.Date(2026, 9, 30, 20, 1, 0, 123987, time.FixedZone("fixture-UTC+8", 8*60*60))
	got, err := repo.RecordValidationFailure(ctx, request, latest)
	wantFailure := biz.InputValidationFailure{Code: first.Code, ObservedAt: time.Date(2026, 9, 30, 12, 1, 0, 123000, time.UTC)}
	want := biz.InputVersion{Import: request, State: biz.InputStateValidating, Failure: &wantFailure}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("latest failure time was not preserved in microsecond UTC: %+v, %v", got, err)
	}
	reader := input.New(openPool())
	got, err = reader.RecordValidationFailure(ctx, request, first)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("old source error replaced the newer observation: %+v, %v", got, err)
	}
	proof := verificationObservation(request)
	ready, err := reader.RecordVerifiedCSV(ctx, request, proof)
	if err != nil || ready.State != biz.InputStateReady || ready.Failure != nil || ready.Verification == nil {
		t.Fatalf("successful verification did not clear retryable failure: %+v, %v", ready, err)
	}
	requireStoredInputVersion(t, ctx, input.New(openPool()), ready)
}

func TestInputFailureRequiresFrozenRequestAndFiniteObservation(t *testing.T) {
	openPool := postgres.Prepare(t)
	repo := input.New(openPool())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request := importFixture()
	failure := biz.InputValidationFailure{Code: biz.InputFailureContentRejected, ObservedAt: request.RequestedAt.Add(time.Second)}
	if _, err := repo.RecordValidationFailure(ctx, request, failure); !errors.Is(err, biz.ErrInputNotFound) {
		t.Fatalf("failure created an unrequested input: %v", err)
	}
	frozen, err := repo.FreezeImport(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		name   string
		update func(*biz.InputImport)
		want   error
	}{
		{"tenant", func(r *biz.InputImport) { r.TenantID = "88888888-8888-4888-8888-888888888888" }, biz.ErrInputNotFound},
		{"request", func(r *biz.InputImport) { r.RequestID = "55555555-5555-4555-8555-555555555555" }, biz.ErrInputConflict},
		{"actor", func(r *biz.InputImport) { r.Actor = "governance:user:99" }, biz.ErrInputConflict},
		{"credential", func(r *biz.InputImport) { r.Scope.CredentialReference = "managed-secret:other" }, biz.ErrInputConflict},
		{"object", func(r *biz.InputImport) { r.Object.Key = "tenant/input/other.csv" }, biz.ErrInputConflict},
	} {
		t.Run(change.name, func(t *testing.T) {
			candidate := request
			change.update(&candidate)
			if _, err := repo.RecordValidationFailure(ctx, candidate, failure); !errors.Is(err, change.want) {
				t.Fatalf("failure changed another frozen request: %v", err)
			}
			requireStoredInputVersion(t, ctx, repo, frozen)
		})
	}
	for _, invalid := range []biz.InputValidationFailure{
		{Code: "S3_ERROR_WITH_SECRET", ObservedAt: failure.ObservedAt},
		{Code: failure.Code},
		{Code: failure.Code, ObservedAt: request.RequestedAt.Add(-time.Nanosecond)},
		{Code: failure.Code, ObservedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		if _, err := repo.RecordValidationFailure(ctx, request, invalid); !errors.Is(err, biz.ErrInputVerification) {
			t.Fatalf("invalid failure observation persisted: %v", err)
		}
		requireStoredInputVersion(t, ctx, repo, frozen)
	}
}

func TestConcurrentInputReadyAndRejectionKeepOneTerminalFact(t *testing.T) {
	openPool := postgres.Prepare(t)
	repos := [2]*input.Repository{input.New(openPool()), input.New(openPool())}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	request := importFixture()
	if _, err := repos[0].FreezeImport(ctx, request); err != nil {
		t.Fatal(err)
	}
	type result struct {
		version biz.InputVersion
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		got, err := repos[0].RecordVerifiedCSV(ctx, request, verificationObservation(request))
		results <- result{got, err}
	}()
	go func() {
		defer workers.Done()
		<-start
		got, err := repos[1].RecordValidationFailure(ctx, request, biz.InputValidationFailure{Code: biz.InputFailureContentRejected, ObservedAt: request.RequestedAt.Add(time.Second)})
		results <- result{got, err}
	}()
	close(start)
	var first *biz.InputVersion
	for range 2 {
		select {
		case got := <-results:
			if got.err != nil && !(errors.Is(got.err, biz.ErrInputVerification) && got.version.State == biz.InputStateRejected) {
				t.Fatalf("concurrent finalization failed: %v", got.err)
			}
			if got.version.State != biz.InputStateReady && got.version.State != biz.InputStateRejected {
				t.Fatalf("no terminal fact: %+v", got.version)
			}
			if first == nil {
				first = &got.version
			} else if !reflect.DeepEqual(*first, got.version) {
				t.Fatalf("competing verifiers saw different terminal facts: %+v / %+v", *first, got.version)
			}
		case <-ctx.Done():
			t.Fatal("concurrent validation did not finish within its bound")
		}
	}
	requireStoredInputVersion(t, ctx, input.New(openPool()), *first)
}
