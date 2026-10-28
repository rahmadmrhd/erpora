package secfixture

import "net/http"

// Fixture: klien mentah tanpa konstruktor (zero value), sebagai field/parameter/return, dan pemanggilan method
// pada klien yang tidak menyebut nama tipenya di situ. Tepat 8 pelanggaran EGRESS (satu per baris).
var zeroVar http.Client // 1

type holder struct {
	c  http.Client     // 2
	tr *http.Transport // 3
}

func useParam(c *http.Client) {} // 4

func factory() *http.Client { return nil } // 5

func zeroCall() {
	var c http.Client         // 6
	_, _ = c.Get("https://x") // 7
	_, _ = factory().Do(nil)  // 8 (pemanggilan Do pada klien dari factory; baris ini tidak menyebut tipe)
}
