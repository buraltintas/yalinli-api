package email

import "fmt"

func LoginCodeSubject(language string) string {
	if language == "en" {
		return "Your yalinli.org login code"
	}
	return "yalinli.org giris kodunuz"
}

func LoginCodeBody(language, code string, ttlMinutes int) string {
	if language == "en" {
		return fmt.Sprintf(`Hello,

Your yalinli.org login code is:

%s

This code is valid for %d minutes. If you did not request this email, you can safely ignore it.

yalinli.org
Dosmealti / Antalya / Yalinli neighborhood platform`, code, ttlMinutes)
	}
	return fmt.Sprintf(`Merhaba,

yalinli.org giris kodunuz:

%s

Bu kod %d dakika boyunca gecerlidir. Bu e-postayi siz talep etmediyseniz guvenle yok sayabilirsiniz.

yalinli.org
Dosemealti / Antalya / Yalinli mahalle platformu`, code, ttlMinutes)
}

func TestEmailSubject() string {
	return "yalinli.org test email"
}

func TestEmailBody() string {
	return `Merhaba,

Bu yalinli.org API tarafindan gonderilen test e-postasidir.

Hello,

This is a test email sent by the yalinli.org API.

yalinli.org`
}
