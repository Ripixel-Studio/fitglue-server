package server

import "net/http"

// queryParam returns the first non-empty query value among the provided names.
// Used to accept both the generated OpenAPI camelCase parameter names and the
// underlying snake_case proto field names.
func queryParam(r *http.Request, names ...string) string {
	q := r.URL.Query()
	for _, name := range names {
		if v := q.Get(name); v != "" {
			return v
		}
	}
	return ""
}
