package metadata

import (
	"math/big"
	"strings"
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

// Decimal adalah bilangan desimal eksak (tanpa float) berbasis math/big: nilai = coef x 10^-scale dengan skala
// yang dipertahankan (12.50 tetap skala 2). Nilai nol Decimal adalah 0 berskala 0. Decimal tidak dapat diubah
// (operasi selalu menghasilkan nilai baru).
type Decimal struct {
	coef  *big.Int
	scale int
}

// c mengembalikan koefisien (0 bila Decimal nol); tidak boleh diubah pemanggil.
func (a Decimal) c() *big.Int {
	if a.coef == nil {
		return new(big.Int)
	}
	return a.coef
}

// DecimalFromInt membuat Decimal berskala 0.
func DecimalFromInt(n int64) Decimal { return Decimal{coef: big.NewInt(n)} }

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
	return Decimal{coef: coef, scale: fracLen}, true
}

// align menyamakan skala kedua operand (tanpa mengubah operand) dan mengembalikan koefisien serta skala bersama.
func align(a, b Decimal) (x, y *big.Int, scale int) {
	scale = max(a.scale, b.scale)
	x, y = a.c(), b.c()
	if a.scale < scale {
		x = new(big.Int).Mul(x, pow10(scale-a.scale))
	}
	if b.scale < scale {
		y = new(big.Int).Mul(y, pow10(scale-b.scale))
	}
	return x, y, scale
}

// Cmp membandingkan nilai (bukan skala): -1, 0, atau 1.
func (a Decimal) Cmp(b Decimal) int {
	x, y, _ := align(a, b)
	return x.Cmp(y)
}

// Add menjumlahkan secara eksak; skala hasil = skala terbesar kedua operand.
func (a Decimal) Add(b Decimal) Decimal {
	x, y, s := align(a, b)
	return Decimal{coef: new(big.Int).Add(x, y), scale: s}
}

// Sub mengurangkan secara eksak; skala hasil = skala terbesar kedua operand.
func (a Decimal) Sub(b Decimal) Decimal {
	x, y, s := align(a, b)
	return Decimal{coef: new(big.Int).Sub(x, y), scale: s}
}

// Mul mengalikan secara eksak; skala hasil = jumlah skala kedua operand.
func (a Decimal) Mul(b Decimal) Decimal {
	return Decimal{coef: new(big.Int).Mul(a.c(), b.c()), scale: a.scale + b.scale}
}

// IsZero melaporkan apakah nilainya nol.
func (a Decimal) IsZero() bool { return a.c().Sign() == 0 }

// IntPart mengembalikan bagian bulat (dipotong ke nol); berguna untuk nilai bertipe integer. Hasil tidak
// bermakna bila bagian bulat melebihi int64.
func (a Decimal) IntPart() int64 {
	q := new(big.Int).Quo(a.c(), pow10(a.scale))
	return q.Int64()
}

// Scale mengembalikan jumlah digit pecahan yang dipertahankan.
func (a Decimal) Scale() int { return a.scale }

// String menulis nilai dengan seluruh digit pecahan yang dipertahankan (12.50 tetap "12.50"); nol tidak
// pernah bertanda minus.
func (a Decimal) String() string {
	c := a.c()
	digits := new(big.Int).Abs(c).String()
	if a.scale > 0 {
		if len(digits) <= a.scale {
			digits = strings.Repeat("0", a.scale-len(digits)+1) + digits
		}
		digits = digits[:len(digits)-a.scale] + "." + digits[len(digits)-a.scale:]
	}
	if c.Sign() < 0 {
		return "-" + digits
	}
	return digits
}

// maxRoundScale membatasi skala hasil Round dan Div (jauh di atas MaxScale; mencegah pow10 raksasa).
const maxRoundScale = 1 << 12

var (
	bigOne = big.NewInt(1)
	bigTen = big.NewInt(10)
)

func pow10(n int) *big.Int { return new(big.Int).Exp(bigTen, big.NewInt(int64(n)), nil) }

// roundQuotient membulatkan num/den (den != 0) ke bilangan bulat menurut mode. mode harus valid (RoundMode.Valid);
// pemanggil publik (Round dan Div) menolak mode tak dikenal sebelum sampai ke sini.
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
		case RoundHalfUp, RoundHalfDown, RoundHalfEven:
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

// Round membulatkan ke scale digit pecahan dengan mode yang diberikan; hasil berskala tepat scale. ok=false
// (tanpa membulatkan) bila mode tidak dikenal atau scale di luar 0..4096: mode tak dikenal tidak pernah
// berperilaku diam-diam sebagai mode lain.
func (a Decimal) Round(scale int, mode RoundMode) (Decimal, bool) {
	if scale < 0 || scale > maxRoundScale || !mode.Valid() {
		return Decimal{}, false
	}
	if a.scale <= scale {
		return Decimal{coef: new(big.Int).Mul(a.c(), pow10(scale-a.scale)), scale: scale}, true
	}
	return Decimal{coef: roundQuotient(a.c(), pow10(a.scale-scale), mode), scale: scale}, true
}

// Div membagi a dengan b dan membulatkan ke scale digit pecahan. ok=false bila b nol, mode tidak dikenal, atau
// scale di luar 0..4096.
func (a Decimal) Div(b Decimal, scale int, mode RoundMode) (Decimal, bool) {
	if b.IsZero() || scale < 0 || scale > maxRoundScale || !mode.Valid() {
		return Decimal{}, false
	}
	num, den := a.c(), b.c()
	if k := b.scale - a.scale + scale; k >= 0 {
		num = new(big.Int).Mul(num, pow10(k))
	} else {
		den = new(big.Int).Mul(den, pow10(-k))
	}
	return Decimal{coef: roundQuotient(num, den, mode), scale: scale}, true
}
