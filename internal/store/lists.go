package store

import "github.com/go-tangra/go-tangra/v4/listquery"

// List definitions of the paperless tables (specs/032-server-side-tables in
// go-tangra). Sort fields map to constant SQL expressions only; the memstore
// sorts the same public names in Go. Every documents sort column is NOT NULL:
// NotNull drops NULLS LAST from the ORDER BY, so plain btree indexes serve
// both directions.

// DocumentList pages paperless_documents: newest first by default.
var DocumentList = listquery.Spec{
	Fields: map[string]listquery.Field{
		"name":              {Expr: "name", Text: true, NotNull: true},
		"file_size":         {Expr: "file_size", DefaultDir: listquery.Desc, NotNull: true},
		"mime_type":         {Expr: "mime_type", Text: true, NotNull: true},
		"status":            {Expr: "status", NotNull: true},
		"processing_status": {Expr: "processing_status", NotNull: true},
		"created_at":        {Expr: "created_at", DefaultDir: listquery.Desc, NotNull: true},
	},
	Default: "created_at", TieBreak: "id",
}

// ListRequest completes r with the Spec's defaults (a zero Request from an
// internal caller pages with the defaults); an invalid hand-built Request
// falls back to the defaults entirely.
func ListRequest(r listquery.Request, s listquery.Spec) listquery.Request {
	out, err := listquery.New(r.Page, r.PageSize, r.Sort, r.Order, s)
	if err != nil {
		out, _ = listquery.New(0, 0, "", "", s)
	}
	return out
}
