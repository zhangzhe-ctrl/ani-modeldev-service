package main

import (
	_ "embed"
	"encoding/json"
)

type column struct {
	Table  string `json:"table_name"`
	Column string `json:"column_name"`
	Type   string `json:"data_type"`
}
type foreignKey struct {
	Name       string `json:"conname"`
	Child      string `json:"child"`
	Parent     string `json:"parent"`
	Definition string `json:"definition"`
}
type trigger struct {
	Name       string `json:"tgname"`
	Table      string `json:"table_name"`
	Definition string `json:"definition"`
}
type schemaContract struct {
	MDColumns         []column            `json:"modeldev_columns"`
	GovColumns        []column            `json:"governance_columns"`
	MDKeys            []foreignKey        `json:"modeldev_foreign_keys"`
	GovKeys           []foreignKey        `json:"governance_foreign_keys"`
	Triggers          []trigger           `json:"modeldev_triggers"`
	TriggerBodySHA256 string              `json:"immutable_gpu_trigger_body_sha256"`
	PrimaryKeys       map[string][]string `json:"modeldev_primary_keys"`
}

//go:embed schema_contract.json
var contractBytes []byte

func fixedContract() schemaContract {
	var v schemaContract
	if json.Unmarshal(contractBytes, &v) != nil {
		panic("compiled schema facts invalid")
	}
	return v
}
