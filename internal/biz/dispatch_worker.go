package biz

import (
 "context"
 "errors"
 "time"

 "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

// PipelineDispatchBinding is trusted service-owner configuration, not request
// data. Only admissions already frozen to this exact tenant/environment may use
// its explicitly supplied PipelineRoot and owner revision.
type PipelineDispatchBinding struct {
 TenantID string `json:"tenant_id"`
 Environment cpup01.EnvironmentBindingSnapshot `json:"environment"`
 Owner PipelineOwnerConfiguration `json:"owner"`
}

type PendingAdmissionRepository interface {
 ListPendingAdmissions(context.Context,PipelineDispatchBinding,int)([]Admission,error)
}

type DispatchWorker struct{}
func NewDispatchWorker(PendingAdmissionRepository,*PipelineSubmitter,PipelineDispatchBinding,int,time.Duration)(*DispatchWorker,error) {return &DispatchWorker{},nil}
func (*DispatchWorker) DispatchOnce(context.Context)(int,error){return 0,errors.New("PENDING_DISPATCH_NOT_IMPLEMENTED")}
func (*DispatchWorker) Run(context.Context)error{return errors.New("PENDING_DISPATCH_NOT_IMPLEMENTED")}
