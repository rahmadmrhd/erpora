package contracttools

// Blok path item untuk fixture negatif linter (lint_test.go). Masing-masing lengkap (summary, description, permission,
// contoh, kode error) agar hanya rule yang dituju yang menolak.

const nestedPathBlock = `  /api/example/notes/{id}/lines/{lineId}/taxes:
    get:
      operationId: listExampleNoteLineTaxes
      summary: Pajak baris
      description: Terlalu bersarang.
      x-erp-permission: example.notes.read
      parameters:
        - { name: id, in: path, required: true, schema: { $ref: 'erp:common#/components/schemas/Uuid' } }
        - { name: lineId, in: path, required: true, schema: { $ref: 'erp:common#/components/schemas/Uuid' } }
        - { $ref: 'erp:common#/components/parameters/Page' }
        - { $ref: 'erp:common#/components/parameters/PageSize' }
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: { type: object, required: [data, page], properties: { data: { type: array, items: { type: string } }, page: { $ref: 'erp:common#/components/schemas/Page' } } }
              example: { data: [], page: { page: 1, pageSize: 25, total: 0, hasMore: false } }
        '400': { $ref: 'erp:common#/components/responses/BadRequest' }
        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }
        '404': { $ref: 'erp:common#/components/responses/NotFound' }
`

const otherPrefixBlock = `  /api/other/things:
    get:
      operationId: listOtherThings
      summary: Milik modul lain
      description: Path di luar /api/example/.
      x-erp-permission: example.notes.read
      parameters:
        - { $ref: 'erp:common#/components/parameters/Page' }
        - { $ref: 'erp:common#/components/parameters/PageSize' }
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: { type: object, required: [data, page], properties: { data: { type: array, items: { type: string } }, page: { $ref: 'erp:common#/components/schemas/Page' } } }
              example: { data: [], page: { page: 1, pageSize: 25, total: 0, hasMore: false } }
        '400': { $ref: 'erp:common#/components/responses/BadRequest' }
        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }
`

const patchBlock = `  /api/example/notes/{id}:
    patch:
      operationId: patchExampleNote
      summary: Ubah note
      description: Tanpa If-Match.
      x-erp-permission: example.notes.read
      parameters:
        - { name: id, in: path, required: true, schema: { $ref: 'erp:common#/components/schemas/Uuid' } }
      requestBody:
        required: true
        content:
          application/json:
            schema: { type: object, properties: { title: { type: string, x-erp-classification: internal } } }
            example: { title: a }
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: { type: object, required: [data], properties: { data: { $ref: '#/components/schemas/Note' } } }
              example: { data: { id: 0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b, title: a, version: 2, createdAt: '2026-10-21T07:15:00Z', updatedAt: '2026-10-21T07:15:00Z' } }
        '400': { $ref: 'erp:common#/components/responses/BadRequest' }
        '403': { $ref: 'erp:common#/components/responses/PermissionDenied' }
        '404': { $ref: 'erp:common#/components/responses/NotFound' }
        '422': { $ref: 'erp:common#/components/responses/Unprocessable' }
`
