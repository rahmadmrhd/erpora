package apicontract

import "fmt"

// Set adalah kumpulan dokumen yang dapat saling merujuk lewat erp:<nama>.
type Set struct {
	docs map[string]*Doc
	// urutan memasukkan dokumen, agar iterasi deterministik.
	order []string
}

// NewSet membuat Set; nama dokumen ganda adalah error.
func NewSet(docs ...*Doc) (*Set, error) {
	s := &Set{docs: map[string]*Doc{}}
	for _, d := range docs {
		if _, dup := s.docs[d.Name]; dup {
			return nil, fmt.Errorf("dokumen kontrak %q terdaftar lebih dari sekali", d.Name)
		}
		s.docs[d.Name] = d
		s.order = append(s.order, d.Name)
	}
	return s, nil
}

// Doc mencari dokumen berdasarkan nama.
func (s *Set) Doc(name string) (*Doc, bool) {
	d, ok := s.docs[name]
	return d, ok
}

// Resolve menyelesaikan satu nilai $ref yang ditulis di dokumen from. Mengembalikan nama dokumen pemilik node
// dan node-nya (belum diikuti bila node itu sendiri berisi $ref).
func (s *Set) Resolve(from, ref string) (doc string, node any, err error) {
	r, err := ParseRef(from, ref)
	if err != nil {
		return "", nil, fmt.Errorf("dokumen %s: %w", from, err)
	}
	d, ok := s.docs[r.Doc]
	if !ok {
		return "", nil, fmt.Errorf("dokumen %s: $ref %q merujuk dokumen %q yang tidak ada", from, ref, r.Doc)
	}
	n, ok := walkPointer(d.Spec, r.Pointer)
	if !ok {
		return "", nil, fmt.Errorf("dokumen %s: $ref %q tidak ditemukan", from, ref)
	}
	return r.Doc, n, nil
}

// resolveDeep seperti Resolve, tetapi mengikuti rantai $ref sampai node tanpa $ref. Siklus rantai adalah error.
func (s *Set) resolveDeep(from string, node any) (doc string, out any, err error) {
	doc, out = from, node
	for i := 0; i < 32; i++ {
		m, ok := out.(map[string]any)
		if !ok {
			return doc, out, nil
		}
		ref, ok := m["$ref"].(string)
		if !ok {
			return doc, out, nil
		}
		doc, out, err = s.Resolve(doc, ref)
		if err != nil {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("dokumen %s: rantai $ref terlalu dalam atau siklik", from)
}
