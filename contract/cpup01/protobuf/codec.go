// Package protobuf adapts CPU-P01 pure contracts to the owned protobuf API.
// It is a transport boundary, not an authorization decision or live validator.
package protobuf

import (
	"errors"

	modeldevv1 "github.com/zhangzhe-ctrl/ani-modeldev-service/api/ani/modeldev/v1"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

var errNotImplemented = errors.New("CPU-P01 protobuf codec not implemented")

func EncodeIntent(intent cpup01.Intent) (*modeldevv1.UserIntent, error) {
	return nil, errNotImplemented
}

func DecodeIntent(message *modeldevv1.UserIntent) (cpup01.Intent, error) {
	return cpup01.Intent{}, errNotImplemented
}

func EncodeSnapshot(snapshot cpup01.Snapshot) (*modeldevv1.ExecutionSnapshot, error) {
	return nil, errNotImplemented
}

func DecodeSnapshot(message *modeldevv1.ExecutionSnapshot) (cpup01.Snapshot, error) {
	return cpup01.Snapshot{}, errNotImplemented
}
