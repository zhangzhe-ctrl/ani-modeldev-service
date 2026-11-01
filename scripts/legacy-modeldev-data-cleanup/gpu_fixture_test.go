package main

import (
	accpb "github.com/zhangzhe-ctrl/ani-accelerator-service/api/gen/go/accelerator/v1"
	"github.com/zhangzhe-ctrl/ani-accelerator-service/pkg/gpuprotocol"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	codec "github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01/protobuf"
	compute "github.com/zhangzhe-ctrl/ani-resource-service/api/compute/v1"
	"strings"
	"testing"
)

func gpuCleanupSnapshot(t *testing.T, s cpup01.Snapshot) cpup01.Snapshot {
	t.Helper()

	selection := &compute.ComputeSelection{SchemaVersion: 1, PresetId: s.ComputeOrigin.ComputePresetID, Code: "gpu-t4", Name: "T4", ComputeType: "gpu", CpuMillis: 1000, MemoryMib: 1024, Gpu: &compute.GpuSelection{ProfileId: "12121212-1212-4212-8212-121212121212", ProfileVersion: 3, SpecDigest: strings.Repeat("a", 64), DevicesPerUnit: 1, ClusterId: s.Environment.ClusterID, PoolId: "13131313-1313-4313-8313-131313131313", Mode: accpb.SupplyMode_SHARED_FIXED, ModelKey: "nvidia-t4", MemoryMibPerDevice: 2048, CoreLimitPercent: 30}}
	var err error
	selection.SelectionDigest, err = compute.SelectionDigest(selection)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := compute.CanonicalSelection(selection)
	if err != nil {
		t.Fatal(err)
	}
	s.ComputeOrigin, err = codec.DecodeComputeSelection(raw, selection.SelectionDigest)
	if err != nil {
		t.Fatal(err)
	}
	e := &accpb.GpuExecution{SchedulerName: "volcano", QueueName: "gpu-task", RuntimeClassName: "nvidia", NodeSelector: map[string]string{"accelerator.ani.io/supply-group": "14141414-1414-4414-8414-141414141414", "accelerator.ani.io/model-key": "nvidia-t4", "accelerator.ani.io/baseline-id": "15151515-1515-4515-8515-151515151515"}, PodAnnotations: map[string]string{"volcano.sh/vgpu-mode": "hami-core"}, Resources: map[string]string{"volcano.sh/vgpu-number": "1", "volcano.sh/vgpu-memory": "2048", "volcano.sh/vgpu-cores": "30"}}
	e.ExecutionDigest, err = gpuprotocol.GpuExecutionDigest(e)
	if err != nil {
		t.Fatal(err)
	}
	er, dr, err := codec.EncodeGPUPlanCanonical(e, &accpb.GpuDisplay{ProfileId: selection.Gpu.ProfileId, ProfileVersion: 3, ProfileName: "T4 fixture", ModelKey: "nvidia-t4", Mode: selection.Gpu.Mode})
	if err != nil {
		t.Fatal(err)
	}
	s.GPU, err = codec.DecodeGPUPlanCanonical(er, dr)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
