package contracttools

import (
	"context"
	"strings"
	"testing"
)

// authenticatedBlock: operasi terautentikasi tanpa permission (x-erp-authenticated), seperti GET /api/bootstrap:
// tanpa x-erp-permission dan tanpa 403 (tidak ada permission yang dapat ditolak), tetapi dengan 401.
const authenticatedBlock = `  /api/example/whoami:
    x-erp-singleton: true
    get:
      operationId: getExampleWhoami
      summary: Pengguna saat ini
      description: Cukup login; tanpa permission tertentu.
      x-erp-authenticated: true
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: { type: object, required: [data], properties: { data: { type: string } } }
              example: { data: halo }
        '401': { description: Tidak terautentikasi., content: { application/json: { schema: { $ref: 'erp:common#/components/schemas/Error' } } } }
`

func lintWithBlock(t *testing.T, block string) []LintResult {
	t.Helper()
	bun := requireBun(t)
	root := t.TempDir()
	copyRepoSubset(t, root, "example", "exampleaddon")
	editFile(t, root, "modules/example/openapi.yaml",
		[2]string{notes403Then, "        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }\n" + block + "components:\n"})
	repo, err := LoadRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	repo.ToolsRoot = realRoot
	res, err := Lint(context.Background(), repo, "example", bun)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// C1.13: operasi terautentikasi-saja (x-erp-authenticated) sah tanpa x-erp-permission dan tanpa 403.
func TestLint_AuthenticatedOperationNeedsNoPermissionOr403(t *testing.T) {
	for _, r := range lintWithBlock(t, authenticatedBlock) {
		t.Errorf("tidak boleh ada temuan: %s %s", r.Rule, r.Message)
	}
}

// Jalur 401 wajib didokumentasikan pada operasi terautentikasi-saja.
func TestLint_AuthenticatedOperationNeeds401(t *testing.T) {
	block := strings.Replace(authenticatedBlock, "        '401': { description: Tidak terautentikasi., content: { application/json: { schema: { $ref: 'erp:common#/components/schemas/Error' } } } }\n",
		"        '400': { $ref: 'erp:common#/components/responses/BadRequest' }\n", 1)
	for _, r := range lintWithBlock(t, block) {
		if r.Rule == "erp-error-codes-documented" && strings.Contains(r.Message, "401") {
			return
		}
	}
	t.Error("operasi x-erp-authenticated tanpa respons 401 harus melanggar erp-error-codes-documented")
}
