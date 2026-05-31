package email

import (
	"fmt"
	"strings"
)

func VerifyEmailTemplate(verificationURL, baseURL string) string {
	content := joinContent(
		emoji("✉️"),
		heading("Verify your email address"),
		paragraph("Thanks for signing up for FitGlue! Please confirm your email address by clicking the button below."),
		ctaButton("Verify Email", verificationURL),
		paragraph("This link will expire in 24 hours. If you didn't create a FitGlue account, you can safely ignore this email."),
		divider(),
		smallText(fmt.Sprintf(`If the button doesn't work, copy and paste this URL into your browser: <a href="%s" style="color:%s;word-break:break-all;">%s</a>`, verificationURL, Brand.Primary, verificationURL)),
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: "Verify your email to start using FitGlue",
		Content:     content,
	})
}

func PasswordResetTemplate(resetURL, baseURL string) string {
	content := joinContent(
		emoji("🔐"),
		heading("Reset your password"),
		paragraph("We received a request to reset your FitGlue password. Click the button below to choose a new password."),
		ctaButton("Reset Password", resetURL),
		paragraph("This link will expire in 1 hour. If you didn't request a password reset, you can safely ignore this email — your password won't be changed."),
		divider(),
		smallText(fmt.Sprintf(`If the button doesn't work, copy and paste this URL into your browser: <a href="%s" style="color:%s;word-break:break-all;">%s</a>`, resetURL, Brand.Primary, resetURL)),
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: "Reset your FitGlue password",
		Content:     content,
	})
}

func WelcomeTemplate(baseURL string) string {
	dashboardURL := fmt.Sprintf("%s/app", baseURL)

	stepsHTML := fmt.Sprintf(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="margin:24px 0;">
  <tr>
    <td style="background:%s;border-radius:10px;padding:20px 24px;">
      <p style="color:%s;font-size:15px;font-weight:600;margin:0 0 12px;">Here's what you can do next:</p>
      <table role="presentation" cellpadding="0" cellspacing="0" border="0">
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">🔗 Connect Strava, Fitbit, Hevy, and more</td></tr>
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">⚡ Set up automated pipelines with boosters</td></tr>
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">🏆 Share your showcase profile with friends</td></tr>
      </table>
    </td>
  </tr>
</table>`, Brand.BgBody, Brand.TextPrimary, Brand.TextSecondary, Brand.TextSecondary, Brand.TextSecondary)

	content := joinContent(
		emoji("🎉"),
		heading("Welcome to FitGlue!"),
		paragraph("Your email has been verified and your account is all set. You're ready to start connecting your fitness services and building powerful pipelines."),
		stepsHTML,
		paragraph(fmt.Sprintf(`You have a <strong style="color:%s;">30-day free trial</strong> of our Athlete tier, giving you full access to all features.`, Brand.TextPrimary)),
		ctaButton("Go to Dashboard", dashboardURL),
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: "Welcome to FitGlue — your fitness data, your way",
		Content:     content,
	})
}

func ChangeEmailTemplate(verificationURL, newEmail, baseURL string) string {
	content := joinContent(
		emoji("📧"),
		heading("Confirm your new email"),
		paragraph(fmt.Sprintf(`You requested to change your FitGlue email address to <strong style="color:%s;">%s</strong>. Please confirm this change by clicking the button below.`, Brand.TextPrimary, newEmail)),
		ctaButton("Confirm Email Change", verificationURL),
		paragraph("If you didn't request this change, please ignore this email and your account will remain unchanged. You may also want to update your password for security."),
		divider(),
		smallText(fmt.Sprintf(`If the button doesn't work, copy and paste this URL into your browser: <a href="%s" style="color:%s;word-break:break-all;">%s</a>`, verificationURL, Brand.Primary, verificationURL)),
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: "Confirm your new FitGlue email address",
		Content:     content,
	})
}

func DataExportTemplate(downloadURL, baseURL string) string {
	content := joinContent(
		emoji("📦"),
		heading("Your data export is ready"),
		paragraph("Your FitGlue data export has been prepared and is ready to download. The file contains all of your account data in JSON format."),
		ctaButton("Download My Data", downloadURL),
		paragraph(fmt.Sprintf(`This download link will expire in <strong style="color:%s;">24 hours</strong>. If you didn't request this export, please contact us at <a href="mailto:support@fitglue.tech" style="color:%s;">support@fitglue.tech</a>.`, Brand.TextPrimary, Brand.Primary)),
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: "Your FitGlue data export is ready to download",
		Content:     content,
	})
}

func TrialExpiringTemplate(daysLeft int, baseURL string) string {
	upgradeURL := fmt.Sprintf("%s/app/subscription", baseURL)

	s := ""
	if daysLeft != 1 {
		s = "s"
	}

	featuresHTML := fmt.Sprintf(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="margin:24px 0;">
  <tr>
    <td style="background:%s;border-radius:10px;padding:20px 24px;">
      <p style="color:%s;font-size:15px;font-weight:600;margin:0 0 12px;">What you'll keep on Hobbyist:</p>
      <table role="presentation" cellpadding="0" cellspacing="0" border="0">
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">✅ 1 active pipeline</td></tr>
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">✅ Basic boosters</td></tr>
      </table>
      <p style="color:%s;font-size:15px;font-weight:600;margin:16px 0 12px;">What requires Athlete:</p>
      <table role="presentation" cellpadding="0" cellspacing="0" border="0">
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">🔒 Unlimited pipelines</td></tr>
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">🔒 Premium boosters &amp; integrations</td></tr>
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">🔒 Showcase profile</td></tr>
      </table>
    </td>
  </tr>
</table>`, Brand.BgBody, Brand.TextPrimary, Brand.TextSecondary, Brand.TextSecondary, Brand.TextPrimary, Brand.TextSecondary, Brand.TextSecondary, Brand.TextSecondary)

	content := joinContent(
		emoji("⏳"),
		heading(fmt.Sprintf("Your trial ends in %d day%s", daysLeft, s)),
		paragraph("Your free Athlete trial is coming to an end. After it expires, your account will switch to our Hobbyist tier, and some features will be limited."),
		featuresHTML,
		ctaButton("Upgrade to Athlete", upgradeURL),
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: fmt.Sprintf("Your FitGlue Athlete trial ends in %d day%s", daysLeft, s),
		Content:     content,
	})
}

func TrialExpiredTemplate(baseURL string) string {
	upgradeURL := fmt.Sprintf("%s/app/subscription", baseURL)

	content := joinContent(
		emoji("⌛"),
		heading("Your Athlete trial has ended"),
		paragraph("Your 30-day Athlete trial has expired and your account has been moved to our free Hobbyist tier. Your data is safe — nothing has been deleted."),
		paragraph("Upgrade anytime to unlock all Athlete features again, including unlimited pipelines, premium boosters, and your showcase profile."),
		ctaButton("Upgrade to Athlete", upgradeURL),
		divider(),
		smallText(fmt.Sprintf(`If you have any questions about plans or pricing, reach out to us at <a href="mailto:support@fitglue.tech" style="color:%s;">support@fitglue.tech</a>.`, Brand.Primary)),
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: "Your FitGlue Athlete trial has ended",
		Content:     content,
	})
}

func SubscriptionConfirmationTemplate(baseURL string) string {
	manageURL := fmt.Sprintf("%s/app/subscription", baseURL)

	benefitsHTML := fmt.Sprintf(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="margin:24px 0;">
  <tr>
    <td style="background:%s;border-radius:10px;padding:20px 24px;">
      <p style="color:%s;font-size:15px;font-weight:600;margin:0 0 12px;">Your Athlete benefits:</p>
      <table role="presentation" cellpadding="0" cellspacing="0" border="0">
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">⚡ Unlimited pipelines &amp; syncs</td></tr>
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">🤖 Premium AI &amp; image boosters</td></tr>
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">🏆 Showcase profile &amp; permanent activity history</td></tr>
        <tr><td style="padding:4px 0;color:%s;font-size:14px;">🔗 All integrations &amp; destinations</td></tr>
      </table>
    </td>
  </tr>
</table>`, Brand.BgBody, Brand.TextPrimary, Brand.TextSecondary, Brand.TextSecondary, Brand.TextSecondary, Brand.TextSecondary)

	content := joinContent(
		emoji("🎉"),
		heading("You're now an Athlete!"),
		paragraph("Your subscription is confirmed. Welcome to FitGlue Athlete — you now have full access to all features."),
		benefitsHTML,
		ctaButton("Go to Dashboard", manageURL),
		divider(),
		smallText(fmt.Sprintf(`To manage your subscription, visit your <a href="%s" style="color:%s;">subscription page</a>. Questions? Email <a href="mailto:support@fitglue.tech" style="color:%s;">support@fitglue.tech</a>.`, manageURL, Brand.Primary, Brand.Primary)),
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: "Your FitGlue Athlete subscription is confirmed",
		Content:     content,
	})
}

func PaymentFailedTemplate(baseURL string) string {
	updateURL := fmt.Sprintf("%s/app/subscription", baseURL)

	content := joinContent(
		emoji("⚠️"),
		heading("Payment failed — action required"),
		paragraph("We were unable to process your FitGlue Athlete subscription payment. Your access has not been affected yet, but we'll retry the charge over the next few days."),
		paragraph(fmt.Sprintf(`Please update your payment method to avoid any interruption to your Athlete benefits.`, )),
		ctaButton("Update Payment Method", updateURL),
		divider(),
		smallText(fmt.Sprintf(`If your payment continues to fail, your subscription will be cancelled and your account will revert to the Hobbyist tier. Questions? Contact us at <a href="mailto:support@fitglue.tech" style="color:%s;">support@fitglue.tech</a>.`, Brand.Primary)),
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: "Action required: your FitGlue payment failed",
		Content:     content,
	})
}

func SubscriptionCancelledTemplate(baseURL string) string {
	upgradeURL := fmt.Sprintf("%s/app/subscription", baseURL)

	content := joinContent(
		emoji("👋"),
		heading("Your Athlete subscription has ended"),
		paragraph("Your FitGlue Athlete subscription has been cancelled and your account has moved to the free Hobbyist tier. Your data and activity history are safe — nothing has been deleted."),
		paragraph("You can re-subscribe at any time to regain full Athlete access."),
		ctaButton("Re-subscribe to Athlete", upgradeURL),
		divider(),
		smallText(fmt.Sprintf(`If you cancelled by mistake or have questions, reach out at <a href="mailto:support@fitglue.tech" style="color:%s;">support@fitglue.tech</a>.`, Brand.Primary)),
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: "Your FitGlue Athlete subscription has ended",
		Content:     content,
	})
}

// RegistrationSummaryUser represents a user row in the admin email
type RegistrationSummaryUser struct {
	Email         string
	AccessEnabled bool
	CreatedAt     string
}

func RegistrationSummaryTemplate(dateStr string, users []RegistrationSummaryUser, baseURL string) string {
	if len(users) == 0 {
		content := joinContent(
			emoji("📊"),
			heading("Daily Registration Summary"),
			paragraph(fmt.Sprintf(`<strong style="color:%s;">Date:</strong> %s`, Brand.TextPrimary, dateStr)),
			paragraph("No new registrations in the last 24 hours."),
		)
		return RenderLayout(LayoutOptions{
			BaseURL:     baseURL,
			PreviewText: fmt.Sprintf("No new registrations — %s", dateStr),
			Content:     content,
		})
	}

	waitingCount := 0
	enabledCount := 0
	for _, u := range users {
		if u.AccessEnabled {
			enabledCount++
		} else {
			waitingCount++
		}
	}

	limit := len(users)
	moreNote := ""
	if limit > 50 {
		limit = 50
		moreNote = fmt.Sprintf(`<p style="color:%s;font-size:13px;margin-top:8px;">... and %d more</p>`, Brand.TextMuted, len(users)-50)
	}

	var userRows strings.Builder
	for i := 0; i < limit; i++ {
		u := users[i]
		statusHTML := `<span style="color:#f59e0b;font-weight:600;">⏳ Waiting</span>`
		if u.AccessEnabled {
			statusHTML = `<span style="color:#10b981;font-weight:600;">✓ Enabled</span>`
		}

		userRows.WriteString(fmt.Sprintf(`<tr>
  <td style="padding:10px 12px;border-bottom:1px solid %s;color:%s;font-size:14px;">%s</td>
  <td style="padding:10px 12px;border-bottom:1px solid %s;font-size:14px;">%s</td>
  <td style="padding:10px 12px;border-bottom:1px solid %s;color:%s;font-size:13px;">%s</td>
</tr>`, Brand.Border, Brand.TextPrimary, u.Email, Brand.Border, statusHTML, Brand.Border, Brand.TextMuted, u.CreatedAt))
	}

	s := ""
	if len(users) != 1 {
		s = "s"
	}

	statsHTML := fmt.Sprintf(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="margin:24px 0;">
  <tr>
    <td style="background:%s;border-radius:10px;padding:20px 24px;width:33%%;text-align:center;">
      <p style="color:%s;font-size:28px;font-weight:800;margin:0;">%d</p>
      <p style="color:%s;font-size:12px;margin:4px 0 0;text-transform:uppercase;letter-spacing:0.05em;">Total</p>
    </td>
    <td style="width:8px;"></td>
    <td style="background:%s;border-radius:10px;padding:20px 24px;width:33%%;text-align:center;">
      <p style="color:#f59e0b;font-size:28px;font-weight:800;margin:0;">%d</p>
      <p style="color:%s;font-size:12px;margin:4px 0 0;text-transform:uppercase;letter-spacing:0.05em;">Waiting</p>
    </td>
    <td style="width:8px;"></td>
    <td style="background:%s;border-radius:10px;padding:20px 24px;width:33%%;text-align:center;">
      <p style="color:#10b981;font-size:28px;font-weight:800;margin:0;">%d</p>
      <p style="color:%s;font-size:12px;margin:4px 0 0;text-transform:uppercase;letter-spacing:0.05em;">Enabled</p>
    </td>
  </tr>
</table>`, Brand.BgBody, Brand.Primary, len(users), Brand.TextMuted, Brand.BgBody, waitingCount, Brand.TextMuted, Brand.BgBody, enabledCount, Brand.TextMuted)

	tableHTML := fmt.Sprintf(`<table role="presentation" cellpadding="0" cellspacing="0" border="0" width="100%%" style="border:1px solid %s;border-radius:8px;overflow:hidden;">
  <tr style="background:%s;">
    <th style="padding:10px 12px;text-align:left;font-size:12px;color:%s;text-transform:uppercase;letter-spacing:0.05em;font-weight:600;">Email</th>
    <th style="padding:10px 12px;text-align:left;font-size:12px;color:%s;text-transform:uppercase;letter-spacing:0.05em;font-weight:600;">Status</th>
    <th style="padding:10px 12px;text-align:left;font-size:12px;color:%s;text-transform:uppercase;letter-spacing:0.05em;font-weight:600;">Registered</th>
  </tr>
  %s
</table>`, Brand.Border, Brand.BgBody, Brand.TextMuted, Brand.TextMuted, Brand.TextMuted, userRows.String())

	content := joinContent(
		emoji("📊"),
		heading("Daily Registration Summary"),
		paragraph(fmt.Sprintf(`<strong style="color:%s;">Date:</strong> %s`, Brand.TextPrimary, dateStr)),
		statsHTML,
		tableHTML,
		moreNote,
	)

	return RenderLayout(LayoutOptions{
		BaseURL:     baseURL,
		PreviewText: fmt.Sprintf("%d new registration%s — %s", len(users), s, dateStr),
		Content:     content,
	})
}
