package metadata

// Batas sumber daya (Requirement 1.3 dan 19.1); ditegakkan saat pemuatan skema dan saat evaluasi.
const (
	// MaxConditionNodes adalah jumlah simpul maksimum satu kondisi (grup dan daun).
	MaxConditionNodes = 32
	// MaxConditionDepth adalah kedalaman maksimum kondisi; simpul akar berkedalaman 1.
	MaxConditionDepth = 4
	// MaxInValues adalah jumlah nilai maksimum pada operator in.
	MaxInValues = 100
	// MaxPatternLen adalah panjang maksimum pattern dalam code point.
	MaxPatternLen = 200
	// MaxStringLen adalah panjang maksimum nilai teks yang dievaluasi, dalam satuan UTF-16.
	MaxStringLen = 65536
	// MaxDecimalDigits adalah jumlah digit maksimum nilai desimal masukan (tanpa tanda dan titik).
	MaxDecimalDigits = 64
	// MaxScale adalah skala maksimum hasil aritmetika ekspresi.
	MaxScale = 30
	// MaxExprNodes adalah jumlah simpul maksimum satu ekspresi, termasuk simpul kondisi pada if.
	MaxExprNodes = 32
	// MaxExprDepth adalah kedalaman maksimum simpul ekspresi (kondisi pada if punya batas sendiri).
	MaxExprDepth = 8
	// MaxConcatArgs adalah jumlah argumen maksimum concat dan coalesce.
	MaxConcatArgs = 16
	// MaxArithArgs adalah jumlah argumen maksimum add dan mul.
	MaxArithArgs = 8
	// MaxIdentifierLen adalah panjang maksimum nama field (Requirement 1.4).
	MaxIdentifierLen = 63
	// MaxTemplateLen adalah panjang maksimum templat label dalam satuan UTF-16.
	MaxTemplateLen = 256
	// MaxTemplatePlaceholders adalah jumlah placeholder maksimum pada templat label.
	MaxTemplatePlaceholders = 16
	// MaxRepeat adalah batas atas kuantor terhitung pada pattern ({n}, {n,m}).
	MaxRepeat = 1000
	// ClientPatternInputLimit adalah panjang input maksimum (UTF-16) yang diuji regex di klien; server tidak
	// membatasi di sini karena RE2 berwaktu linear dan panjang field divalidasi terpisah.
	ClientPatternInputLimit = 1024
)
