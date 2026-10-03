package traininglogs

import (
    "context"

    "github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
    coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
)

type Reader struct { client coreclient.CoreV1Interface }

func New(client coreclient.CoreV1Interface) *Reader { return &Reader{client: client} }

func (reader *Reader) ReadTrainingLogs(context.Context, biz.TrainingLogSource, biz.TrainingLogOptions) (biz.TrainingLogResult, error) {
    return biz.TrainingLogResult{}, biz.ErrTrainingLogsUnavailable
}
