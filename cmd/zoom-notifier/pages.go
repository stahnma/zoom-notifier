package main

import (
	"fmt"
	"net/http"
)

// legalPageStyle is the shared stylesheet for the privacy and terms pages.
// It mirrors the landing page look so the site feels consistent.
const legalPageStyle = `
  body { font-family: system-ui, -apple-system, sans-serif; max-width: 560px; margin: 2rem auto; padding: 0 1rem; color: #333; background: #fafafa; line-height: 1.5; }
  .container { background: #fff; border: 1px solid #ddd; border-radius: 8px; padding: 2rem; }
  h1 { color: #1a73e8; margin-bottom: 0.25rem; }
  h3 { margin-top: 1.5rem; margin-bottom: 0.5rem; color: #555; }
  ul { padding-left: 1.25rem; }
  li { margin: 0.4rem 0; }
  a { color: #1a73e8; text-decoration: none; font-weight: 500; }
  a:hover { text-decoration: underline; }
  .footer { border-top: 1px solid #eee; margin-top: 1.5rem; padding-top: 1rem; font-size: 0.9rem; color: #666; }
`

// handlePrivacy serves a plain-English privacy policy.
func handlePrivacy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>zoom-notifier Privacy Policy</title>
<style>`+legalPageStyle+`</style>
</head>
<body>
<div class="container">
  <h1>Privacy Policy</h1>
  <p>This is short because there isn't much to say.</p>

  <h3>What we store</h3>
  <p>Only what the app needs to do its job:</p>
  <ul>
    <li>Your Slack workspace ID and name, and the bot token Slack gives us when you install the app.</li>
    <li>The Slack user IDs of your admins and the channels you subscribe.</li>
    <li>The filters and settings you configure.</li>
    <li>Your Zoom account ID and any Zoom credentials you enter during setup.</li>
    <li>While a Zoom meeting is running: the meeting topic and the names of people who join or leave. This is deleted when the meeting ends.</li>
  </ul>

  <h3>What we do with it</h3>
  <p>We use it to send meeting notifications to your Slack channels. That's it. We may also look at logs and usage to fix bugs and make the product better.</p>

  <h3>What we don't do</h3>
  <ul>
    <li>We don't sell your data.</li>
    <li>We don't share it with anyone else.</li>
    <li>We don't use it for advertising.</li>
  </ul>

  <h3>Removing your data</h3>
  <p>Uninstall the app from your Slack workspace and ask us to delete your tenant. We'll remove everything we have for your workspace.</p>

  <div class="footer">
    <a href="/">Home</a> &middot; <a href="/terms">Terms of Service</a>
  </div>
</div>
</body>
</html>`)
}

// handleTerms serves a plain-English terms of service page.
func handleTerms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>zoom-notifier Terms of Service</title>
<style>`+legalPageStyle+`</style>
</head>
<body>
<div class="container">
  <h1>Terms of Service</h1>
  <p>Also short.</p>

  <h3>The deal</h3>
  <ul>
    <li>zoom-notifier is provided as-is, free of charge, with no guarantees it will work, keep working, or be available at any particular time.</li>
    <li>You're responsible for the Zoom and Slack accounts you connect and for having permission to connect them.</li>
    <li>Don't use the service to harass anyone, break the law, or abuse Zoom, Slack, or this server.</li>
  </ul>

  <h3>Getting cut off</h3>
  <p>If you're doing something we don't like, we can suspend or remove your workspace from the service at any time, without notice. We'll try to be reasonable about it.</p>

  <h3>Changes</h3>
  <p>We may update these terms. If we do, the new version will be posted here.</p>

  <div class="footer">
    <a href="/">Home</a> &middot; <a href="/privacy">Privacy Policy</a>
  </div>
</div>
</body>
</html>`)
}
