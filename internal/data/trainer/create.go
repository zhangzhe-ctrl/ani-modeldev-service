package trainer

import (
	"context"
	"errors"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// CreateTraining is called only by the holder of the durable first creation
// permit. Reading an existing intent must use FindTraining, never another POST.
func (a *Adapter) CreateTraining(context.Context, biz.TrainingPlan) (biz.TrainingHandle, error) {
	return biz.TrainingHandle{}, errors.New("TRAINING_CREATE_NOT_IMPLEMENTED")
}

func (a *Adapter) FindTraining(context.Context, biz.TrainingPlan) (biz.TrainingHandle, error) {
	return biz.TrainingHandle{}, errors.New("TRAINING_FIND_NOT_IMPLEMENTED")
}

func (a *Adapter) ObserveTraining(context.Context, biz.TrainingPlan, biz.TrainingHandle, []biz.RuntimeResource) (biz.TrainingRuntimeObservation, error) {
	return biz.TrainingRuntimeObservation{}, errors.New("TRAINING_OBSERVE_NOT_IMPLEMENTED")
}

func (a *Adapter) StopTraining(context.Context, biz.TrainingPlan, biz.TrainingHandle) error {
	return errors.New("TRAINING_STOP_NOT_IMPLEMENTED")
}
