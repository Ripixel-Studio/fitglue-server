// Package showcaseview holds the shared, dependency-free primitives for
// showcase view metrics: the Firestore target-key format, the privacy-friendly
// daily-rotating visitor hash, and bot detection. The api-public gateway (which
// writes views) and the activity service (which reads them for owners) both
// depend on this package so the key format and hashing can never drift.
package showcaseview

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// ActivityKey returns the view-metrics key for a single showcased activity.
func ActivityKey(showcaseID string) string { return "activity:" + showcaseID }

// ProfileKey returns the view-metrics key for an athlete's profile page.
func ProfileKey(slug string) string { return "profile:" + slug }

// RoundupKey returns the view-metrics key for a roundup page.
func RoundupKey(slug, periodKey string) string { return "roundup:" + slug + ":" + periodKey }

// DailySalt derives a deterministic per-UTC-day salt from secret. It is
// deterministic across gateway instances (so de-dup works while Cloud Run
// scales horizontally) yet rotates every day, so stored visitor hashes are not
// durable identifiers.
func DailySalt(secret string, now time.Time) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(now.UTC().Format("2006-01-02")))
	return hex.EncodeToString(mac.Sum(nil))
}

// VisitorHash binds a visitor to a specific target for a single UTC day. The
// raw IP and user-agent never leave the gateway — only this one-way hash is
// persisted, and it expires daily via the rotating salt.
func VisitorHash(secret, ip, userAgent, targetKey string, now time.Time) string {
	salt := DailySalt(secret, now)
	h := sha256.New()
	// NUL separators prevent field-boundary collisions.
	for _, part := range []string{salt, ip, userAgent, targetKey} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// botSignatures are lower-cased substrings that identify automated clients,
// including the social-card unfurlers that fetch showcase OG metadata. Most
// crawlers never run the page's JavaScript and so never fire the beacon at all;
// this is defence-in-depth for JS-capable and prefetching agents.
var botSignatures = []string{
	"bot", "crawl", "spider", "slurp", "mediapartners",
	"facebookexternalhit", "facebot", "twitterbot", "linkedinbot",
	"whatsapp", "telegrambot", "discordbot", "slackbot", "pinterest",
	"embedly", "redditbot", "applebot", "bingpreview", "vkshare",
	"google-inspectiontool", "headlesschrome", "phantomjs", "python-requests",
	"curl/", "wget/", "go-http-client", "axios/", "node-fetch", "preview",
	"lighthouse", "pingdom", "uptimerobot", "monitor",
}

// IsBot reports whether userAgent looks like an automated client. An empty
// user-agent is treated as a bot.
func IsBot(userAgent string) bool {
	if strings.TrimSpace(userAgent) == "" {
		return true
	}
	ua := strings.ToLower(userAgent)
	for _, sig := range botSignatures {
		if strings.Contains(ua, sig) {
			return true
		}
	}
	return false
}
