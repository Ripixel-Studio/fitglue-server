package parkrun

import (
	"fmt"
	"strings"
)

// DefaultCountryHost is the canonical fallback parkrun host used when a stored
// CountryUrl is empty or unrecognisable.
const DefaultCountryHost = "www.parkrun.org.uk"

// shortCodeHosts maps the short country codes that turn up in stored integration
// data (e.g. onboarding wrote "uk") to their canonical full parkrun host.
// The full list of supported hosts mirrors resolveTimezone in
// internal/pipeline/enricher/providers/parkrun/parkrun.go.
var shortCodeHosts = map[string]string{
	"uk": "www.parkrun.org.uk",
	"ie": "www.parkrun.ie",
	"de": "www.parkrun.de",
	"dk": "www.parkrun.dk",
	"fi": "www.parkrun.fi",
	"fr": "www.parkrun.fr",
	"it": "www.parkrun.it",
	"nl": "www.parkrun.nl",
	"no": "www.parkrun.no",
	"pl": "www.parkrun.pl",
	"se": "www.parkrun.se",
	"jp": "www.parkrun.jp",
	"sg": "www.parkrun.sg",
	"my": "www.parkrun.my",
	"nz": "www.parkrun.co.nz",
	"za": "www.parkrun.co.za",
	"au": "www.parkrun.com.au",
	"ca": "www.parkrun.ca",
	"us": "www.parkrun.us",
}

// NormalizeCountryHost turns a stored parkrun CountryUrl into a full, fetchable
// host. The field is written verbatim at onboarding time and comes in several
// forms — a short code ("uk"), a bare apex ("parkrun.org.uk"), an already-full
// host ("www.parkrun.org.uk"), or empty. All of these must resolve to a host
// that builds a valid https://<host>/parkrunner/<id>/all/ URL; without this the
// fetch hit junk like https://uk/parkrunner/... and silently found no results.
//
// Rules, in order:
//   - empty/whitespace                → DefaultCountryHost
//   - known short code (uk, de, ...)  → its canonical host
//   - already a www.parkrun.* host    → unchanged
//   - bare parkrun.* apex             → prepend "www."
//   - anything else (junk)            → DefaultCountryHost
func NormalizeCountryHost(countryURL string) string {
	h := strings.ToLower(strings.TrimSpace(countryURL))

	// Strip any scheme/path that may have crept in, keep just the host.
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}

	if h == "" {
		return DefaultCountryHost
	}
	if full, ok := shortCodeHosts[h]; ok {
		return full
	}
	if strings.HasPrefix(h, "www.parkrun.") {
		return h
	}
	if strings.HasPrefix(h, "parkrun.") {
		return "www." + h
	}
	// Unrecognised form — safer to fall back to the canonical UK host than to
	// build a URL against a host that will never serve results.
	return DefaultCountryHost
}

// BuildAthleteResultsURL builds the parkrunner "all results" URL for an athlete,
// normalizing the country host first. athleteID may be a barcode ("A12345") or
// the bare numeric id.
func BuildAthleteResultsURL(athleteID, countryURL string) string {
	athleteID = strings.TrimPrefix(athleteID, "A")
	return fmt.Sprintf("https://%s/parkrunner/%s/all/", NormalizeCountryHost(countryURL), athleteID)
}
