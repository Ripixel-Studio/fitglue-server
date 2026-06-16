package showcaseview

import (
	"testing"
	"time"
)

func TestKeys(t *testing.T) {
	if got := ActivityKey("abc"); got != "activity:abc" {
		t.Errorf("ActivityKey = %q", got)
	}
	if got := ProfileKey("jane"); got != "profile:jane" {
		t.Errorf("ProfileKey = %q", got)
	}
	if got := RoundupKey("jane", "week-23-2025"); got != "roundup:jane:week-23-2025" {
		t.Errorf("RoundupKey = %q", got)
	}
}

func TestDailySaltRotatesDaily(t *testing.T) {
	secret := "s3cr3t"
	d1 := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	d1Later := time.Date(2026, 6, 16, 23, 30, 0, 0, time.UTC)
	d2 := time.Date(2026, 6, 17, 0, 1, 0, 0, time.UTC)

	if DailySalt(secret, d1) != DailySalt(secret, d1Later) {
		t.Error("salt should be stable within the same UTC day")
	}
	if DailySalt(secret, d1) == DailySalt(secret, d2) {
		t.Error("salt should rotate across UTC days")
	}
}

func TestVisitorHashDeterministicAndSensitive(t *testing.T) {
	secret := "s3cr3t"
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	base := VisitorHash(secret, "1.2.3.4", "Mozilla/5.0", "activity:abc", now)

	if base != VisitorHash(secret, "1.2.3.4", "Mozilla/5.0", "activity:abc", now) {
		t.Error("same inputs must produce the same hash")
	}

	cases := map[string]string{
		"different ip":     VisitorHash(secret, "9.9.9.9", "Mozilla/5.0", "activity:abc", now),
		"different ua":     VisitorHash(secret, "1.2.3.4", "Safari", "activity:abc", now),
		"different target": VisitorHash(secret, "1.2.3.4", "Mozilla/5.0", "activity:xyz", now),
		"different day":    VisitorHash(secret, "1.2.3.4", "Mozilla/5.0", "activity:abc", now.Add(24*time.Hour)),
		"different secret": VisitorHash("other", "1.2.3.4", "Mozilla/5.0", "activity:abc", now),
	}
	for name, h := range cases {
		if h == base {
			t.Errorf("hash should differ for %s", name)
		}
	}
}

func TestVisitorHashNoFieldBleed(t *testing.T) {
	secret := "s"
	now := time.Now()
	// "ab" + "c" must not collide with "a" + "bc" thanks to the NUL separators.
	if VisitorHash(secret, "ab", "c", "k", now) == VisitorHash(secret, "a", "bc", "k", now) {
		t.Error("field boundaries should not collide")
	}
}

func TestIsBot(t *testing.T) {
	bots := []string{
		"",
		"   ",
		"Googlebot/2.1 (+http://www.google.com/bot.html)",
		"facebookexternalhit/1.1",
		"Twitterbot/1.0",
		"WhatsApp/2.0",
		"Slackbot-LinkExpanding 1.0",
		"curl/8.4.0",
		"python-requests/2.31",
	}
	for _, ua := range bots {
		if !IsBot(ua) {
			t.Errorf("expected bot for %q", ua)
		}
	}

	humans := []string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/124.0 Safari/537.36",
	}
	for _, ua := range humans {
		if IsBot(ua) {
			t.Errorf("expected human for %q", ua)
		}
	}
}
