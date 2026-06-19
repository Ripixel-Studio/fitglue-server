package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueryParam(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?userId=abc&page_token=tok", nil)

	if got := queryParam(r, "userId", "user_id"); got != "abc" {
		t.Fatalf("camelCase first = %q, want abc", got)
	}
	// snake_case fallback when camelCase absent.
	if got := queryParam(r, "pageToken", "page_token"); got != "tok" {
		t.Fatalf("snake fallback = %q, want tok", got)
	}
	// none present → empty.
	if got := queryParam(r, "missing", "alsoMissing"); got != "" {
		t.Fatalf("missing = %q, want empty", got)
	}
}
