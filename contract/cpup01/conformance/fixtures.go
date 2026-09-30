// Package conformance exposes synthetic CPU-P01 vectors to consumer tests.
// Production packages must not use these fixtures as defaults or ENV facts.
package conformance

import (
	_ "embed"
	"encoding/json"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

const IntentSHA256V1 = "152ed6b8e0a392be5697ee138e5a22be4183139c2954f009d6d85483cdf4a01a"
const SnapshotSHA256V1 = "972dee14e65202d5d4da7da199cf5f37139b535701a0cb1b3de4d8ef8a5170b9"

//go:embed intent-input-v1.json
var intentInputV1 []byte

//go:embed intent-canonical-v1.json
var intentCanonicalV1 []byte

//go:embed snapshot-v1.json
var snapshotCanonicalV1 []byte

func IntentInputV1() []byte { return append([]byte{}, intentInputV1...) }

func IntentCanonicalV1() []byte { return append([]byte{}, intentCanonicalV1...) }

func SnapshotCanonicalV1() []byte { return append([]byte{}, snapshotCanonicalV1...) }

func IntentV1() cpup01.Intent {
	intent, err := cpup01.ParseIntent(intentInputV1)
	if err != nil { panic("invalid embedded CPU-P01 intent conformance fixture") }
	return intent
}

// SnapshotV1 returns fresh fixture values on each call, including all slices
// and optional object-reference fields. None identify a real environment.
func SnapshotV1() cpup01.Snapshot {
	var snapshot cpup01.Snapshot
	if err := json.Unmarshal(snapshotCanonicalV1, &snapshot); err != nil { panic("invalid embedded CPU-P01 snapshot conformance fixture") }
	if err := snapshot.Validate(); err != nil { panic("embedded CPU-P01 snapshot violates its contract") }
	return snapshot
}
