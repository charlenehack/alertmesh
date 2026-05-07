package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
	"strings"

	"github.com/gomarkdown/markdown"
	mdhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// EmailConfig is unmarshalled from notification_contacts.config.
//
//	{
//	  "smtp_host": "smtp.example.com",
//	  "smtp_port": 587,
//	  "username": "alerts@example.com",
//	  "password": "secret",
//	  "from": "alerts@example.com",
//	  "to": ["oncall@example.com"]
//	}
type EmailConfig struct {
	SMTPHost string   `json:"smtp_host"`
	SMTPPort int      `json:"smtp_port"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	From     string   `json:"from"`
	To       []string `json:"to"`
}

type EmailChannel struct{}

func (c *EmailChannel) Type() string { return "email" }

// SendBatched issues exactly one smtp.SendMail call covering every
// Recipient's Endpoint as the To list.  When the bucket has ≥2
// recipients this halves to 1/N the SMTP latency and connection cost
// vs. the v2 per-contact loop.
//
// The cfg is the shared SystemConfig SMTP credentials; per-contact
// overrides are NOT supported on purpose — running multiple SMTP
// servers per alert is operationally a foot-gun (auth races, sender-
// reputation drift, retry confusion).
func (c *EmailChannel) SendBatched(
	_ context.Context, msg Message, recipients []Recipient, config json.RawMessage,
) error {
	var cfg EmailConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return fmt.Errorf("email: invalid config: %w", err)
	}
	if cfg.SMTPHost == "" {
		return fmt.Errorf("email: smtp_host is required")
	}

	to := dedupNonEmpty(cfg.To)
	for _, r := range recipients {
		if e := strings.TrimSpace(r.Endpoint); e != "" {
			to = appendIfMissing(to, e)
		}
	}
	if len(to) == 0 {
		return fmt.Errorf("email: no recipient addresses")
	}
	cfg.To = to

	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587
	}

	body := buildEmailBody(msg, cfg)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)

	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SMTPHost)
	}

	return smtp.SendMail(addr, auth, cfg.From, cfg.To, []byte(body))
}

// buildEmailBody assembles a multipart/alternative email so MUAs can
// render the markdown-formatted incident message in either plain text
// (text/plain part = msg.Body verbatim — markdown is human-readable
// without rendering) or HTML (text/html part = gomarkdown-rendered
// from the same source).  Mirrors what DingTalk / Feishu / Slack do
// natively — keeps every channel rendering the same source-of-truth
// markdown body and avoids the historic plain-text-only email diverging
// from the IM channels.
//
// SMTP wire format requires CRLF on every header / boundary line; the
// part bodies themselves are written through multipart.Writer which
// handles the boundary wrapping for us.  Subject is RFC 2047-encoded so
// Chinese characters survive the SMTP transit.
func buildEmailBody(msg Message, cfg EmailConfig) string {
	var headers strings.Builder
	headers.WriteString(fmt.Sprintf("From: %s\r\n", cfg.From))
	headers.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(cfg.To, ",")))
	headers.WriteString(fmt.Sprintf("Subject: %s\r\n", mime.QEncoding.Encode("utf-8", msg.Title)))
	headers.WriteString("MIME-Version: 1.0\r\n")

	var partsBuf bytes.Buffer
	mw := multipart.NewWriter(&partsBuf)

	// text/plain part — raw markdown.  Most non-MUA tooling (curl, less,
	// log forwarders) will see this; markdown renders fine as-is.
	plainHdr := textproto.MIMEHeader{}
	plainHdr.Set("Content-Type", "text/plain; charset=UTF-8")
	plainHdr.Set("Content-Transfer-Encoding", "8bit")
	if w, err := mw.CreatePart(plainHdr); err == nil {
		writePlainEmailPart(w, msg)
	}

	// text/html part — gomarkdown-rendered.  Wrapped in a minimal
	// html/body envelope so MUAs that demand a <html> root (older
	// Outlook) don't silently fall back to the plain part.
	htmlHdr := textproto.MIMEHeader{}
	htmlHdr.Set("Content-Type", "text/html; charset=UTF-8")
	htmlHdr.Set("Content-Transfer-Encoding", "8bit")
	if w, err := mw.CreatePart(htmlHdr); err == nil {
		writeHTMLEmailPart(w, msg)
	}

	_ = mw.Close()

	headers.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q\r\n", mw.Boundary()))
	headers.WriteString("\r\n")

	return headers.String() + partsBuf.String()
}

// writePlainEmailPart emits the raw markdown body plus a trailing
// "View: <url>" pointer.  The body is intentionally NOT prefixed with
// "Severity: ..." — that's already in msg.Title (which is also the
// Subject header) so repeating it here would echo the IM-channel
// duplication we just removed.
func writePlainEmailPart(w interface{ Write([]byte) (int, error) }, msg Message) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Incident ID: %s\n\n", msg.IncidentID))
	sb.WriteString(msg.Body)
	sb.WriteString("\n")
	if msg.URL != "" {
		sb.WriteString(fmt.Sprintf("\nView: %s\n", msg.URL))
	}
	_, _ = w.Write([]byte(sb.String()))
}

// writeHTMLEmailPart renders the markdown body to HTML using gomarkdown
// with the standard CommonExtensions set (tables, fenced code, etc.).
// We deliberately avoid CSS inlining / theming — strict HTML lets every
// MUA apply its own default styling and keeps our binary tiny.
func writeHTMLEmailPart(w interface{ Write([]byte) (int, error) }, msg Message) {
	p := parser.NewWithExtensions(parser.CommonExtensions | parser.AutoHeadingIDs)
	renderer := mdhtml.NewRenderer(mdhtml.RendererOptions{
		Flags: mdhtml.CommonFlags | mdhtml.HrefTargetBlank,
	})
	rendered := markdown.ToHTML([]byte(msg.Body), p, renderer)

	var sb strings.Builder
	sb.WriteString(`<!doctype html><html><body>` + "\n")
	sb.WriteString(fmt.Sprintf("<p><strong>Incident ID:</strong> %s</p>\n", msg.IncidentID))
	sb.Write(rendered)
	if msg.URL != "" {
		sb.WriteString(fmt.Sprintf("<p><a href=%q target=\"_blank\">查看事件</a></p>\n", msg.URL))
	}
	sb.WriteString("</body></html>\n")
	_, _ = w.Write([]byte(sb.String()))
}

// dedupNonEmpty returns the input with empty strings dropped and
// duplicates collapsed, preserving first-seen order.
func dedupNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// appendIfMissing appends s to the slice unless it's already present
// (case-insensitive for emails to defend against operator-typo dups).
func appendIfMissing(out []string, s string) []string {
	cmp := strings.ToLower(s)
	for _, x := range out {
		if strings.ToLower(x) == cmp {
			return out
		}
	}
	return append(out, s)
}
