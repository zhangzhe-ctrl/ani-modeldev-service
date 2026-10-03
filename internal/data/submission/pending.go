package submission

import (
 "context"
 "errors"

 "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

var _ biz.PendingAdmissionRepository = (*Repository)(nil)
func (*Repository) ListPendingAdmissions(context.Context,biz.PipelineDispatchBinding,int)([]biz.Admission,error) {
 return nil,errors.New("PENDING_DISPATCH_NOT_IMPLEMENTED")
}
