package agentmemory

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	// memoryKeyPattern is the shape of a memory file name.
	memoryKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	emailPattern     = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	secretPattern    = regexp.MustCompile(`(?i)\b(?:sk-[a-z0-9_\-]{16,}|gh[pousr]_[a-z0-9]{20,}|AIza[a-z0-9_\-]{20,})\b`)
	phonePattern     = regexp.MustCompile(`(?:^|\D)(?:\+?\d[\s\-]?){11}(?:\D|$)`)
)

// ContainsSensitiveContent reports secrets, identifiers, special-category
// data, and contact details.
func ContainsSensitiveContent(content string) bool {
	return containsSensitiveContent(content, false)
}

// ContainsSecretContent reports what ContainsSensitiveContent does except
// contact details, for a memory block whose description permits them.
func ContainsSecretContent(content string) bool {
	return containsSensitiveContent(content, true)
}

var contactMarkers = map[string]struct{}{"email address": {}, "phone number": {}, "邮箱": {}, "手机号": {}, "电话号码": {}}

var contactDescriptionMarkers = []string{"contact", "phone", "email", "callback", "联系方式", "联系电话", "电话", "手机", "邮箱", "回电"}

// QuotedIn reports whether quote appears in one of messages, ignoring
// whitespace, punctuation, and case, so a quote that drops a list number, a
// comma, or a space still counts as the user's words.
func QuotedIn(quote string, messages ...string) bool {
	normalized := normalizeQuote(quote)
	if utf8.RuneCountInString(normalized) < 2 {
		return false
	}
	for _, message := range messages {
		if strings.Contains(normalizeQuote(message), normalized) {
			return true
		}
	}
	return false
}

func normalizeQuote(value string) string {
	var b strings.Builder
	for _, char := range strings.ToLower(value) {
		if unicode.IsLetter(char) || unicode.IsNumber(char) {
			b.WriteRune(char)
		}
	}
	return b.String()
}

// DescriptionAllowsContact reports whether a memory block's description says
// it holds contact details, such as a support Agent's callback number.
func DescriptionAllowsContact(description string) bool {
	normalized := strings.ToLower(description)
	for _, marker := range contactDescriptionMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func containsSensitiveContent(content string, allowContact bool) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return false
	}
	if secretPattern.MatchString(normalized) || (!allowContact && (emailPattern.MatchString(normalized) || phonePattern.MatchString(normalized))) {
		return true
	}
	digits := 0
	for _, char := range normalized {
		if char >= '0' && char <= '9' {
			digits++
			if digits >= 12 {
				return true
			}
		} else {
			digits = 0
		}
	}
	for _, marker := range []string{
		"password", "passwd", "passcode", "credential", "secret", "api key", "apikey", "access token", "refresh token", "private key", "credit card", "bank card", "card number", "ssn", "email address", "phone number", "home address", "medical record", "health condition", "sexual orientation", "religion", "political affiliation",
		"密码", "口令", "凭据", "令牌", "秘钥", "银行卡", "信用卡", "身份证", "证件号", "验证码", "支付", "邮箱", "手机号", "电话号码", "家庭住址", "病历", "健康状况", "性取向", "宗教信仰", "政治面貌",
	} {
		if _, contact := contactMarkers[marker]; contact && allowContact {
			continue
		}
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
