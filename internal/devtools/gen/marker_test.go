package gen

import (
	"errors"
	"strings"
	"testing"
)

func TestInsertAtMarker_Table(t *testing.T) {
	const marker = "// erp:generated:routes"
	src := "func f() {\n\tr.Handle(a)\n\t" + marker + "\n\treturn\n}\n"
	line := `r.Handle(b)`

	t.Run("sisip dengan indentasi tab", func(t *testing.T) {
		got, err := InsertAtMarker("m.go", []byte(src), marker, line)
		if err != nil {
			t.Fatal(err)
		}
		want := "func f() {\n\tr.Handle(a)\n\tr.Handle(b)\n\t" + marker + "\n\treturn\n}\n"
		if string(got) != want {
			t.Errorf("got:\n%s\nwant:\n%s", got, want)
		}
	})
	t.Run("baris sudah ada tidak berubah", func(t *testing.T) {
		once, _ := InsertAtMarker("m.go", []byte(src), marker, line)
		twice, err := InsertAtMarker("m.go", once, marker, line)
		if err != nil || string(twice) != string(once) {
			t.Errorf("sisip kedua harus no-op: err=%v\n%s", err, twice)
		}
	})
	t.Run("penanda hilang", func(t *testing.T) {
		_, err := InsertAtMarker("modules/x/module.go", []byte("package x\n"), marker, line)
		if !errors.Is(err, ErrMarkerNotFound) {
			t.Fatalf("err = %v, want ErrMarkerNotFound", err)
		}
		for _, want := range []string{"tambahkan `" + line + "` secara manual", "modules/x/module.go", marker} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("pesan tidak memuat %q: %v", want, err)
			}
		}
	})
	t.Run("penanda ganda", func(t *testing.T) {
		_, err := InsertAtMarker("m.go", []byte(src+"\t"+marker+"\n"), marker, line)
		if !errors.Is(err, ErrAmbiguousMarker) {
			t.Errorf("err = %v, want ErrAmbiguousMarker", err)
		}
	})
	t.Run("penanda hanya dikenali sebagai baris utuh", func(t *testing.T) {
		_, err := InsertAtMarker("m.go", []byte("x := \""+marker+"\"\n"), marker, line)
		if !errors.Is(err, ErrMarkerNotFound) {
			t.Errorf("err = %v, want ErrMarkerNotFound", err)
		}
	})
}

func TestAddGoImport(t *testing.T) {
	src := "package app\n\nimport (\n\t\"erpora/sdk/module\"\n)\n\nvar _ module.Module\n"
	got, err := AddGoImport("modules.go", []byte(src), "", "erpora/modules/inventory")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "\"erpora/modules/inventory\"") {
		t.Errorf("impor tidak ditambahkan:\n%s", got)
	}
	again, err := AddGoImport("modules.go", got, "", "erpora/modules/inventory")
	if err != nil || string(again) != string(got) {
		t.Errorf("impor ganda harus no-op: %v\n%s", err, again)
	}
	named, err := AddGoImport("modules.go", got, "httpapi", "erpora/modules/inventory/internal/presentation/http")
	if err != nil || !strings.Contains(string(named), "httpapi \"erpora/modules/inventory/internal/presentation/http\"") {
		t.Errorf("impor bernama: %v\n%s", err, named)
	}
	if _, err := AddGoImport("bad.go", []byte("bukan go"), "", "x"); err == nil {
		t.Error("sumber rusak harus error")
	}
}

func TestFormatGo(t *testing.T) {
	got, err := FormatGo("a.go", []byte("package a\nimport \"fmt\"\nfunc F(){fmt.Println( 1 )}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package a\n\nimport \"fmt\"\n\nfunc F() { fmt.Println(1) }\n" {
		t.Errorf("hasil format:\n%q", got)
	}
}
