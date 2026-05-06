package email

import (
	"fmt"
	"html"
	"strings"
)

const (
	primaryColor = "#1b8d73"
	accentColor  = "#f57e29"
	baseBGColor  = "#f5f2ee"
	cardBGColor  = "#ffffff"
	textColor    = "#1f2933"
	mutedColor   = "#6b7280"
)

func LoginCodeSubject(language string) string {
	if language == "en" {
		return "Your yalinli.org login code"
	}
	return "yalinli.org giris kodunuz"
}

func LoginCodeContent(language, code string, ttlMinutes int, siteURL, siteName string) Content {
	if language == "en" {
		text := fmt.Sprintf(`Hello,

You are receiving this email because a login was requested on %s.

Your %s login code is:

%s

This code is valid for %d minutes. If you did not request this email, you can safely ignore it.

Visit %s: %s

%s
Dosmealti / Antalya / Yalinli neighborhood platform`, siteName, siteName, code, ttlMinutes, siteName, siteURL, siteName)
		htmlBody := loginCodeHTML("Hello", "You are receiving this email because a login was requested on", fmt.Sprintf("This code is valid for %d minutes.", ttlMinutes), "If you did not request this email, you can safely ignore it.", code, siteURL, siteName)
		return Content{Text: text, HTML: htmlBody}
	}
	text := fmt.Sprintf(`Merhaba,

Bu e-posta, %s uzerinden giris istegi yapildigi icin gonderilmistir.

%s giris kodunuz:

%s

Bu kod %d dakika boyunca gecerlidir. Bu e-postayi siz talep etmediyseniz guvenle yok sayabilirsiniz.

%s icin: %s

%s
Dosemealti / Antalya / Yalinli mahalle platformu`, siteName, siteName, code, ttlMinutes, siteName, siteURL, siteName)
	htmlBody := loginCodeHTML("Merhaba", "Bu e-posta, giris istegi yapildigi icin gonderilmistir", fmt.Sprintf("Bu kod %d dakika boyunca gecerlidir.", ttlMinutes), "Bu e-postayi siz talep etmediyseniz guvenle yok sayabilirsiniz.", code, siteURL, siteName)
	return Content{Text: text, HTML: htmlBody}
}

func TestEmailSubject() string {
	return "yalinli.org test email"
}

func TestEmailContent(siteURL, siteName string) Content {
	text := fmt.Sprintf(`Merhaba,

Bu %s API tarafindan gonderilen test e-postasidir.

Hello,

This is a test email sent by the %s API.

Visit %s: %s

%s`, siteName, siteName, siteName, siteURL, siteName)
	body := fmt.Sprintf(`<p style="margin:0 0 16px 0;">Merhaba,</p>
<p style="margin:0 0 16px 0;">Bu %s API tarafindan gonderilen test e-postasidir.</p>
<p style="margin:0 0 16px 0;">Hello,</p>
<p style="margin:0 0 16px 0;">This is a test email sent by the %s API.</p>`, html.EscapeString(siteName), html.EscapeString(siteName))
	htmlBody := htmlLayout("Test Email", "Test email", body, siteName, siteURL, siteName)
	return Content{Text: text, HTML: htmlBody}
}

func NewEntrySubject(language string) string {
	if language == "en" {
		return "New entry on yalinli.org"
	}
	return "yalinli.org uzerinde yeni girdi"
}

func NewEntryContent(language, title, description, entryURL, siteURL, siteName string) Content {
	shortDesc := truncateRunes(description, 240)
	if language == "en" {
		text := fmt.Sprintf(`A new entry was posted on %s:

%s

%s

Open: %s`, siteName, title, shortDesc, entryURL)
		body := fmt.Sprintf(`<p style="margin:0 0 12px 0;">A new entry was posted on %s.</p>
<div style="border:1px solid #e5e7eb;border-radius:12px;padding:14px 16px;margin:0 0 14px 0;background:#f8fafc;">
<div style="font-size:16px;font-weight:700;margin:0 0 6px 0;color:%s;">%s</div>
<div style="margin:0;color:%s;">%s</div>
</div>`, html.EscapeString(siteName), textColor, html.EscapeString(title), mutedColor, html.EscapeString(shortDesc))
		htmlBody := htmlLayout("New entry", "New entry on yalinli.org", body, "Open entry", entryURL, siteName)
		return Content{Text: text, HTML: htmlBody}
	}
	text := fmt.Sprintf(`%s uzerinde yeni bir girdi paylasildi:

%s

%s

Ac: %s`, siteName, title, shortDesc, entryURL)
	body := fmt.Sprintf(`<p style="margin:0 0 12px 0;">%s uzerinde yeni bir girdi paylasildi.</p>
<div style="border:1px solid #e5e7eb;border-radius:12px;padding:14px 16px;margin:0 0 14px 0;background:#f8fafc;">
<div style="font-size:16px;font-weight:700;margin:0 0 6px 0;color:%s;">%s</div>
<div style="margin:0;color:%s;">%s</div>
</div>`, html.EscapeString(siteName), textColor, html.EscapeString(title), mutedColor, html.EscapeString(shortDesc))
	htmlBody := htmlLayout("Yeni girdi", "Yeni girdi bildirimi", body, "Girdiyi ac", entryURL, siteName)
	return Content{Text: text, HTML: htmlBody}
}

func NewCommentSubject(language string) string {
	if language == "en" {
		return "New comment on your entry"
	}
	return "girdinize yeni yorum var"
}

func NewCommentContent(language, entryTitle, commentBody, entryURL, siteURL, siteName string) Content {
	shortComment := truncateRunes(commentBody, 240)
	if language == "en" {
		text := fmt.Sprintf(`A new comment was posted on your entry:

%s

%s

Open: %s`, entryTitle, shortComment, entryURL)
		body := fmt.Sprintf(`<p style="margin:0 0 12px 0;">A new comment was posted on your entry.</p>
<div style="border:1px solid #e5e7eb;border-radius:12px;padding:14px 16px;margin:0 0 14px 0;background:#f8fafc;">
<div style="font-size:16px;font-weight:700;margin:0 0 6px 0;color:%s;">%s</div>
<div style="margin:0;color:%s;">%s</div>
</div>`, textColor, html.EscapeString(entryTitle), mutedColor, html.EscapeString(shortComment))
		htmlBody := htmlLayout("New comment", "New comment on your entry", body, "View comment", entryURL, siteName)
		return Content{Text: text, HTML: htmlBody}
	}
	text := fmt.Sprintf(`Girdinize yeni bir yorum geldi:

%s

%s

Ac: %s`, entryTitle, shortComment, entryURL)
	body := fmt.Sprintf(`<p style="margin:0 0 12px 0;">Girdinize yeni bir yorum geldi.</p>
<div style="border:1px solid #e5e7eb;border-radius:12px;padding:14px 16px;margin:0 0 14px 0;background:#f8fafc;">
<div style="font-size:16px;font-weight:700;margin:0 0 6px 0;color:%s;">%s</div>
<div style="margin:0;color:%s;">%s</div>
</div>`, textColor, html.EscapeString(entryTitle), mutedColor, html.EscapeString(shortComment))
	htmlBody := htmlLayout("Yeni yorum", "Yeni yorum bildirimi", body, "Yorumu gor", entryURL, siteName)
	return Content{Text: text, HTML: htmlBody}
}

func loginCodeHTML(greeting, reason, ttlLine, ignoreLine, code, siteURL, siteName string) string {
	body := fmt.Sprintf(`<p style="margin:0 0 12px 0;">%s,</p>
<p style="margin:0 0 16px 0;">%s <strong>%s</strong>.</p>
<div style="border:1px solid #e5e7eb;border-radius:12px;padding:18px 16px;margin:0 0 16px 0;background:#f8fafc;text-align:center;">
<div style="font-size:28px;font-weight:700;letter-spacing:4px;color:%s;">%s</div>
</div>
<p style="margin:0 0 16px 0;">%s %s</p>`, html.EscapeString(greeting), html.EscapeString(reason), html.EscapeString(siteName), primaryColor, html.EscapeString(code), html.EscapeString(ttlLine), html.EscapeString(ignoreLine))
	return htmlLayout("Login code", "Your login code inside", body, "Siteye git", siteURL, siteName)
}

func htmlLayout(title, preheader, bodyHTML, ctaLabel, ctaURL, siteName string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
</head>
<body style="margin:0;padding:0;background:%s;">
<span style="display:none;visibility:hidden;opacity:0;color:transparent;height:0;width:0;overflow:hidden;">%s</span>
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:%s;width:100%%;">
  <tr>
    <td align="center" style="padding:24px 12px;">
      <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:600px;background:%s;border-radius:18px;overflow:hidden;box-shadow:0 6px 18px rgba(0,0,0,0.08);">
        <tr>
		  <td style="background:%s;padding:22px 24px;color:#ffffff;font-family:'Helvetica Neue',Arial,sans-serif;">
		    <div style="font-size:20px;letter-spacing:0.6px;font-weight:700;">%s</div>
		    <div style="font-size:13px;opacity:0.9;">Yalinli Mahallesi sorun, cozum ve sosyal paylasim platformu</div>
          </td>
        </tr>
        <tr>
          <td style="padding:26px 24px;font-family:'Helvetica Neue',Arial,sans-serif;color:%s;font-size:16px;line-height:1.6;">
            %s
          </td>
        </tr>
        <tr>
          <td style="padding:0 24px 24px 24px;">
            <a href="%s" target="_blank" style="background:%s;color:#ffffff;text-decoration:none;padding:12px 18px;border-radius:999px;font-family:'Helvetica Neue',Arial,sans-serif;font-weight:600;display:inline-block;">%s</a>
          </td>
        </tr>
		
      </table>
    </td>
  </tr>
</table>
</body>
</html>`, html.EscapeString(title), baseBGColor, html.EscapeString(preheader), baseBGColor, cardBGColor, primaryColor, html.EscapeString(siteName), textColor, bodyHTML, html.EscapeString(ctaURL), accentColor, html.EscapeString(ctaLabel))
}

func truncateRunes(v string, max int) string {
	v = strings.TrimSpace(v)
	if max <= 0 {
		return ""
	}
	r := []rune(v)
	if len(r) <= max {
		return v
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}
