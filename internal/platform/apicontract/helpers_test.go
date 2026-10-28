package apicontract

import (
	"os"
	"path/filepath"
	"testing"
)

// loadDoc membaca testdata/<name>.yaml sebagai dokumen bernama name.
func loadDoc(t *testing.T, name string) *Doc {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(name, data)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return d
}

func loadSet(t *testing.T, names ...string) *Set {
	t.Helper()
	docs := make([]*Doc, 0, len(names))
	for _, n := range names {
		docs = append(docs, loadDoc(t, n))
	}
	s, err := NewSet(docs...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// opOf mencari operasi berdasarkan operationId.
func opOf(t *testing.T, s *Set, doc, operationID string) Operation {
	t.Helper()
	for _, op := range s.Operations(doc) {
		if op.OperationID == operationID {
			return op
		}
	}
	t.Fatalf("operasi %s tidak ada di %s", operationID, doc)
	return Operation{}
}
