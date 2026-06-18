package email

import (
	"strings"
	"testing"
)

const baseURL = "https://fitglue.tech"

func TestRenderLayout(t *testing.T) {
	out := RenderLayout(LayoutOptions{
		Content:     "<p>hello</p>",
		PreviewText: "preview here",
		BaseURL:     baseURL,
	})
	if !strings.Contains(out, "<p>hello</p>") {
		t.Error("layout should embed content")
	}
	if !strings.Contains(out, "preview here") {
		t.Error("layout should embed preview text")
	}
	if !strings.Contains(out, "<!DOCTYPE html>") {
		t.Error("layout should be a full HTML document")
	}
}

func TestLayoutHelpers(t *testing.T) {
	if !strings.Contains(ctaButton("Click", "https://x"), "https://x") {
		t.Error("ctaButton should embed url")
	}
	if !strings.Contains(heading("Title"), "Title") {
		t.Error("heading")
	}
	if !strings.Contains(paragraph("Body"), "Body") {
		t.Error("paragraph")
	}
	if !strings.Contains(smallText("note"), "note") {
		t.Error("smallText")
	}
	if divider() == "" {
		t.Error("divider should produce markup")
	}
	if !strings.Contains(emoji("🔥"), "🔥") {
		t.Error("emoji")
	}
	if got := joinContent("a", "b"); !strings.Contains(got, "a") || !strings.Contains(got, "b") {
		t.Error("joinContent should concatenate parts")
	}
}

// TestTemplates exercises every template function, asserting it returns a
// full HTML document that embeds its dynamic inputs.
func TestTemplates(t *testing.T) {
	contains := func(t *testing.T, name, out string, subs ...string) {
		t.Helper()
		if !strings.Contains(out, "<!DOCTYPE html>") {
			t.Errorf("%s: expected full HTML document", name)
		}
		for _, s := range subs {
			if !strings.Contains(out, s) {
				t.Errorf("%s: expected output to contain %q", name, s)
			}
		}
	}

	contains(t, "VerifyEmail", VerifyEmailTemplate("https://verify", baseURL), "https://verify")
	contains(t, "PasswordReset", PasswordResetTemplate("https://reset", baseURL), "https://reset")
	contains(t, "Welcome", WelcomeTemplate(baseURL))
	contains(t, "ChangeEmail", ChangeEmailTemplate("https://verify", "new@x.com", baseURL), "new@x.com")
	contains(t, "DataExport", DataExportTemplate("https://download", baseURL), "https://download")
	contains(t, "TrialExpiring", TrialExpiringTemplate(3, baseURL), "3")
	contains(t, "TrialExpired", TrialExpiredTemplate(baseURL))
	contains(t, "SubscriptionConfirmation", SubscriptionConfirmationTemplate(baseURL))
	contains(t, "PaymentFailed", PaymentFailedTemplate(baseURL))
	contains(t, "SubscriptionCancelled", SubscriptionCancelledTemplate(baseURL))
	contains(t, "AccessGranted", AccessGrantedTemplate(baseURL))
	contains(t, "ActivitySynced", ActivitySyncedTemplate("Morning Run", "Strava", "https://act", baseURL), "Morning Run", "Strava")
	contains(t, "PipelineFailure", PipelineFailureTemplate("Run", "boom", "https://act", baseURL), "boom")
	contains(t, "PendingInput", PendingInputTemplate("https://act", baseURL))
	contains(t, "ConnectionAction", ConnectionActionTemplate("Strava", "https://conn", baseURL), "Strava")
	// ShowcaseRoundupTemplate lowercases the period for display.
	contains(t, "ShowcaseRoundup", ShowcaseRoundupTemplate("January", "great month", "https://roundup", baseURL), "january", "great month")
	contains(t, "PipelineCancelled", PipelineCancelledTemplate(baseURL))

	users := []RegistrationSummaryUser{
		{Email: "a@x.com", AccessEnabled: true, CreatedAt: "2024-01-01"},
		{Email: "b@x.com", AccessEnabled: false, CreatedAt: "2024-01-02"},
	}
	contains(t, "RegistrationSummary", RegistrationSummaryTemplate("2024-01-02", users, baseURL), "a@x.com", "b@x.com")
}

func TestTrialExpiringTemplate_SingularDay(t *testing.T) {
	// Exercises the day vs days branch.
	one := TrialExpiringTemplate(1, baseURL)
	many := TrialExpiringTemplate(5, baseURL)
	if one == many {
		t.Error("expected different copy for 1 day vs 5 days")
	}
}
