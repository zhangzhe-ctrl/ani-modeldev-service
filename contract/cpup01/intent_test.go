package cpup01_test

import (
	"errors"
	"strings"
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
	// Independently computed from the literal with Python hashlib on Fedora.
	if digest != "152ed6b8e0a392be5697ee138e5a22be4183139c2954f009d6d85483cdf4a01a" {
		t.Fatalf("intent SHA256 = %q", digest)
	}
}

func TestIntentRejectsAmbiguousOrUnsupportedUserInput(t *testing.T) {
	base := `{"name":"CPU","kind":"GENERAL_TRAINING","preset_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","dataset_version_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}`
	cases := map[string]string{
		"duplicate key":         strings.Replace(base, `"name":"CPU"`, `"name":"other","name":"CPU"`, 1),
		"null field":            strings.TrimSuffix(base, "}") + `,"image_version_id":null}`,
		"trailing JSON":         base + `{}`,
		"unknown field":         strings.TrimSuffix(base, "}") + `,"tenant_id":"forged"}`,
		"case variant key":      strings.Replace(base, `"name"`, `"Name"`, 1),
		"lone surrogate":        strings.Replace(base, "CPU", `CPU\ud800`, 1),
		"invalid UTF8":          strings.Replace(base, "CPU", string([]byte{0xff}), 1),
		"missing required":      strings.Replace(base, `"name":"CPU",`, "", 1),
		"unsupported kind":      strings.Replace(base, "GENERAL_TRAINING", "FINETUNING", 1),
		"zero UUID":             strings.Replace(base, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "00000000-0000-0000-0000-000000000000", 1),
		"empty optional":        strings.TrimSuffix(base, "}") + `,"image_version_id":""}`,
		"noncanonical name":     strings.Replace(base, "CPU", " CPU ", 1),
		"unsupported parameter": strings.TrimSuffix(base, "}") + `,"general_parameters":[{"name":"command","type":"STRING","value":"run"}]}`,
		"duplicate parameter":   strings.TrimSuffix(base, "}") + `,"general_parameters":[{"name":"epochs","type":"INTEGER","value":"3"},{"name":"epochs","type":"INTEGER","value":"3"}]}`,
		"wrong parameter type":  strings.TrimSuffix(base, "}") + `,"general_parameters":[{"name":"epochs","type":"DECIMAL","value":"3"}]}`,
		"changed CPU recipe":    strings.TrimSuffix(base, "}") + `,"general_parameters":[{"name":"epochs","type":"INTEGER","value":"4"}]}`,
		"exponent decimal":      strings.TrimSuffix(base, "}") + `,"general_parameters":[{"name":"learning_rate","type":"DECIMAL","value":"1e-2"}]}`,
		"decimal out of range":  strings.TrimSuffix(base, "}") + `,"general_parameters":[{"name":"learning_rate","type":"DECIMAL","value":"0.1001"}]}`,
		"zero decimal":          strings.TrimSuffix(base, "}") + `,"general_parameters":[{"name":"learning_rate","type":"DECIMAL","value":"0.000"}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := cpup01.ParseIntent([]byte(raw)); !errors.Is(err, cpup01.ErrInvalidArgument) {
				t.Fatalf("ambiguous/unsupported request accepted: %v", err)
			}
		})
	}
	if _, _, err := cpup01.CanonicalIntent(cpup01.Intent{}); !errors.Is(err, cpup01.ErrInvalidArgument) {
		t.Fatalf("typed transport bypassed validation: %v", err)
	}
}

func TestIntentPreservesPresenceWithoutMutatingCaller(t *testing.T) {
	base := cpup01.Intent{Name: "CPU", Kind: "GENERAL_TRAINING", PresetID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", DatasetVersionID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}
	_, absent, err := cpup01.CanonicalIntent(base)
	if err != nil {
		t.Fatal(err)
	}
	empty := []cpup01.Parameter{}
	base.GeneralParameters = &empty
	_, explicitEmpty, err := cpup01.CanonicalIntent(base)
	if err != nil {
		t.Fatal(err)
	}
	parameters := []cpup01.Parameter{{Name: "learning_rate", Type: "DECIMAL", Value: "0.0100"}}
	base.GeneralParameters = &parameters
	_, explicitDefault, err := cpup01.CanonicalIntent(base)
	if err != nil {
		t.Fatal(err)
	}
	if absent == explicitEmpty || absent == explicitDefault || explicitEmpty == explicitDefault {
		t.Fatal("optional-field presence was lost")
	}
	if parameters[0].Value != "0.0100" {
		t.Fatal("canonicalization mutated the caller")
	}
}
