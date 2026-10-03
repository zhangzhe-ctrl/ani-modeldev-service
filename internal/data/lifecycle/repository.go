package lifecycle

import (
 "context"
 "errors"

 "github.com/jackc/pgx/v5/pgxpool"
 "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

type Repository struct { pool *pgxpool.Pool }
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }
var _ biz.ExecutionRuntimeRepository = (*Repository)(nil)
var errNotImplemented = errors.New("EXECUTION_RUNTIME_NOT_IMPLEMENTED")
func (*Repository) GetRuntime(context.Context,string,string)(biz.ExecutionRuntime,error) { return biz.ExecutionRuntime{},errNotImplemented }
func (*Repository) RecordPrepared(context.Context,biz.RunAuthorityCandidate,biz.WorkspaceBinding)(biz.ExecutionRuntime,bool,error) { return biz.ExecutionRuntime{},false,errNotImplemented }
func (*Repository) ReserveTraining(context.Context,biz.RunAuthorityCandidate)(biz.TrainingReservation,error) { return biz.TrainingReservation{},errNotImplemented }
func (*Repository) RecordTrainingHandle(context.Context,biz.RunAuthorityCandidate,biz.TrainingHandle)(biz.ExecutionRuntime,error) { return biz.ExecutionRuntime{},errNotImplemented }
func (*Repository) RecordTrainingObservation(context.Context,biz.RunAuthorityCandidate,biz.TrainingRuntimeObservation)(biz.ExecutionRuntime,error) { return biz.ExecutionRuntime{},errNotImplemented }
func (*Repository) RecordPublication(context.Context,biz.RunAuthorityCandidate,biz.RuntimePublication)(biz.ExecutionRuntime,bool,error) { return biz.ExecutionRuntime{},false,errNotImplemented }
func (*Repository) RequestRuntimeClose(context.Context,biz.RunAuthorityCandidate,string)(biz.ExecutionRuntime,bool,error) { return biz.ExecutionRuntime{},false,errNotImplemented }
func (*Repository) ConfirmRuntimeClosed(context.Context,biz.RunAuthorityCandidate,uint64,biz.TrainingRuntimeObservation)(biz.ExecutionRuntime,error) { return biz.ExecutionRuntime{},errNotImplemented }
