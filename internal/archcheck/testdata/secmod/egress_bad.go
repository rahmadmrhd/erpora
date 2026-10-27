package secfixture

import (
	"net/http"
	nh "net/http"
)

// Fixture: tepat 10 pelanggaran EGRESS.
func egressBad() {
	_, _ = http.Get("https://x")                     // 1
	_, _ = http.Post("https://x", "text/plain", nil) // 2
	_, _ = http.Head("https://x")                    // 3
	_, _ = http.PostForm("https://x", nil)           // 4
	_, _ = http.DefaultClient.Do(nil)                // 5
	_ = &http.Client{}                               // 6
	_ = new(http.Client)                             // 7
	_ = http.Transport{}                             // 8
	_ = http.DefaultTransport                        // 9
	_, _ = nh.Get("https://x")                       // 10
}
