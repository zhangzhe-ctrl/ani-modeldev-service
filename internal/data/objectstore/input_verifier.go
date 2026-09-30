package objectstore

import (
	"context"
	"encoding/csv"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/zhangzhe-ctrl/ani-modeldev-service/contract/cpup01"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/biz"
)

// VerifyCSV verifies the exact version's bytes and fixed CPU input structure.
// Import authorization, version fixation and durable READY are separate steps.
func (v *Verifier) VerifyCSV(ctx context.Context, scope cpup01.StorageScope, object cpup01.FixedObjectRef) (biz.VerifiedCSV, error) {
	if err := ctx.Err(); err != nil {
		return biz.VerifiedCSV{}, err
	}
	if object.SizeBytes <= 0 || object.SizeBytes > 32*1024*1024 {
		return biz.VerifiedCSV{}, biz.ErrInputVerification
	}
	verified, err := v.verify(ctx, scope, object, inspectCPUCSV)
	if err != nil {
		if ctx.Err() != nil {
			return biz.VerifiedCSV{}, ctx.Err()
		}
		return biz.VerifiedCSV{}, biz.ErrInputVerification
	}
	return biz.VerifiedCSV{VerifiedObject: verified, SchemaVersion: "ani.cpu.csv.v1", RowCount: 1024, FeatureCount: 16}, nil
}

func inspectCPUCSV(stream io.Reader) error {
	reader := csv.NewReader(stream)
	reader.FieldsPerRecord = 17
	reader.ReuseRecord = true
	header, err := reader.Read()
	if err != nil || len(header) != 17 || header[16] != "label" {
		return biz.ErrInputVerification
	}
	for column := 0; column < 16; column++ {
		if header[column] != "x"+strconv.Itoa(column) {
			return biz.ErrInputVerification
		}
	}
	rows := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || rows >= 1024 || (row[16] != "0" && row[16] != "1") {
			return biz.ErrInputVerification
		}
		for _, raw := range row[:16] {
			value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > math.MaxFloat32 {
				return biz.ErrInputVerification
			}
		}
		rows++
	}
	if rows != 1024 {
		return biz.ErrInputVerification
	}
	return nil
}
