package parkrun

import (
	"os"
	"testing"
)

// The fixture is the exact page the Cloud Run fetcher received from
// parkrun.org.uk on 2026-08-24: a 9.9KB AWS WAF captcha interstitial that
// parses to zero result rows.
func TestIsBotChallenge_RealWAFPage(t *testing.T) {
	html, err := os.ReadFile("testdata/aws_waf_challenge.html")
	if err != nil {
		t.Fatal(err)
	}
	if !IsBotChallenge(string(html)) {
		t.Fatal("real AWS WAF captcha page not detected as bot challenge")
	}
}

func TestIsBotChallenge_ResultsPageIsNot(t *testing.T) {
	html := `<html><head><title>results | parkrun UK</title></head><body>
<table><tbody><tr><td><a href="https://www.parkrun.org.uk/newark/results/">Newark</a></td>
<td>22/08/2026</td><td>568</td><td>617</td><td>43:46</td><td>29.97%</td><td></td></tr></tbody></table>
</body></html>`
	if IsBotChallenge(html) {
		t.Fatal("results page wrongly flagged as bot challenge")
	}
	if IsBotChallenge("") {
		t.Fatal("empty page flagged as bot challenge")
	}
}
