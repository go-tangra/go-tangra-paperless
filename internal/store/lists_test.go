package store

import (
	"testing"

	"github.com/go-tangra/go-tangra/v4/listquery"
)

func TestDocumentListSpec(t *testing.T) {
	if err := DocumentList.Validate(); err != nil {
		t.Fatal(err)
	}
	if r := ListRequest(listquery.Request{}, DocumentList); r != (listquery.Request{Page: 1, PageSize: 25, Sort: "created_at", Order: listquery.Desc}) {
		t.Fatalf("zero request = %+v", r)
	}
	if r := ListRequest(listquery.Request{Page: 2, PageSize: 10, Sort: "name"}, DocumentList); r != (listquery.Request{Page: 2, PageSize: 10, Sort: "name", Order: listquery.Asc}) {
		t.Fatalf("partial request = %+v", r)
	}
	if r := ListRequest(listquery.Request{Page: 3, PageSize: 500, Sort: "content_text"}, DocumentList); r != (listquery.Request{Page: 1, PageSize: 25, Sort: "created_at", Order: listquery.Desc}) {
		t.Fatalf("invalid request = %+v", r)
	}
	if got := (listquery.Request{Sort: "name", Order: listquery.Asc}).OrderBy(DocumentList); got != "lower(name) ASC, id ASC" {
		t.Fatalf("name order = %s", got)
	}
	if got := (listquery.Request{Sort: "created_at", Order: listquery.Desc}).OrderBy(DocumentList); got != "created_at DESC, id DESC" {
		t.Fatalf("created_at order = %s", got)
	}
}
