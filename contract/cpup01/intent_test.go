package cpup01_test

import (
	"testing"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
)

// This literal is the normative v1 wire vector, not JSON produced by the code
// under test. Consumers use the same vector at their transport boundary.
func TestIntentCanonicalizesUserRequestWithoutResolvingDefaults(t *testing.T) {
	raw := []byte(`{"dataset_version_id":"BBBBBBBB-BBBB-4BBB-8BBB-BBBBBBBBBBBB","name":"CPU <试验>","general_parameters":[{"value":"0.0100","type":"DECIMAL","name":"learning_rate"},{"name":"epochs","type":"INTEGER","value":"3"}],"preset_id":"AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA","kind":"GENERAL_TRAINING"}`)
	intent, err := cpup01.ParseIntent(raw)
	if err != nil {
		t.Fatalf("valid registered CPU intent rejected: %v", err)
	}
	canonical, digest, err := cpup01.CanonicalIntent(intent)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema":"ani.modeldev.intent.v1","name":"CPU <试验>","kind":"GENERAL_TRAINING","preset_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","dataset_version_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","general_parameters":[{"name":"epochs","type":"INTEGER","value":"3"},{"name":"learning_rate","type":"DECIMAL","value":"0.01"}]}`
	if string(canonical) != want {
		t.Fatalf("canonical intent = %s; want %s", canonical, want)
	}
	if len(digest) != 64 {
		t.Fatalf("missing SHA256: %q", digest)
	}
}
