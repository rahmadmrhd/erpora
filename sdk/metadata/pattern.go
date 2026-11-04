package metadata

import (
	"regexp"
	"sort"
	"unicode/utf8"
)

// Kode galat validator pattern.
const (
	PatternTooLong          = "too_long"
	PatternInvalidSyntax    = "invalid_syntax"
	PatternUnsupported      = "unsupported"
	PatternNestedQuantifier = "nested_quantifier"
	PatternAmbiguous        = "ambiguous_alternation"
	PatternOverlapping      = "overlapping_repeat"
)

// PatternError menjelaskan pelanggaran subset pattern.
type PatternError struct {
	Code string
	Msg  string
}

func (e *PatternError) Error() string { return "metadata: pattern " + e.Code + ": " + e.Msg }

func perr(code, msg string) *PatternError { return &PatternError{Code: code, Msg: msg} }

// ValidatePattern memeriksa bahwa pattern berada dalam subset RE2 yang berperilaku sama di Go dan di
// JavaScript (Requirement 2.5): literal, kelas karakter ([...], \d \D \w \W), titik, jangkar (^ $ \b \B), grup
// (...) dan (?:...), alternasi, serta kuantor * + ? {n} {n,} {n,m} (<= 1000) tanpa varian malas atau posesif.
// Ditolak: lookaround, backreference, grup bernama, flag inline, \s \S \p \P \A \z \Q, kelas POSIX, kuantor
// bersarang (tinggi bintang <= 1: kuantor berulang di dalam operand kuantor berulang), dan alternasi ambigu di
// bawah kuantor berulang (cabang kosong, awal cabang tak dapat dianalisis, atau himpunan karakter awal yang
// tumpang tindih). Panjang <= MaxPatternLen code point.
func ValidatePattern(pattern string) error {
	if !utf8.ValidString(pattern) {
		return perr(PatternInvalidSyntax, "bukan UTF-8 yang sah")
	}
	if utf8.RuneCountInString(pattern) > MaxPatternLen {
		return perr(PatternTooLong, "pattern melebihi 200 karakter")
	}
	p := &patParser{src: []rune(pattern)}
	if _, err := p.parseAlts(); err != nil {
		return err
	}
	if p.pos < len(p.src) { // sisa hanya mungkin ')' tak berpasangan
		return perr(PatternInvalidSyntax, "tanda kurung tutup tanpa pasangan")
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return perr(PatternInvalidSyntax, err.Error())
	}
	return nil
}

// Pattern adalah pola yang sudah divalidasi dan dikompilasi (RE2, waktu linear).
type Pattern struct{ re *regexp.Regexp }

// CompilePattern memvalidasi dan mengompilasi pattern.
func CompilePattern(pattern string) (*Pattern, error) {
	if err := ValidatePattern(pattern); err != nil {
		return nil, err
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, perr(PatternInvalidSyntax, err.Error())
	}
	return &Pattern{re}, nil
}

// Match mencari pola pada input (tidak berjangkar, seperti JSON Schema pattern).
func (p *Pattern) Match(input string) bool { return p.re.MatchString(input) }

// MatchPattern memvalidasi dan mencocokkan sekali pakai; untuk pemakaian berulang gunakan CompilePattern.
func MatchPattern(pattern, input string) (bool, error) {
	p, err := CompilePattern(pattern)
	if err != nil {
		return false, err
	}
	return p.Match(input), nil
}

// ---- parser subset ----

type rng struct{ lo, hi rune }

type rset []rng

const maxRune = 0x10FFFF

func (s rset) normalize() rset {
	if len(s) == 0 {
		return s
	}
	c := append(rset(nil), s...)
	sort.Slice(c, func(i, j int) bool { return c[i].lo < c[j].lo })
	out := rset{c[0]}
	for _, r := range c[1:] {
		last := &out[len(out)-1]
		if r.lo <= last.hi+1 {
			if r.hi > last.hi {
				last.hi = r.hi
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

func (s rset) negate() rset {
	n := s.normalize()
	var out rset
	next := rune(0)
	for _, r := range n {
		if r.lo > next {
			out = append(out, rng{next, r.lo - 1})
		}
		next = r.hi + 1
	}
	if next <= maxRune {
		out = append(out, rng{next, maxRune})
	}
	return out
}

func (s rset) intersects(o rset) bool {
	for _, a := range s {
		for _, b := range o {
			if a.lo <= b.hi && b.lo <= a.hi {
				return true
			}
		}
	}
	return false
}

var (
	digitSet = rset{{'0', '9'}}
	wordSet  = rset{{'0', '9'}, {'A', 'Z'}, {'_', '_'}, {'a', 'z'}}
	anySet   = rset{{0, '\n' - 1}, {'\n' + 1, maxRune}}
)

type pnodeKind uint8

const (
	pnChars  pnodeKind = iota // literal atau kelas: set himpunan karakter
	pnAnchor                  // ^ $ \b \B (tidak dapat dikuantori)
	pnGroup
)

type pnode struct {
	kind pnodeKind
	set  rset
	alts [][]*pelem
	// hasRepeat: ada kuantor berulang di dalam simpul ini (termasuk keturunannya).
	hasRepeat bool
	// hasAmbig: ada grup beralternasi ambigu di dalam simpul ini (termasuk keturunannya).
	hasAmbig bool
}

type pelem struct {
	n        *pnode
	min, max int // max -1 = tak terbatas; hanya bermakna bila quant
	quant    bool
}

// repeating: kuantor berulang variabel (tak terbatas, atau rentang {n,m} dengan m > 1); inilah "bintang" pada
// tinggi bintang.
func (e *pelem) repeating() bool {
	return e.quant && (e.max < 0 || (e.max > 1 && e.min != e.max))
}

// multiplies: kuantor yang mengulang operandnya lebih dari sekali, termasuk {n} tepat (n > 1). Operand yang
// memuat kuantor variabel di bawah pengulang seperti itu berlipat ganda ((?:\d+){1000}).
func (e *pelem) multiplies() bool { return e.quant && (e.max < 0 || e.max > 1) }

// wide: kuantor tak terbatas atau berbatas atas besar; dua yang berurutan dan beririsan membuat backtracking
// polinomial berderajat tinggi.
func (e *pelem) wide() bool { return e.quant && (e.max < 0 || e.max > wideRepeat) }

// wideRepeat adalah batas atas kuantor terhitung yang dianggap "lebar" pada pemeriksaan kuantor bertetangga.
const wideRepeat = 32

type patParser struct {
	src []rune
	pos int
}

func (p *patParser) eof() bool { return p.pos >= len(p.src) }
func (p *patParser) peek() rune {
	if p.eof() {
		return -1
	}
	return p.src[p.pos]
}

// parseAlts menguraikan satu badan grup: seq ('|' seq)*; berhenti pada ')' atau akhir.
func (p *patParser) parseAlts() ([][]*pelem, error) {
	var alts [][]*pelem
	for {
		seq, err := p.parseSeq()
		if err != nil {
			return nil, err
		}
		alts = append(alts, seq)
		if p.peek() == '|' {
			p.pos++
			continue
		}
		return alts, nil
	}
}

func (p *patParser) parseSeq() ([]*pelem, error) {
	var seq []*pelem
	for !p.eof() && p.peek() != '|' && p.peek() != ')' {
		n, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		el := &pelem{n: n}
		if err := p.parseQuant(el); err != nil {
			return nil, err
		}
		if el.quant {
			if n.kind == pnAnchor {
				return nil, perr(PatternInvalidSyntax, "kuantor pada jangkar")
			}
			if el.multiplies() {
				if n.hasRepeat {
					return nil, perr(PatternNestedQuantifier, "kuantor bersarang (tinggi bintang > 1)")
				}
				if n.hasAmbig {
					return nil, perr(PatternAmbiguous, "alternasi ambigu di bawah kuantor")
				}
			}
		}
		seq = append(seq, el)
	}
	if err := checkAdjacentRepeats(seq); err != nil {
		return nil, err
	}
	return seq, nil
}

// checkAdjacentRepeats menolak dua elemen berulang lebar dalam satu urutan yang himpunan karakternya beririsan
// dan hanya dipisahkan elemen opsional (mis. \d*\d*\d*, .*.*=.*, \d+-?\d+): pembagian masukan di antara keduanya
// ambigu sehingga backtracking polinomial. Pemisah wajib di antaranya (\d+-\d+, .+@.+) membatasi ambiguitas.
func checkAdjacentRepeats(seq []*pelem) error {
	for i, a := range seq {
		if !a.wide() {
			continue
		}
		for _, b := range seq[i+1:] {
			if b.wide() && a.n.set.intersects(b.n.set) {
				return perr(PatternOverlapping, "dua pengulangan lebar beririsan bertetangga")
			}
			if !b.quant || b.min > 0 {
				break // elemen wajib memisahkan
			}
		}
	}
	return nil
}

func (p *patParser) parseAtom() (*pnode, error) {
	c := p.src[p.pos]
	switch c {
	case '(':
		p.pos++
		if p.peek() == '?' {
			if p.pos+1 >= len(p.src) {
				return nil, perr(PatternInvalidSyntax, "grup tidak lengkap")
			}
			if p.src[p.pos+1] != ':' {
				return nil, perr(PatternUnsupported, "grup khusus (lookaround, bernama, flag) tidak didukung")
			}
			p.pos += 2
		}
		alts, err := p.parseAlts()
		if err != nil {
			return nil, err
		}
		if p.peek() != ')' {
			return nil, perr(PatternInvalidSyntax, "tanda kurung buka tanpa pasangan")
		}
		p.pos++
		n := &pnode{kind: pnGroup, alts: alts}
		for _, seq := range alts {
			for _, el := range seq {
				if el.repeating() || el.n.hasRepeat {
					n.hasRepeat = true
				}
				if el.n.hasAmbig {
					n.hasAmbig = true
				}
			}
		}
		if len(alts) > 1 && !disjointAlts(alts) {
			n.hasAmbig = true
		}
		var union rset
		for _, seq := range alts {
			for _, el := range seq {
				union = append(union, el.n.set...)
			}
		}
		n.set = union.normalize()
		return n, nil
	case '[':
		return p.parseClass()
	case '.':
		p.pos++
		return &pnode{kind: pnChars, set: anySet}, nil
	case '^', '$':
		p.pos++
		return &pnode{kind: pnAnchor}, nil
	case '\\':
		return p.parseEscape()
	case '*', '+', '?', '{', '}', ']':
		return nil, perr(PatternInvalidSyntax, "karakter tak terduga "+string(c))
	}
	p.pos++
	return &pnode{kind: pnChars, set: rset{{c, c}}}, nil
}

func hexVal(r rune) (rune, bool) {
	switch {
	case r >= '0' && r <= '9':
		return r - '0', true
	case r >= 'a' && r <= 'f':
		return r - 'a' + 10, true
	case r >= 'A' && r <= 'F':
		return r - 'A' + 10, true
	}
	return 0, false
}

const syntaxChars = "^$\\.*+?()[]{}|/"

func isSyntaxChar(r rune) bool {
	for _, s := range syntaxChars {
		if r == s {
			return true
		}
	}
	return false
}

// escapeAtom menguraikan escape setelah '\' (pos menunjuk huruf escape). inClass memengaruhi '-' dan \b.
// Mengembalikan himpunan, apakah escape kelas (\d \w ...), dan galat.
func (p *patParser) escapeAtom(inClass bool) (set rset, classEsc bool, err error) {
	if p.pos >= len(p.src) {
		return nil, false, perr(PatternInvalidSyntax, "garis miring terbalik di akhir")
	}
	c := p.src[p.pos]
	p.pos++
	switch c {
	case 'd':
		return digitSet, true, nil
	case 'D':
		return digitSet.negate(), true, nil
	case 'w':
		return wordSet, true, nil
	case 'W':
		return wordSet.negate(), true, nil
	case 'n':
		return rset{{'\n', '\n'}}, false, nil
	case 'r':
		return rset{{'\r', '\r'}}, false, nil
	case 't':
		return rset{{'\t', '\t'}}, false, nil
	case 'x':
		if p.pos+1 < len(p.src) {
			h1, ok1 := hexVal(p.src[p.pos])
			h2, ok2 := hexVal(p.src[p.pos+1])
			if ok1 && ok2 {
				p.pos += 2
				r := h1*16 + h2
				return rset{{r, r}}, false, nil
			}
		}
		if p.pos < len(p.src) && p.src[p.pos] == '{' {
			return nil, false, perr(PatternUnsupported, "\\x{...} tidak didukung")
		}
		return nil, false, perr(PatternInvalidSyntax, "escape \\x tidak lengkap")
	}
	if isSyntaxChar(c) || (inClass && c == '-') {
		return rset{{c, c}}, false, nil
	}
	return nil, false, perr(PatternUnsupported, "escape \\"+string(c)+" tidak didukung")
}

func (p *patParser) parseEscape() (*pnode, error) {
	p.pos++ // '\'
	if !p.eof() && (p.peek() == 'b' || p.peek() == 'B') {
		p.pos++
		return &pnode{kind: pnAnchor}, nil
	}
	set, _, err := p.escapeAtom(false)
	if err != nil {
		return nil, err
	}
	return &pnode{kind: pnChars, set: set}, nil
}

type classTok struct {
	r        rune
	set      rset
	classEsc bool
	dash     bool // '-' tanpa escape
}

func (p *patParser) parseClass() (*pnode, error) {
	p.pos++ // '['
	negate := false
	if p.peek() == '^' {
		negate = true
		p.pos++
	}
	if p.peek() == ']' {
		return nil, perr(PatternInvalidSyntax, "kelas karakter kosong")
	}
	var toks []classTok
	for {
		if p.eof() {
			return nil, perr(PatternInvalidSyntax, "kelas karakter tidak ditutup")
		}
		c := p.src[p.pos]
		if c == ']' {
			p.pos++
			break
		}
		switch c {
		case '[':
			if p.pos+1 < len(p.src) && p.src[p.pos+1] == ':' {
				return nil, perr(PatternUnsupported, "kelas POSIX tidak didukung")
			}
			return nil, perr(PatternInvalidSyntax, "'[' di dalam kelas karakter harus di-escape")
		case '\\':
			p.pos++
			set, esc, err := p.escapeAtom(true)
			if err != nil {
				return nil, err
			}
			t := classTok{set: set, classEsc: esc}
			if !esc {
				t.r = set[0].lo
			}
			toks = append(toks, t)
		case '-':
			p.pos++
			toks = append(toks, classTok{dash: true, r: '-', set: rset{{'-', '-'}}})
		default:
			p.pos++
			toks = append(toks, classTok{r: c, set: rset{{c, c}}})
		}
	}
	var set rset
	n := len(toks)
	for i := 0; i < n; {
		t := toks[i]
		if t.dash {
			if i == 0 || i == n-1 {
				set = append(set, rng{'-', '-'})
				i++
				continue
			}
			return nil, perr(PatternInvalidSyntax, "tanda minus ambigu di kelas karakter")
		}
		if i+2 < n && toks[i+1].dash {
			hi := toks[i+2]
			if t.classEsc || hi.classEsc || t.r > hi.r {
				return nil, perr(PatternInvalidSyntax, "rentang kelas karakter tidak sah")
			}
			set = append(set, rng{t.r, hi.r})
			i += 3
			continue
		}
		set = append(set, t.set...)
		i++
	}
	set = set.normalize()
	if negate {
		set = set.negate()
	}
	return &pnode{kind: pnChars, set: set}, nil
}

// parseQuant membaca kuantor opsional setelah atom dan mengisi el.
func (p *patParser) parseQuant(el *pelem) error {
	switch p.peek() {
	case '*':
		el.quant, el.min, el.max = true, 0, -1
		p.pos++
	case '+':
		el.quant, el.min, el.max = true, 1, -1
		p.pos++
	case '?':
		el.quant, el.min, el.max = true, 0, 1
		p.pos++
	case '{':
		min, max, ok := p.parseBraces()
		if !ok {
			return perr(PatternInvalidSyntax, "kuantor {…} tidak sah")
		}
		if min > MaxRepeat || max > MaxRepeat || (max >= 0 && max < min) {
			return perr(PatternInvalidSyntax, "batas kuantor tidak sah")
		}
		el.quant, el.min, el.max = true, min, max
	default:
		return nil
	}
	switch p.peek() {
	case '?', '+':
		return perr(PatternUnsupported, "kuantor malas atau posesif tidak didukung")
	case '*', '{':
		return perr(PatternInvalidSyntax, "kuantor bersusun")
	}
	return nil
}

func (p *patParser) parseBraces() (min, max int, ok bool) {
	i := p.pos + 1
	readInt := func() (int, bool) {
		start := i
		v := 0
		for i < len(p.src) && p.src[i] >= '0' && p.src[i] <= '9' {
			if v < 100000 {
				v = v*10 + int(p.src[i]-'0')
			}
			i++
		}
		return v, i > start
	}
	min, ok = readInt()
	if !ok {
		return 0, 0, false
	}
	max = min
	if i < len(p.src) && p.src[i] == ',' {
		i++
		if i < len(p.src) && p.src[i] == '}' {
			max = -1
		} else if max, ok = readInt(); !ok {
			return 0, 0, false
		}
	}
	if i >= len(p.src) || p.src[i] != '}' {
		return 0, 0, false
	}
	p.pos = i + 1
	return min, max, true
}

// disjointAlts: setiap cabang tidak kosong, diawali literal atau kelas yang wajib muncul (kuantor min >= 1),
// dan himpunan karakter awal antar cabang saling lepas.
func disjointAlts(alts [][]*pelem) bool {
	firsts := make([]rset, 0, len(alts))
	for _, seq := range alts {
		if len(seq) == 0 {
			return false
		}
		f := seq[0]
		if f.n.kind != pnChars || (f.quant && f.min < 1) {
			return false
		}
		firsts = append(firsts, f.n.set)
	}
	for i := range firsts {
		for j := i + 1; j < len(firsts); j++ {
			if firsts[i].intersects(firsts[j]) {
				return false
			}
		}
	}
	return true
}
