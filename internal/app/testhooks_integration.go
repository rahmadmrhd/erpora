//go:build integration

package app

import (
	"os"
	"strings"
	"time"

	"erpora/internal/platform/modules/lifecycle"
)

// Seam khusus test yang HANYA ada pada biner bertag `integration` (seperti testroutes_integration.go); biner
// release/devtools tidak membaca env ini. Dipakai tests/integration untuk mensimulasikan crash dan kontensi lock:
//
//	ERP_TEST_CRASH_AFTER_STEP=<langkah>   keluar dengan kode 99 (tanpa defer) setelah langkah itu selesai
//	ERP_TEST_STEP_DELAY=<langkah>:<durasi> tidur sebelum langkah itu (mis. hook:enable:7s)
func init() {
	lifecycleTestHooks = func(d *lifecycle.Deps) {
		if step := os.Getenv("ERP_TEST_CRASH_AFTER_STEP"); step != "" {
			d.AfterStep = func(_ lifecycle.Op, _, s string) {
				if s == step {
					os.Exit(99)
				}
			}
		}
		if spec := os.Getenv("ERP_TEST_STEP_DELAY"); spec != "" {
			if i := strings.LastIndex(spec, ":"); i > 0 {
				if dur, err := time.ParseDuration(spec[i+1:]); err == nil {
					step := spec[:i]
					d.BeforeStep = func(_ lifecycle.Op, _, s string) {
						if s == step {
							time.Sleep(dur)
						}
					}
				}
			}
		}
	}
}
