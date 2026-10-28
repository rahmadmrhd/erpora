package i18n

import (
	"fmt"
	"strings"

	"golang.org/x/text/feature/plural"
)

// Simpul AST pesan.
type node interface{ isNode() }

type (
	textNode  string
	poundNode struct{}
	argNode   struct{ name string }
	tagNode   struct {
		name string
		body []node
	}
	blockNode struct {
		name   string
		plural bool
		opts   []option
	}
)

func (textNode) isNode()  {}
func (poundNode) isNode() {}
func (argNode) isNode()   {}
func (tagNode) isNode()   {}
func (blockNode) isNode() {}

// option adalah satu cabang plural/select. key menyimpan teks aslinya ("one", "=0", "male"); exact dan
// cat hanya bermakna pada plural; pound menandai cabang yang memuat '#' langsung sehingga angkanya
// perlu diformat.
type option struct {
	key   string
	exact bool
	cat   plural.Form
	body  []node
	pound bool
}

var pluralCats = map[string]plural.Form{
	"zero": plural.Zero, "one": plural.One, "two": plural.Two,
	"few": plural.Few, "many": plural.Many, "other": plural.Other,
}

// parser mengurai pesan ICU. Tata bahasanya mengikuti @formatjs/icu-messageformat-parser (kutip
// apostrof, '}' liar di tingkat atas, '<' yang bukan tag, tag self-closing) dengan tambahan batas
// Limits dan penolakan konstruksi yang tidak didukung.
type parser struct {
	src []rune
	pos int
	lim Limits

	paramIdx map[string]int
	params   []Param
	argNames map[string]struct{}
	html     string // alasan markup non-rich pertama
}

func (p *parser) eof() bool { return p.pos >= len(p.src) }

func (p *parser) cur() rune {
	if p.eof() {
		return 0
	}
	return p.src[p.pos]
}

func (p *parser) peek() rune {
	if p.pos+1 >= len(p.src) {
		return 0
	}
	return p.src[p.pos+1]
}

func (p *parser) fail(code, format string, a ...any) error {
	return &CompileError{Code: code, Pos: p.pos, Detail: fmt.Sprintf(format, a...)}
}

func (p *parser) invalid(format string, a ...any) error {
	return p.fail(CodeInvalidICU, format, a...)
}

func (p *parser) markHTML(reason string) {
	if p.html == "" {
		p.html = fmt.Sprintf("posisi %d: %s", p.pos, reason)
	}
}

func (p *parser) skipSpace() {
	for !p.eof() && isPatternWhiteSpace(p.src[p.pos]) {
		p.pos++
	}
}

// ident membaca pengenal: karakter yang bukan spasi pola dan bukan sintaks pola.
func (p *parser) ident() string {
	start := p.pos
	for !p.eof() && isIdentRune(p.src[p.pos]) {
		p.pos++
	}
	return string(p.src[start:p.pos])
}

func (p *parser) bumpIf(s string) bool {
	rs := []rune(s)
	if p.pos+len(rs) > len(p.src) {
		return false
	}
	for i, r := range rs {
		if p.src[p.pos+i] != r {
			return false
		}
	}
	p.pos += len(rs)
	return true
}

// touch mencatat penggunaan argumen atau tag dan menegakkan MaxArgs (nama berbeda).
func (p *parser) touch(name string, kind ParamKind, options []string) error {
	key := name
	if kind == KindTag {
		key = "<" + name
	} else if _, ok := p.argNames[name]; !ok {
		if len(p.argNames) >= p.lim.MaxArgs {
			return p.fail(CodeTooLarge, "argumen berbeda melebihi %d", p.lim.MaxArgs)
		}
		p.argNames[name] = struct{}{}
	}
	i, ok := p.paramIdx[key]
	if !ok {
		p.paramIdx[key] = len(p.params)
		p.params = append(p.params, Param{Name: name, Kind: kind, Options: options})
		return nil
	}
	cur := &p.params[i]
	if cur.Kind == KindArg && kind != KindArg {
		cur.Kind = kind
	}
	if kind == KindSelect {
		for _, o := range options {
			found := false
			for _, e := range cur.Options {
				if e == o {
					found = true
					break
				}
			}
			if !found {
				cur.Options = append(cur.Options, o)
			}
		}
	}
	return nil
}

// parseMessage membaca teks dan elemen sampai akhir pesan, '}' penutup cabang (level > 0, tidak
// dikonsumsi), atau '</' penutup tag (expectClose, tidak dikonsumsi). level adalah kedalaman
// sarang; inPlural menandai bahwa '#' adalah angka plural.
func (p *parser) parseMessage(level int, inPlural, expectClose bool) ([]node, error) {
	var out []node
	for !p.eof() {
		c := p.src[p.pos]
		switch {
		case c == '{':
			n, err := p.parseArgument(level, inPlural)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		case c == '}' && level > 0:
			return out, nil
		case c == '#' && inPlural:
			out = append(out, poundNode{})
			p.pos++
		case c == '<' && p.peek() == '/':
			if expectClose {
				return out, nil
			}
			return nil, p.invalid("tag penutup tanpa tag pembuka")
		case c == '<' && isAlpha(p.peek()):
			n, err := p.parseTag(level, inPlural)
			if err != nil {
				return nil, err
			}
			out = append(out, n)
		default:
			out = append(out, textNode(p.parseLiteral(level, inPlural)))
		}
	}
	return out, nil
}

// parseLiteral membaca teks biasa dengan aturan kutip ICU.
func (p *parser) parseLiteral(level int, inPlural bool) string {
	var sb strings.Builder
	for !p.eof() {
		c := p.src[p.pos]
		if c == '\'' {
			if s, ok := p.tryQuote(inPlural); ok {
				sb.WriteString(s)
				continue
			}
		}
		if c == '{' || (c == '#' && inPlural) || (c == '}' && level > 0) {
			break
		}
		if c == '<' {
			if n := p.peek(); isAlpha(n) || n == '/' {
				break
			}
			p.markHTML("karakter '<' yang bukan tag rich")
		}
		sb.WriteRune(c)
		p.pos++
	}
	return sb.String()
}

// tryQuote membaca teks berkutip ICU di p.pos (yang berisi apostrof). Apostrof hanya membuka kutip
// bila diikuti '{', '}', '<', '>' (atau '#' di dalam plural); ” menjadi satu apostrof.
func (p *parser) tryQuote(inPlural bool) (string, bool) {
	switch p.peek() {
	case '\'':
		p.pos += 2
		return "'", true
	case '{', '<', '>', '}':
	case '#':
		if !inPlural {
			return "", false
		}
	default:
		return "", false
	}
	p.pos++ // apostrof pembuka
	out := []rune{p.src[p.pos]}
	p.pos++
	for !p.eof() {
		ch := p.src[p.pos]
		if ch == '\'' {
			if p.peek() == '\'' {
				out = append(out, '\'')
				p.pos++
			} else {
				p.pos++
				break
			}
		} else {
			out = append(out, ch)
		}
		p.pos++
	}
	return string(out), true
}

func (p *parser) parseTag(level int, inPlural bool) (node, error) {
	if level+1 > p.lim.MaxDepth {
		return nil, p.fail(CodeTooLarge, "kedalaman bersarang melebihi %d", p.lim.MaxDepth)
	}
	p.pos++ // '<'
	start := p.pos
	p.pos++ // huruf pertama
	for !p.eof() && isTagNameRune(p.src[p.pos]) {
		p.pos++
	}
	name := string(p.src[start:p.pos])
	spaced := p.pos
	p.skipSpace()
	spaced = p.pos - spaced
	if p.bumpIf("/>") {
		p.markHTML(fmt.Sprintf("tag <%s/> bukan tag rich yang sah", quoteForDetail(name)))
		return textNode("<" + name + "/>"), nil
	}
	if !p.bumpIf(">") {
		return nil, p.invalid("tag <%s> tidak valid (hanya <nama> dan </nama> tanpa atribut)", quoteForDetail(name))
	}
	if !simpleTagName(name) {
		p.markHTML(fmt.Sprintf("tag <%s> bukan tag rich yang sah (nama harus [a-z][a-z0-9]*)", quoteForDetail(name)))
	} else if _, bad := htmlDenylist[name]; bad {
		p.markHTML(fmt.Sprintf("elemen HTML <%s> tidak boleh dipakai", name))
	}
	if spaced > 0 {
		p.markHTML(fmt.Sprintf("tag <%s> memuat spasi", name))
	}
	if err := p.touch(name, KindTag, nil); err != nil {
		return nil, err
	}
	body, err := p.parseMessage(level+1, inPlural, true)
	if err != nil {
		return nil, err
	}
	if !p.bumpIf("</") {
		return nil, p.invalid("tag <%s> tidak ditutup", name)
	}
	if p.eof() || !isAlpha(p.cur()) {
		return nil, p.invalid("tag penutup tidak valid")
	}
	cs := p.pos
	p.pos++
	for !p.eof() && isTagNameRune(p.src[p.pos]) {
		p.pos++
	}
	if closing := string(p.src[cs:p.pos]); closing != name {
		return nil, p.invalid("tag penutup </%s> tidak cocok dengan <%s>", quoteForDetail(closing), name)
	}
	before := p.pos
	p.skipSpace()
	if p.pos != before {
		p.markHTML(fmt.Sprintf("tag </%s> memuat spasi", name))
	}
	if !p.bumpIf(">") {
		return nil, p.invalid("tag penutup </%s> tidak valid", name)
	}
	return tagNode{name: name, body: body}, nil
}

func (p *parser) parseArgument(level int, inPlural bool) (node, error) {
	p.pos++ // '{'
	p.skipSpace()
	if p.eof() {
		return nil, p.invalid("kurung '{' tidak ditutup")
	}
	if p.cur() == '}' {
		return nil, p.invalid("argumen kosong {}")
	}
	name := p.ident()
	if name == "" {
		return nil, p.invalid("nama argumen tidak valid")
	}
	p.skipSpace()
	if p.eof() {
		return nil, p.invalid("kurung '{' tidak ditutup")
	}
	switch p.cur() {
	case '}':
		p.pos++
		if err := p.touch(name, KindArg, nil); err != nil {
			return nil, err
		}
		return argNode{name: name}, nil
	case ',':
		p.pos++
		p.skipSpace()
		if p.eof() {
			return nil, p.invalid("kurung '{' tidak ditutup")
		}
		return p.parseBlock(name, level)
	}
	return nil, p.invalid("setelah nama argumen %q diharapkan ',' atau '}'", quoteForDetail(name))
}

func (p *parser) parseBlock(name string, level int) (node, error) {
	typ := p.ident()
	p.skipSpace()
	switch typ {
	case "":
		return nil, p.invalid("tipe argumen hilang")
	case "plural", "select":
	case "number", "date", "time", "selectordinal":
		return nil, p.fail(CodeUnsupportedConstruct, "tipe argumen %q tidak didukung (hanya argumen, plural, select)", typ)
	default:
		return nil, p.invalid("tipe argumen %q tidak dikenal", quoteForDetail(typ))
	}
	isPlural := typ == "plural"
	if level+1 > p.lim.MaxDepth {
		return nil, p.fail(CodeTooLarge, "kedalaman bersarang melebihi %d", p.lim.MaxDepth)
	}
	if !p.bumpIf(",") {
		return nil, p.invalid("setelah %s diharapkan ','", typ)
	}
	p.skipSpace()
	sel := p.ident()
	if isPlural && sel == "offset" {
		return nil, p.fail(CodeUnsupportedConstruct, "offset pada plural tidak didukung")
	}
	kind := KindSelect
	if isPlural {
		kind = KindPlural
	}
	// Dicatat sebelum cabang diurai agar urutan Params mengikuti kemunculan pertama.
	if err := p.touch(name, kind, nil); err != nil {
		return nil, err
	}
	blk := blockNode{name: name, plural: isPlural}
	seen := map[string]struct{}{}
	hasOther := false
	for {
		o := option{}
		if sel == "" {
			if !isPlural || p.cur() != '=' {
				break
			}
			start := p.pos
			p.pos++
			if p.cur() == '+' || p.cur() == '-' {
				p.pos++
			}
			digits := p.pos
			for !p.eof() && p.cur() >= '0' && p.cur() <= '9' {
				p.pos++
			}
			if p.pos == digits {
				return nil, p.invalid("setelah '=' diharapkan bilangan bulat")
			}
			sel = string(p.src[start:p.pos])
			o.exact = true
		} else if isPlural {
			cat, ok := pluralCats[sel]
			if !ok {
				return nil, p.invalid("kategori plural %q tidak dikenal (zero, one, two, few, many, other, atau =N)", quoteForDetail(sel))
			}
			o.cat = cat
		}
		if _, dup := seen[sel]; dup {
			return nil, p.invalid("kunci ganda %q pada %s", quoteForDetail(sel), typ)
		}
		seen[sel] = struct{}{}
		if sel == "other" {
			hasOther = true
		}
		o.key = sel
		p.skipSpace()
		if p.cur() != '{' || p.eof() {
			return nil, p.invalid("setelah kunci %q diharapkan '{'", quoteForDetail(sel))
		}
		p.pos++
		body, err := p.parseMessage(level+1, isPlural, false)
		if err != nil {
			return nil, err
		}
		if p.eof() || p.cur() != '}' {
			return nil, p.invalid("kurung '{' tidak ditutup")
		}
		p.pos++
		o.body = body
		o.pound = isPlural && hasPound(body)
		blk.opts = append(blk.opts, o)
		if len(blk.opts) > p.lim.MaxOptions {
			return nil, p.fail(CodeTooLarge, "opsi pada %s melebihi %d", typ, p.lim.MaxOptions)
		}
		p.skipSpace()
		sel = p.ident()
	}
	if len(blk.opts) == 0 {
		return nil, p.invalid("%s tanpa opsi", typ)
	}
	if !hasOther {
		return nil, p.invalid("opsi 'other' wajib ada pada %s", typ)
	}
	if p.eof() || p.cur() != '}' {
		return nil, p.invalid("kurung '{' tidak ditutup")
	}
	p.pos++
	if !isPlural {
		optNames := make([]string, 0, len(blk.opts))
		for _, o := range blk.opts {
			optNames = append(optNames, o.key)
		}
		if err := p.touch(name, kind, optNames); err != nil {
			return nil, err
		}
	}
	return blk, nil
}

// hasPound melaporkan apakah body memuat '#' milik plural yang sedang diurai (menembus tag, tidak
// menembus blok bersarang yang punya '#' sendiri atau select yang menjadikannya literal).
func hasPound(body []node) bool {
	for _, n := range body {
		switch x := n.(type) {
		case poundNode:
			return true
		case tagNode:
			if hasPound(x.body) {
				return true
			}
		}
	}
	return false
}
