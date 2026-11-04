package contracttools

import (
	"context"
	"sort"
	"strings"
	"testing"
)

// Penanda kredensial pada properti respons (K-C1.4; PRD-000 Q42 butir 4; PRD-001 Q16/Q27): rule
// erp-credential-response-valid (allowlist modul, operationId, properti) dan erp-credential-response-exception (INFO).

const coreSpec = "modules/core/openapi.yaml"

func lintCore(t *testing.T, edits ...[2]string) []LintResult {
	t.Helper()
	bun := requireBun(t)
	root := t.TempDir()
	copyRepoSubset(t, root, "core")
	if len(edits) > 0 {
		editFile(t, root, coreSpec, edits...)
	}
	repo, err := LoadRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	repo.ToolsRoot = realRoot
	res, err := Lint(context.Background(), repo, "core", bun)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func rulesOf(res []LintResult, sev string) []string {
	seen := map[string]bool{}
	for _, r := range res {
		if (sev == "" && r.Severity != SeverityInfo) || (sev != "" && r.Severity == sev) {
			seen[r.Rule] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func hasMessage(res []LintResult, rule, part string) bool {
	for _, r := range res {
		if r.Rule == rule && strings.Contains(r.Message, part) {
			return true
		}
	}
	return false
}

// Kontrak core yang sah lolos tanpa ERROR dan setiap operasi yang mengembalikan kredensial dicetak INFO.
func TestLint_CredentialMarkers_CoreCleanAndRecordedAsInfo(t *testing.T) {
	res := lintCore(t)
	if got := rulesOf(res, ""); len(got) != 0 {
		for _, r := range res {
			if r.Severity != SeverityInfo {
				t.Errorf("%s %s", r.Rule, r.Message)
			}
		}
		t.Fatalf("tidak boleh ada ERROR: %v", got)
	}
	info := 0
	for _, r := range res {
		if r.Rule == "erp-credential-response-exception" && r.Severity == SeverityInfo {
			info++
		}
	}
	if info != 7 {
		t.Errorf("INFO erp-credential-response-exception = %d, want 7 (login, refresh, enroll, confirm, verify, recovery, recovery-codes)", info)
	}
}

func TestLint_CredentialMarkers_Negative(t *testing.T) {
	const (
		accessTokenMark = "          x-erp-credential-response: true\n        expiresIn:"
		accessTokenNone = "        expiresIn:"
	)
	cases := []struct {
		name  string
		edits [][2]string
		want  []string // rule ERROR yang harus muncul (hanya itu)
		rule  string   // rule untuk pencocokan pesan
		msg   string
	}{
		{
			// Tanpa penanda, accessToken (highly_confidential) di respons ditolak seperti biasa.
			name:  "tanpa_penanda_ditolak",
			edits: [][2]string{{accessTokenMark, accessTokenNone}},
			want:  []string{"erp-no-highly-confidential-in-response"}, rule: "erp-no-highly-confidential-in-response", msg: "highly_confidential",
		},
		{
			// operationId login diganti: (core, loginX, accessToken) tidak ada di allowlist.
			name:  "operationId_di_luar_allowlist",
			edits: [][2]string{{"operationId: login\n", "operationId: loginLain\n"}},
			want:  []string{"erp-credential-response-valid"}, rule: "erp-credential-response-valid", msg: "allowlist",
		},
		{
			name: "tanpa_klasifikasi_highly",
			edits: [][2]string{{
				"          description: URI otpauth untuk QR code; memuat secret, tampil sekali.\n          x-erp-classification: highly_confidential\n",
				"          description: URI otpauth untuk QR code; memuat secret, tampil sekali.\n",
			}},
			want: []string{"erp-credential-response-valid"}, rule: "erp-credential-response-valid", msg: "highly_confidential",
		},
		{
			// Tingkat operasi saja tidak cukup.
			name: "tingkat_operasi",
			edits: [][2]string{{
				"      operationId: logout\n", "      operationId: logout\n      x-erp-credential-response: true\n",
			}},
			want: []string{"erp-credential-response-valid"}, rule: "erp-credential-response-valid", msg: "tingkat operasi",
		},
		{
			// Penanda pada properti requestBody ditolak.
			name: "di_request_body",
			edits: [][2]string{{
				"        password: { type: string, x-erp-classification: highly_confidential }\n",
				"        password: { type: string, x-erp-classification: highly_confidential, x-erp-credential-response: true }\n",
			}},
			want: []string{"erp-credential-response-valid"}, rule: "erp-credential-response-valid", msg: "respons 2xx",
		},
		{
			// Properti kredensial lain berpenanda di properti yang tidak ada di allowlist (login hanya accessToken).
			name: "properti_di_luar_allowlist",
			edits: [][2]string{{
				accessTokenNone,
				"        sessionSecret: { type: string, x-erp-classification: highly_confidential, x-erp-credential-response: true }\n" + accessTokenNone,
			}},
			want: []string{"erp-credential-response-valid"}, rule: "erp-credential-response-valid", msg: "sessionSecret",
		},
		{
			// refresh token di body selalu ERROR, walau berpenanda.
			name: "refresh_token_di_body_berpenanda",
			edits: [][2]string{{
				accessTokenNone,
				"        refreshToken: { type: string, x-erp-classification: highly_confidential, x-erp-credential-response: true }\n" + accessTokenNone,
			}},
			want: []string{"erp-credential-response-valid"}, rule: "erp-credential-response-valid", msg: "cookie",
		},
		{
			// refresh* tanpa penanda dan bukan nama sensitif tetap ERROR.
			name: "refresh_expires_di_body",
			edits: [][2]string{{
				accessTokenNone, "        refreshExpiresAt: { type: string }\n" + accessTokenNone,
			}},
			want: []string{"erp-credential-response-valid"}, rule: "erp-credential-response-valid", msg: "cookie",
		},
		{
			// Nilai selain true ditolak.
			name: "nilai_bukan_true",
			edits: [][2]string{{
				"          x-erp-credential-response: true\n        expiresIn:", "          x-erp-credential-response: 'ya'\n        expiresIn:",
			}},
			want: []string{"erp-credential-response-valid", "erp-no-highly-confidential-in-response"}, rule: "erp-credential-response-valid", msg: "true",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := lintCore(t, c.edits...)
			got := rulesOf(res, "")
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				for _, r := range res {
					t.Logf("%s %s: %s", r.Severity, r.Rule, r.Message)
				}
				t.Fatalf("rule ERROR = %v, want %v", got, c.want)
			}
			if !hasMessage(res, c.rule, c.msg) {
				t.Errorf("tidak ada temuan %s dengan %q", c.rule, c.msg)
			}
		})
	}
}

// Modul selain core/platform tidak punya allowlist: penanda di modul example selalu ERROR.
func TestLint_CredentialMarker_OutsideCoreAlwaysError(t *testing.T) {
	bun := requireBun(t)
	root := t.TempDir()
	copyRepoSubset(t, root, "example", "exampleaddon")
	editFile(t, root, "modules/example/openapi.yaml",
		[2]string{versionProp, "        accessToken: { type: string, x-erp-classification: highly_confidential, x-erp-credential-response: true }\n" + versionProp})
	repo, err := LoadRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	repo.ToolsRoot = realRoot
	res, err := Lint(context.Background(), repo, "example", bun)
	if err != nil {
		t.Fatal(err)
	}
	if !hasMessage(res, "erp-credential-response-valid", "allowlist") {
		for _, r := range res {
			t.Logf("%s %s: %s", r.Severity, r.Rule, r.Message)
		}
		t.Error("penanda kredensial di modul example harus ditolak (di luar allowlist)")
	}
}
