package i18n

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Flat adalah katalog satu namespace dan bahasa: kunci lengkap bertitik menjadi sumber pesan.
type Flat map[string]string

// maxIssues membatasi jumlah Issue yang dikumpulkan agar berkas rusak parah tidak menghabiskan memori.
const maxIssues = 100

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ParseCatalog membaca satu berkas katalog JSON bersarang dan meratakannya (Requirement 1.4-1.7).
// Pembaca token dipakai agar kunci ganda pada tingkat yang sama dilaporkan (encoding/json biasa
// membuangnya diam-diam). Setiap nilai daun divalidasi lewat ValidateMessage (ICU sah, tanpa HTML,
// dalam batas). Bila ada Issue, Flat yang dikembalikan nil dan tidak boleh dipakai; Issue.Key berisi
// kunci lengkap (atau jalur induk) dan File/Locale diisi pemanggil. Tidak ada I/O selain membaca r,
// paling banyak lim.MaxFileBytes+1 byte.
func ParseCatalog(r io.Reader, lim Limits) (Flat, []Issue) {
	lim = lim.normalize()
	data, err := io.ReadAll(io.LimitReader(r, int64(lim.MaxFileBytes)+1))
	if err != nil {
		return nil, []Issue{{Code: CodeInvalidJSON, Detail: "gagal membaca berkas: " + err.Error()}}
	}
	if len(data) > lim.MaxFileBytes {
		return nil, []Issue{{Code: CodeTooLarge, Detail: fmt.Sprintf("berkas melebihi %d byte", lim.MaxFileBytes)}}
	}
	data = bytes.TrimPrefix(data, utf8BOM)
	c := &catalogParser{lim: lim, flat: Flat{}, origin: map[string]string{}}
	c.run(json.NewDecoder(bytes.NewReader(data)))
	if len(c.issues) > 0 {
		return nil, c.issues
	}
	return c.flat, nil
}

// frame adalah satu objek JSON yang sedang dibaca.
type frame struct {
	segs    []string            // jalur segmen menuju objek ini
	prefix  string              // jalur dipratakan menuju objek ini ("" untuk akar)
	seen    map[string]struct{} // kunci yang sudah dibaca pada objek ini
	invalid bool                // ada segmen tidak sah di atasnya: daun tidak dicatat
}

type catalogParser struct {
	lim    Limits
	flat   Flat
	origin map[string]string // kunci lengkap -> jalur asal, untuk deteksi tabrakan
	issues []Issue
}

// add mencatat Issue; false berarti pembacaan harus dihentikan (terlalu banyak Issue).
func (c *catalogParser) add(i Issue) bool {
	if len(c.issues) >= maxIssues {
		return false
	}
	c.issues = append(c.issues, i)
	if len(c.issues) == maxIssues {
		c.issues = append(c.issues, Issue{Code: CodeTooLarge, Detail: fmt.Sprintf("lebih dari %d galat; pemeriksaan dihentikan", maxIssues)})
		return false
	}
	return true
}

func (c *catalogParser) fatal(code, key, detail string) {
	c.add(Issue{Code: code, Key: key, Detail: detail})
}

func (c *catalogParser) run(dec *json.Decoder) {
	tok, err := dec.Token()
	if errors.Is(err, io.EOF) {
		c.fatal(CodeInvalidJSON, "", "berkas kosong")
		return
	}
	if err != nil {
		c.fatal(CodeInvalidJSON, "", "JSON tidak valid: "+err.Error())
		return
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		c.fatal(CodeInvalidJSON, "", "akar katalog harus berupa objek JSON")
		return
	}
	stack := []*frame{{seen: map[string]struct{}{}}}
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		tok, err := dec.Token()
		if err != nil {
			c.fatal(CodeInvalidJSON, top.prefix, "JSON tidak valid: "+err.Error())
			return
		}
		if d, ok := tok.(json.Delim); ok {
			if d != '}' {
				c.fatal(CodeInvalidJSON, top.prefix, "token tak terduga")
				return
			}
			stack = stack[:len(stack)-1]
			continue
		}
		seg, ok := tok.(string)
		if !ok {
			c.fatal(CodeInvalidJSON, top.prefix, "kunci objek harus berupa string")
			return
		}
		next, fatal := c.entry(dec, top, seg)
		if fatal {
			return
		}
		if next != nil {
			stack = append(stack, next)
		}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		c.fatal(CodeInvalidJSON, "", "ada data sesudah objek akar")
	}
}

// entry memproses satu pasangan kunci-nilai pada top. Mengembalikan frame baru bila nilainya objek.
func (c *catalogParser) entry(dec *json.Decoder, top *frame, seg string) (next *frame, fatal bool) {
	flatKey := seg
	if top.prefix != "" {
		flatKey = top.prefix + "." + seg
	}
	invalid := top.invalid
	if _, dup := top.seen[seg]; dup {
		invalid = true
		if !c.add(Issue{Code: CodeDuplicateKey, Key: flatKey, Detail: fmt.Sprintf("kunci %s muncul lebih dari sekali pada %s", quoteForDetail(seg), pathString(top.segs))}) {
			return nil, true
		}
	}
	top.seen[seg] = struct{}{}
	if why := segmentProblem(seg); why != "" {
		invalid = true
		if !c.add(Issue{Code: CodeInvalidSegment, Key: flatKey, Detail: why}) {
			return nil, true
		}
	}
	if len(flatKey) > c.lim.MaxKeyLen {
		c.fatal(CodeTooLarge, quoteForDetail(flatKey), fmt.Sprintf("kunci lengkap melebihi %d karakter", c.lim.MaxKeyLen))
		return nil, true
	}
	tok, err := dec.Token()
	if err != nil {
		c.fatal(CodeInvalidJSON, flatKey, "JSON tidak valid: "+err.Error())
		return nil, true
	}
	segs := append(append([]string(nil), top.segs...), seg)
	switch v := tok.(type) {
	case json.Delim:
		if v != '{' {
			c.fatal(CodeInvalidJSON, flatKey, "nilai harus berupa string atau objek, bukan array")
			return nil, true
		}
		return &frame{segs: segs, prefix: flatKey, seen: map[string]struct{}{}, invalid: invalid}, false
	case string:
		if invalid {
			return nil, false
		}
		return nil, !c.leaf(flatKey, segs, v)
	}
	c.fatal(CodeInvalidJSON, flatKey, "nilai harus berupa string atau objek")
	return nil, true
}

// leaf mencatat satu pesan; false berarti pembacaan harus dihentikan.
func (c *catalogParser) leaf(flatKey string, segs []string, msg string) bool {
	path := pathString(segs)
	if prev, dup := c.origin[flatKey]; dup {
		return c.add(Issue{Code: CodeFlattenCollision, Key: flatKey,
			Detail: fmt.Sprintf("kunci %s dihasilkan oleh dua jalur: %s dan %s", flatKey, prev, path)})
	}
	if len(c.origin) >= c.lim.MaxKeys {
		c.fatal(CodeTooLarge, "", fmt.Sprintf("kunci melebihi %d per katalog", c.lim.MaxKeys))
		return false
	}
	c.origin[flatKey] = path
	if _, err := ValidateMessage(msg, c.lim); err != nil {
		var ce *CompileError
		if errors.As(err, &ce) {
			iss := ce.Issue()
			iss.Key = flatKey
			return c.add(iss)
		}
		return c.add(Issue{Code: CodeInvalidICU, Key: flatKey, Detail: err.Error()})
	}
	c.flat[flatKey] = msg
	return true
}
