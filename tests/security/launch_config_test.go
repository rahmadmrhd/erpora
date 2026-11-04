package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// launchConfig adalah satu entri .claude/launch.json (hanya bagian yang relevan untuk keamanan kunci rahasia).
type launchConfig struct {
	Name              string            `json:"name"`
	RuntimeExecutable string            `json:"runtimeExecutable"`
	RuntimeArgs       []string          `json:"runtimeArgs"`
	Env               map[string]string `json:"env"`
}

var (
	// Variabel kunci rahasia (aktif dan lama; sufiks _PREVIOUS ikut): nilainya tidak boleh ditanam di berkas terlacak.
	secretVarName = regexp.MustCompile(`^ERP_SECRET_KEY(_PREVIOUS)?$`)
	// Penugasan dalam teks perintah: `ERP_SECRET_KEY=...`, `$env:ERP_SECRET_KEY='...'`, `ERP_SECRET_KEY_PREVIOUS="..."`.
	secretAssign = regexp.MustCompile(`ERP_SECRET_KEY(?:_PREVIOUS)?["']?\s*=\s*["']?([^\s"';]+)`)
	tokenSplit   = regexp.MustCompile(`[\s;&|(){}]+`)
)

// tokens memecah semua argumen (dan executable) menjadi token kata, tanpa kutip, dengan urutan asli.
func (c launchConfig) tokens() []string {
	var out []string
	for _, part := range append([]string{c.RuntimeExecutable}, c.RuntimeArgs...) {
		for _, tok := range tokenSplit.Split(part, -1) {
			if tok = strings.Trim(tok, `"'`); tok != "" {
				out = append(out, tok)
			}
		}
	}
	return out
}

// runsServe: true bila ada token `serve` yang mengikuti binary erpora (`erpora`, `erpora-verify.exe`, `./cmd/erpora`,
// `./tmp/erpora.exe`) atau pemisah `--` (mis. `task erpora -- serve`).
func runsServe(tokens []string) bool {
	for i, tok := range tokens {
		if tok != "serve" || i == 0 {
			continue
		}
		prev := tokens[i-1]
		if strings.Contains(prev, "erpora") || prev == "--" {
			return true
		}
	}
	return false
}

// hasEnvFileArg: `--env-file` sebagai token utuh diikuti nilai (bukan flag lain), atau `--env-file=<nilai>`.
func hasEnvFileArg(tokens []string) bool {
	for i, tok := range tokens {
		if v, ok := strings.CutPrefix(tok, "--env-file="); ok && v != "" {
			return true
		}
		if tok == "--env-file" && i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "-") {
			return true
		}
	}
	return false
}

// launchViolations mengembalikan pelanggaran (urut) pada isi launch.json dan jumlah konfigurasi yang menjalankan serve.
func launchViolations(t *testing.T, data []byte) (violations []string, serveConfigs int) {
	t.Helper()
	var file struct {
		Configurations []launchConfig `json:"configurations"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("launch.json tidak valid: %v", err)
	}
	for _, c := range file.Configurations {
		// (a) objek env: variabel kunci dengan nilai non-kosong = kunci tertanam.
		for name, val := range c.Env {
			if secretVarName.MatchString(name) && strings.TrimSpace(val) != "" {
				violations = append(violations, c.Name+": env."+name+" menanam kunci di berkas terlacak")
			}
		}
		// (b) semua argumen (termasuk executable): penugasan kunci literal pada kedua nama variabel.
		joined := c.RuntimeExecutable + " " + strings.Join(c.RuntimeArgs, " ")
		for _, m := range secretAssign.FindAllStringSubmatch(joined, -1) {
			if v := m[1]; len(v) >= 16 && !strings.HasPrefix(v, "$") {
				violations = append(violations, c.Name+": argumen menanam kunci literal ("+strings.SplitN(m[0], "=", 2)[0]+"=...)")
			}
		}
		// (c) command serve wajib mendapat kunci dari sumber tak terlacak lewat --env-file utuh.
		toks := c.tokens()
		if runsServe(toks) {
			serveConfigs++
			if !hasEnvFileArg(toks) {
				violations = append(violations, c.Name+": menjalankan serve tanpa argumen --env-file <berkas> (serve exit 2 tanpa ERP_SECRET_KEY)")
			}
		}
	}
	sort.Strings(violations)
	return violations, serveConfigs
}

// TestLaunchConfigServeHasSecretKeySource: konfigurasi pratinjau (.claude/launch.json) yang menjalankan `serve` harus
// mendapat ERP_SECRET_KEY dari sumber yang tidak terlacak (--env-file ke .env yang di-gitignore), karena serve wajib
// berkunci di semua mode. Kunci (aktif maupun PREVIOUS) tidak boleh ditanam, di argumen maupun objek env.
func TestLaunchConfigServeHasSecretKeySource(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".claude", "launch.json")) //nolint:gosec // berkas repo
	if err != nil {
		t.Fatal(err)
	}
	violations, serves := launchViolations(t, data)
	for _, v := range violations {
		t.Error(v)
	}
	if serves == 0 {
		t.Error("tidak ada konfigurasi yang menjalankan serve; tes ini perlu diperbarui")
	}
}

// TestLaunchViolationsDetectsBadAndAcceptsOK: fixture JSON bad/ok agar pemeriksa tidak longgar diam-diam.
func TestLaunchViolationsDetectsBadAndAcceptsOK(t *testing.T) {
	const key = "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789+/AbCdE=" // bentuk kunci base64 (bukan rahasia nyata)
	cases := []struct {
		name      string
		json      string
		wantBad   []string // potongan pesan yang wajib muncul
		wantServe int
	}{
		{"ok: powershell serve dengan --env-file", `{"configurations":[{"name":"a","runtimeExecutable":"powershell","runtimeArgs":["-Command","& ./tmp/erpora-verify.exe serve --env-file .env"]}]}`, nil, 1},
		{"ok: --env-file=berkas", `{"configurations":[{"name":"a","runtimeExecutable":"./erpora","runtimeArgs":["serve","--env-file=.env"]}]}`, nil, 1},
		{"ok: task erpora -- serve dengan env-file", `{"configurations":[{"name":"a","runtimeExecutable":"task","runtimeArgs":["erpora","--","serve","--env-file",".env"]}]}`, nil, 1},
		{"ok: bukan serve dan kunci kosong di env", `{"configurations":[{"name":"w","runtimeExecutable":"bun","runtimeArgs":["dev"],"env":{"ERP_SECRET_KEY":""}}]}`, nil, 0},
		{"ok: referensi variabel, bukan literal", `{"configurations":[{"name":"a","runtimeExecutable":"powershell","runtimeArgs":["-Command","$env:ERP_SECRET_KEY=$env:MY_KEY; ./erpora serve --env-file .env"]}]}`, nil, 1},
		{"bad: serve tanpa --env-file", `{"configurations":[{"name":"a","runtimeExecutable":"./erpora","runtimeArgs":["serve"]}]}`, []string{"tanpa argumen --env-file"}, 1},
		{"bad: task erpora -- serve tanpa --env-file", `{"configurations":[{"name":"a","runtimeExecutable":"task","runtimeArgs":["erpora","--","serve"]}]}`, []string{"tanpa argumen --env-file"}, 1},
		{"bad: --env-file tanpa nilai", `{"configurations":[{"name":"a","runtimeExecutable":"./erpora","runtimeArgs":["serve","--env-file"]}]}`, []string{"tanpa argumen --env-file"}, 1},
		{"bad: --env-file hanya di dalam komentar", `{"configurations":[{"name":"a","runtimeExecutable":"powershell","runtimeArgs":["-Command","./erpora serve --flag-lain x"]}]}`, []string{"tanpa argumen --env-file"}, 1},
		{"bad: kunci di objek env", `{"configurations":[{"name":"a","runtimeExecutable":"./erpora","runtimeArgs":["serve","--env-file",".env"],"env":{"ERP_SECRET_KEY":"` + key + `"}}]}`, []string{"env.ERP_SECRET_KEY menanam"}, 1},
		{"bad: PREVIOUS di objek env", `{"configurations":[{"name":"a","runtimeExecutable":"bun","runtimeArgs":["x"],"env":{"ERP_SECRET_KEY_PREVIOUS":"` + key + `"}}]}`, []string{"env.ERP_SECRET_KEY_PREVIOUS menanam"}, 0},
		{"bad: kunci literal di argumen powershell", `{"configurations":[{"name":"a","runtimeExecutable":"powershell","runtimeArgs":["-Command","$env:ERP_SECRET_KEY='` + key + `'; ./erpora serve --env-file .env"]}]}`, []string{"menanam kunci literal (ERP_SECRET_KEY=...)"}, 1},
		{"bad: PREVIOUS literal di argumen", `{"configurations":[{"name":"a","runtimeExecutable":"powershell","runtimeArgs":["-Command","$env:ERP_SECRET_KEY_PREVIOUS=\"` + key + `\"; ./erpora serve --env-file .env"]}]}`, []string{"menanam kunci literal (ERP_SECRET_KEY_PREVIOUS=...)"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, serves := launchViolations(t, []byte(tc.json))
			if serves != tc.wantServe {
				t.Errorf("serveConfigs = %d, want %d", serves, tc.wantServe)
			}
			if len(tc.wantBad) == 0 {
				if len(got) != 0 {
					t.Errorf("tidak boleh ada pelanggaran: %v", got)
				}
				return
			}
			all := strings.Join(got, "\n")
			for _, want := range tc.wantBad {
				if !strings.Contains(all, want) {
					t.Errorf("pelanggaran %q tidak terdeteksi; dapat: %v", want, got)
				}
			}
			if strings.Contains(all, key) {
				t.Errorf("pesan pelanggaran tidak boleh memuat nilai kunci: %v", got)
			}
		})
	}
}
