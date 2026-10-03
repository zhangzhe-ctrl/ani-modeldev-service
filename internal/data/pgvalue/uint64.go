// Package pgvalue converts PostgreSQL values without losing their precision.
package pgvalue

import (
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Uint64 accepts only finite, nonnegative integers in the full uint64 range.
// Decimal scale is permitted when all fractional digits are zero. It neither
// rounds the value nor mutates the driver's coefficient. Callers decide whether
// zero is valid for the particular persisted fact.
func Uint64(value pgtype.Numeric) (uint64, bool) {
	if !value.Valid || value.NaN || value.InfinityModifier != pgtype.Finite || value.Int == nil || value.Int.Sign() < 0 {
		return 0, false
	}
	if value.Int.Sign() == 0 {
		return 0, true
	}
	digits := value.Int.String()
	switch {
	case value.Exp > 0:
		// A positive uint64 has at most 20 decimal digits. Check the bound
		// before allocating any padding, even for a hostile exponent.
		if value.Exp > 19 || len(digits) > 20-int(value.Exp) {
			return 0, false
		}
		digits += strings.Repeat("0", int(value.Exp))
	case value.Exp < 0:
		// Widen before negation so MinInt32 cannot overflow. A positive
		// coefficient shorter than its scale cannot be an integer.
		scale := -int64(value.Exp)
		if scale >= int64(len(digits)) {
			return 0, false
		}
		integerEnd := len(digits) - int(scale)
		for _, digit := range digits[integerEnd:] {
			if digit != '0' {
				return 0, false
			}
		}
		digits = digits[:integerEnd]
	}
	result, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return 0, false
	}
	return result, true
}
