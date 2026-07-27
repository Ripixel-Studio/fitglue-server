package parkrun

import "testing"

func TestNormalizeCountryHost(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"short code uk", "uk", "www.parkrun.org.uk"},
		{"bare apex uk", "parkrun.org.uk", "www.parkrun.org.uk"},
		{"already full uk", "www.parkrun.org.uk", "www.parkrun.org.uk"},
		{"empty", "", "www.parkrun.org.uk"},
		{"whitespace", "   ", "www.parkrun.org.uk"},
		{"uppercase short code", "UK", "www.parkrun.org.uk"},
		{"mixed case full host", "WWW.Parkrun.ORG.uk", "www.parkrun.org.uk"},
		{"short code de", "de", "www.parkrun.de"},
		{"short code au", "au", "www.parkrun.com.au"},
		{"short code nz", "nz", "www.parkrun.co.nz"},
		{"short code za", "za", "www.parkrun.co.za"},
		{"short code us", "us", "www.parkrun.us"},
		{"bare apex de", "parkrun.de", "www.parkrun.de"},
		{"bare apex com.au", "parkrun.com.au", "www.parkrun.com.au"},
		{"full host with scheme", "https://www.parkrun.ie/", "www.parkrun.ie"},
		{"apex with scheme", "http://parkrun.fr", "www.parkrun.fr"},
		{"full host with trailing path", "www.parkrun.se/somepath", "www.parkrun.se"},
		{"junk falls back to default", "not-a-host", "www.parkrun.org.uk"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeCountryHost(tc.in); got != tc.want {
				t.Errorf("NormalizeCountryHost(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBuildAthleteResultsURL_ValidURLFromStoredForms is the core regression test:
// every stored CountryUrl form must build the same valid parkrunner URL.
func TestBuildAthleteResultsURL_ValidURLFromStoredForms(t *testing.T) {
	const want = "https://www.parkrun.org.uk/parkrunner/12345/all/"
	for _, country := range []string{"uk", "parkrun.org.uk", "www.parkrun.org.uk", ""} {
		t.Run("country="+country, func(t *testing.T) {
			if got := BuildAthleteResultsURL("A12345", country); got != want {
				t.Errorf("BuildAthleteResultsURL(A12345, %q) = %q, want %q", country, got, want)
			}
		})
	}
}

func TestBuildAthleteResultsURL_StripsBarcodePrefix(t *testing.T) {
	got := BuildAthleteResultsURL("A98765", "de")
	want := "https://www.parkrun.de/parkrunner/98765/all/"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
