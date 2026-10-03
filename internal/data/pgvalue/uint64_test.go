package pgvalue_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zhangzhe-ctrl/ani-modeldev-service/internal/data/pgvalue"
)

func TestUint64PreservesExactIntegerValueAndRejectsLossyConversion(t *testing.T) {
	tests := []struct {
		name string
		coefficient string
		exponent int32
		want uint64
		valid bool
	}{
		{"zero", "0", 0, 0, true},
		{"scaled zero", "0", -2, 0, true},
		{"zero with minimum exponent", "0", -1 << 31, 0, true},
		{"zero with maximum exponent", "0", 1<<31 - 1, 0, true},
		{"one", "1", 0, 1, true},
		{"one with decimal scale", "100", -2, 1, true},
		{"large coefficient with valid scale", "1" + strings.Repeat("0", 100), -100, 1, true},
		{"max int64", "9223372036854775807", 0, 9223372036854775807, true},
		{"above int64", "9223372036854775808", 0, 9223372036854775808, true},
		{"max uint64", "18446744073709551615", 0, 18446744073709551615, true},
		{"scaled max uint64", "1844674407370955161500", -2, 18446744073709551615, true},
		{"positive exponent", "1844674407370955161", 1, 18446744073709551610, true},
		{"max uint64 plus one", "18446744073709551616", 0, 0, false},
		{"overflow after positive exponent", "1844674407370955162", 1, 0, false},
		{"overflow after exact scale removal", "1844674407370955161600", -2, 0, false},
		{"negative one", "-1", 0, 0, false},
		{"min int64", "-9223372036854775808", 0, 0, false},
		{"negative scaled integer", "-100", -2, 0, false},
		{"fraction", "15", -1, 0, false},
		{"fraction with trailing zero", "150", -2, 0, false},
		{"fraction below one", "10", -2, 0, false},
		{"fraction above max uint64", "184467440737095516151", -1, 0, false},
		{"nonzero with minimum exponent", "1", -1 << 31, 0, false},
		{"nonzero with maximum exponent", "1", 1<<31 - 1, 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coefficient, ok := new(big.Int).SetString(test.coefficient, 10)
			if !ok {
				t.Fatal("invalid decimal test fixture")
			}
			original := new(big.Int).Set(coefficient)
			got, valid := pgvalue.Uint64(pgtype.Numeric{Int: coefficient, Exp: test.exponent, Valid: true})
			if valid != test.valid || got != test.want {
				t.Fatalf("conversion returned (%d, %t), want (%d, %t)", got, valid, test.want, test.valid)
			}
			if coefficient.Cmp(original) != 0 {
				t.Fatal("conversion mutated the caller's numeric coefficient")
			}
		})
	}
}

func TestUint64RejectsMissingAndNonfiniteNumericValues(t *testing.T) {
	tests := []struct {
		name string
		value pgtype.Numeric
	}{
		{"null", pgtype.Numeric{}},
		{"invalid with coefficient", pgtype.Numeric{Int: big.NewInt(1)}},
		{"missing coefficient", pgtype.Numeric{Valid: true}},
		{"NaN", pgtype.Numeric{NaN: true, Valid: true}},
		{"NaN with coefficient", pgtype.Numeric{Int: big.NewInt(1), NaN: true, Valid: true}},
		{"positive infinity", pgtype.Numeric{InfinityModifier: pgtype.Infinity, Valid: true}},
		{"negative infinity", pgtype.Numeric{InfinityModifier: pgtype.NegativeInfinity, Valid: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, valid := pgvalue.Uint64(test.value); valid || got != 0 {
				t.Fatalf("missing or nonfinite value returned (%d, %t)", got, valid)
			}
		})
	}
}
