package i18n

var messages = map[string]map[string]string{
	"forbidden":           {"tr": "Erişim reddedildi", "en": "Forbidden"},
	"unauthorized":        {"tr": "Yetkisiz", "en": "Unauthorized"},
	"internal_error":      {"tr": "Sunucu hatası", "en": "Internal server error"},
	"invalid_input":       {"tr": "Geçersiz giriş", "en": "Invalid input"},
	"terms_not_accepted":  {"tr": "Kullanım koşulları kabul edilmelidir", "en": "Terms must be accepted"},
	"user_not_found":      {"tr": "Kullanıcı bulunamadı, kayıt olun", "en": "User not found, please sign up"},
	"user_already_exists": {"tr": "Kullanıcı zaten var, giriş yapın", "en": "User already exists, please log in"},
	"rate_limited":        {"tr": "Çok fazla istek", "en": "Too many requests"},
}

func Message(lang, code string) string {
	if byCode, ok := messages[code]; ok {
		if m, ok := byCode[lang]; ok {
			return m
		}
		if m, ok := byCode["tr"]; ok {
			return m
		}
	}
	return code
}
