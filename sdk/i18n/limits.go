package i18n

// Limits membatasi sumber daya yang boleh dipakai satu pesan atau satu berkas katalog
// (Requirement 2.5, 10.4). Seluruh bidang wajib positif; nilai nol atau negatif diganti nilai bawaan
// oleh normalize sehingga Limits{} tetap aman dipakai.
type Limits struct {
	// MaxMessageLen adalah panjang maksimal satu pesan dalam karakter (rune).
	MaxMessageLen int
	// MaxDepth adalah kedalaman maksimal blok plural/select dan tag rich yang bersarang.
	MaxDepth int
	// MaxOptions adalah jumlah maksimal opsi (cabang) pada satu blok plural atau select.
	MaxOptions int
	// MaxArgs adalah jumlah maksimal NAMA argumen yang berbeda dalam satu pesan (nama yang diulang
	// dihitung satu); nama tag rich tidak dihitung.
	MaxArgs int
	// MaxKeys adalah jumlah maksimal kunci (daun) pada satu katalog namespace-bahasa.
	MaxKeys int
	// MaxFileBytes adalah ukuran maksimal satu berkas katalog dalam byte.
	MaxFileBytes int
	// MaxKeyLen adalah panjang maksimal kunci lengkap hasil perataan (termasuk titik).
	MaxKeyLen int
}

// MaxFormatOutput adalah panjang maksimal (byte) keluaran Format. Nilai argumen berasal dari pemanggil
// dan tidak dibatasi oleh Limits, sehingga satu pesan yang mengulang argumen besar dapat membesar;
// Format menolak keluaran yang melebihi batas ini (Correctness Property P4).
const MaxFormatOutput = 1 << 20

// DefaultLimits mengembalikan batas bawaan platform: pesan 2000 karakter, kedalaman 8, opsi 20,
// argumen 32, kunci 5000 per katalog, berkas 1 MB, dan kunci lengkap 200 karakter.
func DefaultLimits() Limits {
	return Limits{
		MaxMessageLen: 2000,
		MaxDepth:      8,
		MaxOptions:    20,
		MaxArgs:       32,
		MaxKeys:       5000,
		MaxFileBytes:  1 << 20,
		MaxKeyLen:     200,
	}
}

// normalize mengganti bidang yang tidak positif dengan nilai bawaan.
func (l Limits) normalize() Limits {
	d := DefaultLimits()
	if l.MaxMessageLen <= 0 {
		l.MaxMessageLen = d.MaxMessageLen
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = d.MaxDepth
	}
	if l.MaxOptions <= 0 {
		l.MaxOptions = d.MaxOptions
	}
	if l.MaxArgs <= 0 {
		l.MaxArgs = d.MaxArgs
	}
	if l.MaxKeys <= 0 {
		l.MaxKeys = d.MaxKeys
	}
	if l.MaxFileBytes <= 0 {
		l.MaxFileBytes = d.MaxFileBytes
	}
	if l.MaxKeyLen <= 0 {
		l.MaxKeyLen = d.MaxKeyLen
	}
	return l
}

// Kode Issue. Seluruhnya galat struktural: katalog yang memuatnya tidak boleh dimuat (design.md
// "Error Handling"). Kode di luar daftar Requirement 8.1 hanya UnsupportedConstruct, yang dipakai
// untuk sintaks ICU yang sah tetapi sengaja tidak didukung (selectordinal, number, date, offset).
const (
	CodeInvalidJSON          = "invalid_json"
	CodeDuplicateKey         = "duplicate_key"
	CodeFlattenCollision     = "flatten_collision"
	CodeInvalidSegment       = "invalid_segment"
	CodeInvalidICU           = "invalid_icu"
	CodeHTMLNotAllowed       = "html_not_allowed"
	CodeTooLarge             = "too_large"
	CodeUnsupportedConstruct = "unsupported_construct"
)

// Issue adalah satu temuan struktural pada katalog atau pesan. File, Locale, dan Key diisi pemanggil
// bila konteksnya diketahui (ParseCatalog hanya mengisi Key); Detail dalam Bahasa Indonesia dan tidak
// pernah memuat nilai pesan secara utuh.
type Issue struct {
	File   string
	Locale string
	Key    string
	Code   string
	Detail string
}

// Error mengimplementasi error sehingga Issue dapat dibungkus errors.Join oleh pemanggil.
func (i Issue) Error() string {
	s := i.Code
	if i.File != "" {
		s += " " + i.File
	}
	if i.Key != "" {
		s += " [" + i.Key + "]"
	}
	if i.Detail != "" {
		s += ": " + i.Detail
	}
	return s
}
