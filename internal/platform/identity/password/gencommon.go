//go:build ignore

// gencommon membangkitkan commonpasswords.txt: `go generate ./internal/platform/identity/password` (atau
// `go run gencommon.go` dari folder ini). Daftar ini ditulis sendiri oleh proyek dari kata umum Inggris dan Indonesia,
// pola papan ketik, dan pola angka, lalu dikombinasikan dengan akhiran lazim (angka, tahun, simbol) serta substitusi
// "leet". Bukan salinan daftar pihak ketiga (tanpa masalah lisensi). Keluaran deterministik (diurutkan, tanpa duplikat).
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

const minLen = 8 // entri yang lebih pendek tidak berguna: kebijakan mewajibkan >= 12 karakter

var english = strings.Fields(`password passw0rd passwd qwerty qwertyuiop asdfghjkl zxcvbnm admin administrator root welcome
letmein iloveyou monkey dragon master shadow sunshine princess football baseball superman batman trustno1 starwars whatever
freedom michael jennifer jordan hunter ranger buster thomas robert soccer hockey killer george charlie andrew michelle love
secret computer internet login access hello summer winter spring autumn flower cookie banana orange tigger pepper ginger
cheese coffee chocolate purple yellow silver golden diamond matthew daniel joshua jessica ashley nicole amanda hannah samantha
lakers yankees cowboys liverpool chelsea arsenal manchester barcelona madrid ferrari mercedes toyota honda yamaha google
facebook twitter youtube instagram whatsapp apple windows linux changeme default test testing guest user system server manager
support company office business finance account money dollar lucky happy angel family friend mother father sister brother baby
sweet honey cutie beauty pretty naruto pokemon minecraft fortnite hello123 welcome1 abc123 login123 pass word1 iloveu
letmein1 master1 dragon1 monkey1 sunshine1 princess1 qazwsx qazwsxedc zaq12wsx asdfgh zxcvbn football1 baseball1 mustang
harley rangers bulldog charlie1 maggie jackson mickey peanut ranger1 stella thunder tiger trinity victoria warrior wizard
phoenix pumpkin rainbow samsung scooter snoopy spider startrek sunset tennis testtest welcome123 yankee zombie
`)

var indonesian = strings.Fields(`sayang sayangku cintaku cinta kekasih rahasia indonesia jakarta bandung surabaya medan semarang
yogyakarta bali makassar palembang bismillah alhamdulillah allahuakbar muhammad ahmad rahmat ibu bapak keluarga bahagia sukses
semangat merdeka garuda nusantara pancasila merahputih kucing harimau naga anggrek melati mawar bunga bintang matahari bulan
langit laut gunung rumah sekolah kantor perusahaan karyawan sandi katasandi kunci masuk selamat pagi malam terimakasih persija
persib arema sambal nasigoreng mieayam rendang bakso tempe kopi manis ganteng cantik bidadari pahlawan juara kuat hebat
adminadmin rahasiaku sandiku passwordku saya aku kamu kita bersama selamanya setia jodoh bahagiaselalu mantap keren gaul
bangsa negara rakyat jawa sumatra kalimantan sulawesi papua lombok aceh batam bogor depok bekasi tangerang malang solo
bismillahirrahmanirrahim assalamualaikum waalaikumsalam subhanallah masyaallah insyaallah ramadhan lebaran
`)

var keyboard = []string{
	"qwertyuiop", "qwertyuiopasdfghjkl", "qwertyuiopasdf", "asdfghjklzxc", "asdfghjklzxcvbnm", "zxcvbnmasdf",
	"1qaz2wsx", "1qaz2wsx3edc", "1qazxsw2", "qazwsxedc", "qazwsxedcrfv", "zaq12wsx", "zaq1xsw2", "1q2w3e4r", "1q2w3e4r5t",
	"1q2w3e4r5t6y", "q1w2e3r4", "q1w2e3r4t5", "qweasdzxc", "qweasdzxc123", "qwerty123", "qwerty1234", "qwerty12345",
	"qwerty123456", "qwertyuiop123", "asdf1234", "asdfasdfasdf", "abcdefgh", "abcdefghi", "abcdefghij", "abcdefghijk",
	"abcdefghijkl", "abcdefghijklm", "abcdefghijklmnop", "abcd1234", "abcd12345", "abc123456", "abc123456789",
	"abcabcabcabc", "abababababab", "xyzxyzxyzxyz", "poiuytrewq", "lkjhgfdsa", "mnbvcxz", "mnbvcxzlkjhg", "zyxwvutsrqpo",
	"1234qwer", "1234qwerasdf", "123qweasd", "123qweasdzxc", "qwe123qwe123", "asd123asd123", "123abc123abc",
}

var suffixes = []string{
	"", "1", "2", "3", "7", "9", "12", "13", "21", "22", "23", "69", "77", "88", "99", "000", "007", "111", "222", "321", "666",
	"777", "888", "999", "123", "1234", "12345", "123456", "1234567", "12345678", "123456789", "0123", "01234", "1111",
	"2222", "0000", "1212", "4321", "54321", "654321", "!", "!!", "!!!", "@", "#", "@123", "@1234", "123!", "1234!", "!@#",
	"!@#$", "1!", "12!", "123@", "$", "$$", "..", "_1", "_123", "-123", "01", "02", "10", "11", "20", "25", "100", "101",
}

func leet(s string) []string {
	m := map[rune][]rune{'a': {'@', '4'}, 'e': {'3'}, 'i': {'1', '!'}, 'o': {'0'}, 's': {'$', '5'}, 't': {'7'}}
	out := map[string]bool{}
	var rec func(i int, cur []rune)
	rec = func(i int, cur []rune) {
		if i == len(s) {
			out[string(cur)] = true
			return
		}
		r := rune(s[i])
		rec(i+1, append(cur, r))
		for _, alt := range m[r] {
			rec(i+1, append(cur, alt))
		}
	}
	// Batasi ledakan kombinasi: leet hanya untuk kata <= 10 huruf, dan hanya satu atau dua substitusi (hasil
	// dikurangi: ambil varian yang berbeda <= 2 posisi dari aslinya).
	if len(s) > 10 {
		return nil
	}
	rec(0, nil)
	var res []string
	for k := range out {
		if k == s {
			continue
		}
		diff := 0
		for i := 0; i < len(k) && i < len(s); i++ {
			if k[i] != s[i] {
				diff++
			}
		}
		if diff >= 1 && diff <= 2 {
			res = append(res, k)
		}
	}
	return res
}

func main() {
	set := map[string]bool{}
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if len(s) >= minLen && !strings.HasPrefix(s, "#") {
			set[s] = true
		}
	}

	bases := append(append([]string{}, english...), indonesian...)
	years := []string{}
	for y := 1970; y <= 2026; y++ {
		years = append(years, fmt.Sprint(y))
	}
	for i, w := range bases {
		sfx := suffixes
		if i >= 100 {
			sfx = suffixes[:32] // kata yang kurang umum: akhiran angka lazim saja
		}
		for _, s := range sfx {
			add(w + s)
		}
		// Akhiran tahun dan dua digit untuk kata-kata teratas (paling sering dipakai).
		if i < 50 {
			for _, y := range years {
				add(w + y)
			}
			for d := 0; d < 100; d++ {
				add(fmt.Sprintf("%s%02d", w, d))
			}
		}
		for _, l := range leet(w) {
			for _, s := range []string{"", "123"} {
				add(l + s)
			}
		}
	}
	// Kombinasi dua kata untuk kata teratas.
	top := append(append([]string{}, english[:30]...), indonesian[:30]...)
	for _, a := range top {
		for _, b := range top {
			if a != b {
				add(a + b)
				add(a + b + "123")
			}
		}
	}
	// Pola papan ketik dengan akhiran.
	for _, k := range keyboard {
		for _, s := range suffixes {
			add(k + s)
		}
	}
	// Pola angka: urutan naik/turun, berulang, dan pola dasar panjang 8..16.
	for n := 8; n <= 16; n++ {
		up, down := "", ""
		for i := 0; i < n; i++ {
			up += fmt.Sprint((i + 1) % 10)
			down += fmt.Sprint((10 - i%10) % 10)
		}
		add(up)
		add(down)
		for d := 0; d <= 9; d++ {
			add(strings.Repeat(fmt.Sprint(d), n))
		}
		for _, l := range "abcdefghijklmnopqrstuvwxyz" {
			add(strings.Repeat(string(l), n))
		}
		for _, p := range []string{"12", "123", "1234", "10", "0123", "5678", "4321", "1122", "1212", "6969", "1357", "2468"} {
			add(strings.Repeat(p, n/len(p)+1)[:n])
		}
	}
	add("0123456789")
	add("01234567890")
	add("9876543210")
	add("987654321098")
	add("147258369")
	add("147258369147")
	add("159753456852")
	add("741852963")
	add("789456123")
	add("123456789a")
	add("123456789q")
	add("1234567890-")
	add("1234567890qwerty")
	add("0987654321")
	add("11223344")
	add("112233445566")
	add("11223344556677")
	add("1122334455667788")

	list := make([]string, 0, len(set))
	for s := range set {
		list = append(list, s)
	}
	sort.Strings(list)

	f, err := os.Create("commonpasswords.txt")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "# Daftar sandi umum (huruf kecil, satu per baris) untuk password.CheckPolicy (PRD-001 FR-101).")
	fmt.Fprintln(w, "# DIBANGKITKAN oleh gencommon.go (go generate ./internal/platform/identity/password); jangan diedit tangan.")
	fmt.Fprintln(w, "# Asal: kata umum Inggris dan Indonesia, pola papan ketik, dan pola angka yang ditulis sendiri oleh proyek ERPora,")
	fmt.Fprintln(w, "# dikombinasikan dengan akhiran lazim (angka, tahun 1970-2026, simbol) dan substitusi leet. Bukan salinan atau turunan")
	fmt.Fprintln(w, "# daftar pihak ketiga; tidak ada pembatasan lisensi. Hanya entri >= 8 karakter (kebijakan mewajibkan >= 12).")
	fmt.Fprintln(w, "# Baris berawalan # adalah komentar. Pencarian memakai huruf kecil.")
	for _, s := range list {
		fmt.Fprintln(w, s)
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("commonpasswords.txt: %d entri\n", len(list))
}
