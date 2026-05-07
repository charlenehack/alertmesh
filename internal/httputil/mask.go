package httputil

import (
	"regexp"
	"strings"
)

var rePhone = regexp.MustCompile(`^(\+?\d{2,4})?(\d{3})\d+(\d{4})$`)

// MaskEmail masks an email address for safe display in API responses and logs.
//
//	alice@example.com  →  al***@example.com
//	a@b.com            →  a***@b.com
func MaskEmail(email string) string {
	if email == "" {
		return ""
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	local := email[:at]
	domain := email[at:]

	show := 2
	if len(local) <= 2 {
		show = 1
	}
	stars := strings.Repeat("*", max(len(local)-show, 3))
	return local[:show] + stars + domain
}

// MaskPhone masks a phone number for safe display.
//
//	13812345678   →  138****5678
//	+8613812345678 →  +86138****5678
func MaskPhone(phone string) string {
	if phone == "" {
		return ""
	}
	// Keep prefix (country code + first 3 digits) and last 4 digits.
	return rePhone.ReplaceAllString(phone, "${1}${2}****${3}")
}

// MaskIDCard masks a Chinese ID card number.
//
//	110101199001011234  →  110101********1234
func MaskIDCard(id string) string {
	if len(id) < 8 {
		return "***"
	}
	return id[:6] + strings.Repeat("*", len(id)-10) + id[len(id)-4:]
}
