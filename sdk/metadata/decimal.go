package metadata

import (
	"math/big"

	"github.com/shopspring/decimal"
)

// RoundMode adalah mode pembulatan eksplisit (Requirement 5.2). "half_*" menentukan perlakuan nilai tepat di
// tengah: half_up menjauhi nol, half_down mendekati nol, half_even ke digit genap. up menjauhi nol, down
// mendekati nol (pemotongan), floor ke bawah, ceiling ke atas.
type RoundMode string

const (
	RoundHalfUp   RoundMode = "half_up"
	RoundHalfDown RoundMode = "half_down"
	RoundHalfEven RoundMode = "half_even"
	RoundUp       RoundMode = "up"
	RoundDown     RoundMode = "down"
	RoundFloor    RoundMode = "floor"
	RoundCeiling  RoundMode = "ceiling"
)

// Valid melaporkan apakah m adalah mode yang dikenal.
func (m RoundMode) Valid() bool {
	switch m {
	case RoundHalfUp, RoundHalfDown, RoundHalfEven, RoundUp, RoundDown, RoundFloor, RoundCeiling:
		return true
	}
	return false
}

// Decimal adalah bilangan desimal eksak (tanpa float) dengan skala yang dipertahankan. Nilai nol Decimal
// adalah 0 berskala 0.
type Decimal struct{ d decimal.Decimal }

// DecimalFromInt membuat Decimal berskala 0.
func DecimalFromInt(n int64) Decimal { return Decimal{decimal.NewFromInt(n)} }

// ParseDecimal menerima hanya string desimal kanonik: -?(0|[1-9][0-9]*)(\.[0-9]+)? dengan paling banyak
// MaxDecimalDigits digit. Selain itu (eksponen, tanda plus, nol di depan, spasi) ditolak.
func ParseDecimal(s string) (Decimal, bool) {
	n := len(s)
	i := 0
	neg := false
	if i < n && s[i] == '-' {
		neg = true
		i++
	}
	intStart := i
	for i < n && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	intLen := i - intStart
	if intLen == 0 || (intLen > 1 && s[intStart] == '0') {
		return Decimal{}, false
	}
	fracLen := 0
	if i < n && s[i] == '.' {
		i++
		fs := i
		for i < n && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		fracLen = i - fs
		if fracLen == 0 {
			return Decimal{}, false
		}
	}
	if i != n || intLen+fracLen > MaxDecimalDigits {
		return Decimal{}, false
	}
	digits := s[intStart : intStart+intLen]
	if fracLen > 0 {
		digits += s[intStart+intLen+1:]
	}
	coef, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return Decimal{}, false
	}
	if neg {
		coef.Neg(coef)
	}
	return Decimal{decimal.NewFromBigInt(coef, int32(-fracLen))}, true
}

// Cmp membandingkan nilai (bukan skala): -1, 0, atau 1.
func (a Decimal) Cmp(b Decimal) int { return a.d.Cmp(b.d) }

// Add menjumlahkan secara eksak; skala hasil = skala terbesar kedua operand.
func (a Decimal) Add(b Decimal) Decimal { return Decimal{a.d.Add(b.d)} }

// Sub mengurangkan secara eksak; skala hasil = skala terbesar kedua operand.
func (a Decimal) Sub(b Decimal) Decimal { return Decimal{a.d.Sub(b.d)} }

// Mul mengalikan secara eksak; skala hasil = jumlah skala kedua operand.
func (a Decimal) Mul(b Decimal) Decimal { return Decimal{a.d.Mul(b.d)} }

// IsZero melaporkan apakah nilainya nol.
func (a Decimal) IsZero() bool { return a.d.IsZero() }

// IntPart mengembalikan bagian bulat (dipotong ke nol); berguna untuk nilai bertipe integer.
func (a Decimal) IntPart() int64 { return a.d.IntPart() }

// Scale mengembalikan jumlah digit pecahan yang dipertahankan.
func (a Decimal) Scale() int {
	if e := a.d.Exponent(); e < 0 {
		return int(-e)
	}
	return 0
}

// String menulis nilai dengan seluruh digit pecahan yang dipertahankan (12.50 tetap "12.50"); nol tidak
// pernah bertanda minus.
func (a Decimal) String() string {
	if e := a.d.Exponent(); e < 0 {
		return a.d.StringFixed(-e)
	}
	return a.d.StringFixed(0)
}

var (
	bigOne = big.NewInt(1)
	bigTen = big.NewInt(10)
)

func pow10(n int) *big.Int { return new(big.Int).Exp(bigTen, big.NewInt(int64(n)), nil) }

// roundQuotient membulatkan num/den (den != 0) ke bilangan bulat menurut mode.
func roundQuotient(num, den *big.Int, mode RoundMode) *big.Int {
	neg := (num.Sign() < 0) != (den.Sign() < 0)
	n := new(big.Int).Abs(num)
	d := new(big.Int).Abs(den)
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	if r.Sign() != 0 {
		inc := false
		switch mode {
		case RoundUp:
			inc = true
		case RoundDown:
		case RoundFloor:
			inc = neg
		case RoundCeiling:
			inc = !neg
		default: // half_up, half_down, half_even
			switch c := new(big.Int).Lsh(r, 1).Cmp(d); {
			case c > 0:
				inc = true
			case c == 0:
				inc = mode == RoundHalfUp || (mode == RoundHalfEven && q.Bit(0) == 1)
			}
		}
		if inc {
			q.Add(q, bigOne)
		}
	}
	if neg {
		q.Neg(q)
	}
	return q
}

// Round membulatkan ke scale digit pecahan (scale >= 0) dengan mode yang diberikan; hasil berskala tepat
// scale. Mode tak dikenal diperlakukan sebagai half_up.
func (a Decimal) Round(scale int, mode RoundMode) Decimal {
	if scale < 0 {
		scale = 0
	}
	coef := a.d.Coefficient()
	exp := int(a.d.Exponent())
	if -exp <= scale {
		coef.Mul(coef, pow10(scale+exp))
		return Decimal{decimal.NewFromBigInt(coef, int32(-scale))}
	}
	q := roundQuotient(coef, pow10(-exp-scale), mode)
	return Decimal{decimal.NewFromBigInt(q, int32(-scale))}
}

// Div membagi a dengan b dan membulatkan ke scale digit pecahan. ok=false bila b nol.
func (a Decimal) Div(b Decimal, scale int, mode RoundMode) (Decimal, bool) {
	if b.IsZero() {
		return Decimal{}, false
	}
	if scale < 0 {
		scale = 0
	}
	ca, cb := a.d.Coefficient(), b.d.Coefficient()
	k := int(a.d.Exponent()) + scale - int(b.d.Exponent())
	num, den := ca, cb
	if k >= 0 {
		num.Mul(num, pow10(k))
	} else {
		den.Mul(den, pow10(-k))
	}
	return Decimal{decimal.NewFromBigInt(roundQuotient(num, den, mode), int32(-scale))}, true
}
