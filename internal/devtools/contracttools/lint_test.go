package contracttools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
)

const (
	notesPath       = "  /api/example/notes:\n"
	notes403Then    = "        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }\ncomponents:\n"
	pingThenNotes   = "        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }\n  /api/example/notes:\n"
	pageParamsBlock = "      parameters:\n        - { $ref: 'erp:common#/components/parameters/Page' }\n        - { $ref: 'erp:common#/components/parameters/PageSize' }\n"
	versionProp     = "        version: { type: integer, format: int64 }\n"
	pageSizeParam   = "        - { $ref: 'erp:common#/components/parameters/PageSize' }\n"
)

func TestLint_RepositoryContractsClean(t *testing.T) {
	bun := requireBun(t)
	repo, err := LoadRepo(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Lint(context.Background(), repo, "", bun)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		t.Errorf("%s:%d %s %s", r.File, r.Line, r.Rule, r.Message)
	}
}

type lintCase struct {
	name  string
	edits map[string][][2]string // path relatif -> pasangan {lama, baru}
	want  []string               // rule yang harus muncul (dan hanya itu)
	msg   string                 // potongan pesan yang harus ada pada salah satu temuan
}

func TestLint_Fixtures(t *testing.T) {
	bun := requireBun(t)
	ex := "modules/example/openapi.yaml"
	cases := []lintCase{
		{
			name: "ref_not_dependency",
			edits: map[string][][2]string{
				ex: {{versionProp, versionProp + "        extra: { $ref: 'erp:example-addon#/components/schemas/Extra' }\n"}},
				"modules/exampleaddon/openapi.yaml": {{
					"        '500': { $ref: 'erp:common#/components/responses/InternalError' }\n",
					"        '500': { $ref: 'erp:common#/components/responses/InternalError' }\ncomponents:\n  schemas:\n    Extra: { type: string }\n",
				}},
			},
			want: []string{"erp-ref-dependency"}, msg: "FR-23.1",
		},
		{
			name:  "number_total",
			edits: map[string][][2]string{ex: {{versionProp, versionProp + "        total: { type: number }\n"}}},
			want:  []string{"erp-decimal-not-number"}, msg: "api-guidelines §4.1",
		},
		{
			name:  "crud_verb_path",
			edits: map[string][][2]string{ex: {{notesPath, "  /api/example/getItems:\n"}}},
			want:  []string{"erp-path-no-crud-verb", "erp-path-kebab-case"}, msg: "api-guidelines §2",
		},
		{
			name:  "missing_permission",
			edits: map[string][][2]string{ex: {{"      x-erp-permission: example.notes.read\n", ""}}},
			want:  []string{"erp-operation-permission"}, msg: "api-guidelines §1",
		},
		{
			name:  "password_response",
			edits: map[string][][2]string{ex: {{versionProp, versionProp + "        password: { type: string }\n"}}},
			want:  []string{"erp-sensitive-needs-classification"}, msg: "FR-24.2",
		},
		{
			name: "token_query",
			edits: map[string][][2]string{ex: {{pageSizeParam,
				pageSizeParam + "        - { name: token, in: query, x-erp-classification: highly_confidential, schema: { type: string } }\n"}}},
			want: []string{"erp-no-sensitive-in-url"}, msg: "FR-24.2",
		},
		{
			name:  "put_without_flag",
			edits: map[string][][2]string{ex: {{pingThenNotes, "        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }\n" + putBlock + notesPath}}},
			want:  []string{"erp-no-put"}, msg: "api-guidelines §3",
		},
		{
			name:  "missing_envelope",
			edits: map[string][][2]string{ex: {{"                required: [data, page]", "                required: [page]"}}},
			want:  []string{"erp-success-envelope"}, msg: "api-guidelines §4.2",
		},
		{
			name:  "collection_without_page",
			edits: map[string][][2]string{ex: {{pageParamsBlock, ""}}},
			want:  []string{"erp-collection-pagination"}, msg: "api-guidelines §6",
		},
		{
			name:  "post_without_idempotency",
			edits: map[string][][2]string{ex: {{notes403Then, "        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }\n" + postBlock + "components:\n"}}},
			want:  []string{"erp-post-idempotency-key"}, msg: "api-guidelines §7.2",
		},
		{
			name:  "singular_without_flag",
			edits: map[string][][2]string{ex: {{notes403Then, "        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }\n" + reportBlock + "components:\n"}}},
			want:  []string{"erp-path-collection-plural"}, msg: "api-guidelines §2",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			copyRepoSubset(t, root, "example", "exampleaddon")
			for file, edits := range c.edits {
				editFile(t, root, file, edits...)
			}
			repo, err := LoadRepo(root)
			if err != nil {
				t.Fatal(err)
			}
			repo.ToolsRoot = realRoot
			res, err := Lint(context.Background(), repo, "example", bun)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{}
			found := false
			for _, r := range res {
				got[r.Rule] = true
				if strings.Contains(r.Message, c.msg) {
					found = true
				}
			}
			var gotList []string
			for r := range got {
				gotList = append(gotList, r)
			}
			sort.Strings(gotList)
			want := append([]string{}, c.want...)
			sort.Strings(want)
			if fmt.Sprint(gotList) != fmt.Sprint(want) {
				t.Errorf("rule = %v, want %v\n%v", gotList, want, res)
			}
			if !found {
				t.Errorf("tidak ada temuan yang pesannya memuat %q: %v", c.msg, res)
			}
		})
	}
}

func TestLint_UnknownTarget(t *testing.T) {
	bun := requireBun(t)
	repo, err := LoadRepo(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Lint(context.Background(), repo, "tidak-ada", bun); err == nil {
		t.Error("modul tidak dikenal harus error")
	}
}

const putBlock = `    put:
      operationId: putExamplePing
      summary: Ganti ping
      description: Ganti ping.
      x-erp-permission: example.ping.read
      requestBody:
        required: true
        content:
          application/json:
            schema: { type: object, properties: { pong: { type: boolean } } }
            example: { pong: true }
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: { type: object, required: [data], properties: { data: { type: object, properties: { pong: { type: boolean } } } } }
              example: { data: { pong: true } }
        '400': { $ref: 'erp:common#/components/responses/BadRequest' }
        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }
        '422': { $ref: 'erp:common#/components/responses/Unprocessable' }
`

const postBlock = `    post:
      operationId: createExampleNote
      summary: Buat note
      description: Buat note.
      x-erp-permission: example.notes.read
      requestBody:
        required: true
        content:
          application/json:
            schema: { type: object, required: [title], properties: { title: { type: string, x-erp-classification: internal } } }
            example: { title: a }
      responses:
        '201':
          description: dibuat
          content:
            application/json:
              schema: { type: object, required: [data], properties: { data: { $ref: '#/components/schemas/Note' } } }
              example: { data: { id: 0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b, title: a, version: 1, createdAt: '2026-10-21T07:15:00Z', updatedAt: '2026-10-21T07:15:00Z' } }
        '400': { $ref: 'erp:common#/components/responses/BadRequest' }
        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }
        '422': { $ref: 'erp:common#/components/responses/Unprocessable' }
`

const reportBlock = `  /api/example/report:
    get:
      operationId: getExampleReport
      summary: Laporan
      description: Laporan.
      x-erp-permission: example.ping.read
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: { type: object, required: [data], properties: { data: { type: object, properties: { total: { $ref: 'erp:common#/components/schemas/Decimal' } } } } }
              example: { data: { total: '1.00' } }
        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }
`
