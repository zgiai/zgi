package integrations

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type SafetyChecker interface {
	Check(ctx context.Context, action ActionDefinition, input map[string]interface{}) error
}

type DNSResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type DefaultSafetyChecker struct {
	Resolver DNSResolver
}

var sensitiveInputPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-----BEGIN(?: [A-Z0-9]+)? PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]{16,}`),
	regexp.MustCompile(`(?i)\b(?:proxy-)?authorization\s*[:=]\s*[^\r\n]{4,}`),
	regexp.MustCompile(`(?i)\b(?:set-)?cookie\s*:\s*[^\r\n]{8,}`),
	regexp.MustCompile(`(?i)\b(?:x[-_])?(?:api[-_]?key|auth[-_]?token)\s*:\s*\S{8,}`),
	regexp.MustCompile(`(?i)\b(?:sk|exa)[-_][a-z0-9_-]{16,}`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`),
	regexp.MustCompile(`\bAIza[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{16,}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\b`),
	regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password|token|secret)\s*[:=]\s*[^\s]{8,}`),
	regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s/:@]+:[^\s/@]+@`),
	regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mysql|mariadb|mongodb(?:\+srv)?|redis|rediss|amqp|amqps|jdbc|mssql|oracle|ssh|file)://[^\s]+`),
	regexp.MustCompile(`(?i)(?:^|[^a-z0-9.-])(?:localhost|[a-z0-9][a-z0-9.-]*\.(?:local|internal))(?::\d{1,5})?(?:$|[^a-z0-9.-])`),
}

var embeddedHTTPURLPattern = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)
var embeddedIPLiteralPattern = regexp.MustCompile(`(?i)(?:\[[0-9a-f:.%]+\]|(?:\b[0-9a-f]{0,4}:){2,}[0-9a-f:.%]*\b|\b(?:\d{1,10}\.){1,3}\d{1,10}\b)`)

var sensitiveQueryKeys = map[string]struct{}{
	"access_key": {}, "access_token": {}, "api_key": {}, "apikey": {}, "auth": {},
	"authorization": {}, "key": {}, "password": {}, "secret": {}, "signature": {}, "token": {},
	"sig": {}, "googleaccessid": {}, "key_pair_id": {},
}

func IsSensitiveQueryKey(raw string) bool {
	key := strings.ToLower(strings.TrimSpace(raw))
	key = strings.NewReplacer("-", "_", ".", "_").Replace(key)
	if _, sensitive := sensitiveQueryKeys[key]; sensitive {
		return true
	}
	for _, suffix := range []string{"_access_key", "_api_key", "_credential", "_password", "_secret", "_signature", "_token"} {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	if strings.HasPrefix(key, "x_amz_") && (strings.Contains(key, "credential") || strings.Contains(key, "signature") || strings.Contains(key, "security_token")) {
		return true
	}
	if strings.HasPrefix(key, "x_goog_") && (strings.Contains(key, "credential") || strings.Contains(key, "signature")) {
		return true
	}
	return false
}

func (checker DefaultSafetyChecker) Check(ctx context.Context, action ActionDefinition, input map[string]interface{}) error {
	if !action.DataEgress {
		return nil
	}
	if containsSensitiveValue(input) {
		return NewError(ErrorCodeSensitiveInput, "sensitive data cannot be sent to an external integration", nil)
	}
	if action.ID == ActionWebSearch {
		if err := checker.validateEmbeddedPublicWebURLs(ctx, input); err != nil {
			return NewError(ErrorCodeSensitiveInput, "internal URLs cannot be sent to an external integration", err)
		}
		if err := checker.validateSearchDomains(ctx, input); err != nil {
			return NewError(ErrorCodeSensitiveInput, "non-public domains cannot be sent to an external integration", err)
		}
	}
	if action.ID == ActionWebFetch {
		values, ok := input["urls"]
		if !ok {
			return invalidInput("urls are required", nil)
		}
		for _, raw := range stringSlice(values) {
			if err := checker.validateResolvedPublicWebURL(ctx, raw); err != nil {
				return invalidInput("one or more URLs are not allowed", err)
			}
		}
	}
	return nil
}

func containsSensitiveValue(value interface{}) bool {
	switch typed := value.(type) {
	case string:
		for _, pattern := range sensitiveInputPatterns {
			if pattern.MatchString(typed) {
				return true
			}
		}
		if parsed, err := url.Parse(strings.TrimSpace(typed)); err == nil && parsed.IsAbs() {
			if parsed.User != nil {
				return true
			}
			for key, values := range parsed.Query() {
				if IsSensitiveQueryKey(key) && len(values) > 0 {
					return true
				}
			}
		}
		for _, candidate := range embeddedIPLiteralPattern.FindAllString(typed, -1) {
			if ip := parseWebIPLiteral(candidate); ip != nil && isNonPublicWebIP(ip) {
				return true
			}
		}
	case map[string]interface{}:
		for key, nested := range typed {
			if containsSensitiveValue(key) || containsSensitiveValue(nested) {
				return true
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if containsSensitiveValue(nested) {
				return true
			}
		}
	case []string:
		for _, nested := range typed {
			if containsSensitiveValue(nested) {
				return true
			}
		}
	}
	return false
}

func (checker DefaultSafetyChecker) validateSearchDomains(ctx context.Context, input map[string]interface{}) error {
	for _, key := range []string{"include_domains", "exclude_domains"} {
		for _, raw := range stringSlice(input[key]) {
			host := strings.TrimSpace(raw)
			if host == "" {
				return fmt.Errorf("%s contains an empty domain", key)
			}
			if strings.Contains(host, "://") || strings.ContainsAny(host, "/@?#") || strings.HasPrefix(host, "*.") {
				return fmt.Errorf("%s contains an invalid domain", key)
			}
			if err := checker.validateResolvedPublicWebURL(ctx, "https://"+host); err != nil {
				return fmt.Errorf("%s contains a non-public domain: %w", key, err)
			}
		}
	}
	return nil
}

func (checker DefaultSafetyChecker) validateEmbeddedPublicWebURLs(ctx context.Context, value interface{}) error {
	switch typed := value.(type) {
	case string:
		for _, candidate := range embeddedHTTPURLPattern.FindAllString(typed, -1) {
			candidate = strings.TrimRight(candidate, ".,;:!?)]}")
			if err := checker.validateResolvedPublicWebURL(ctx, candidate); err != nil {
				return err
			}
		}
	case map[string]interface{}:
		for _, nested := range typed {
			if err := checker.validateEmbeddedPublicWebURLs(ctx, nested); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, nested := range typed {
			if err := checker.validateEmbeddedPublicWebURLs(ctx, nested); err != nil {
				return err
			}
		}
	case []string:
		for _, nested := range typed {
			if err := checker.validateEmbeddedPublicWebURLs(ctx, nested); err != nil {
				return err
			}
		}
	}
	return nil
}

func (checker DefaultSafetyChecker) validateResolvedPublicWebURL(ctx context.Context, raw string) error {
	if err := ValidatePublicWebURL(raw); err != nil {
		return err
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("parse URL for DNS validation: %w", err)
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if parseWebIPLiteral(host) != nil {
		return nil
	}
	resolver := checker.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve URL hostname: %w", err)
	}
	if len(addresses) == 0 {
		return fmt.Errorf("URL hostname did not resolve to an address")
	}
	for _, address := range addresses {
		if address.IP == nil || isNonPublicWebIP(address.IP) {
			return fmt.Errorf("URL hostname resolved to a non-public IP address")
		}
	}
	return nil
}

func ValidatePublicWebURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme")
	}
	if parsed.User != nil {
		return fmt.Errorf("URL credentials are not allowed")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("URL fragments are not allowed")
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("local hostnames are not allowed")
	}
	for _, suffix := range []string{".nip.io", ".sslip.io", ".xip.io", ".localtest.me", ".lvh.me"} {
		if strings.HasSuffix(host, suffix) {
			return fmt.Errorf("address-mapping hostnames are not allowed")
		}
	}
	if host == "localtest.me" || host == "lvh.me" {
		return fmt.Errorf("local hostnames are not allowed")
	}
	if ip := parseWebIPLiteral(host); ip != nil && isNonPublicWebIP(ip) {
		return fmt.Errorf("private IP addresses are not allowed")
	}
	for key, values := range parsed.Query() {
		if IsSensitiveQueryKey(key) && len(values) > 0 {
			return fmt.Errorf("credential-bearing query parameters are not allowed")
		}
	}
	return nil
}

func validateExternalWebURL(raw string) error { return ValidatePublicWebURL(raw) }

func parseWebIPLiteral(host string) net.IP {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if zoneIndex := strings.LastIndex(host, "%"); zoneIndex >= 0 {
		host = host[:zoneIndex]
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip
	}
	parts := strings.Split(host, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return nil
	}
	values := make([]uint64, len(parts))
	for index, part := range parts {
		if part == "" {
			return nil
		}
		value, err := strconv.ParseUint(part, 0, 32)
		if err != nil {
			value, err = strconv.ParseUint(part, 10, 32)
		}
		if err != nil {
			return nil
		}
		values[index] = value
	}
	var numeric uint64
	switch len(values) {
	case 1:
		numeric = values[0]
	case 2:
		if values[0] > 0xff || values[1] > 0xffffff {
			return nil
		}
		numeric = values[0]<<24 | values[1]
	case 3:
		if values[0] > 0xff || values[1] > 0xff || values[2] > 0xffff {
			return nil
		}
		numeric = values[0]<<24 | values[1]<<16 | values[2]
	case 4:
		for _, value := range values {
			if value > 0xff {
				return nil
			}
		}
		numeric = values[0]<<24 | values[1]<<16 | values[2]<<8 | values[3]
	}
	if numeric > 0xffffffff {
		return nil
	}
	return net.IPv4(byte(numeric>>24), byte(numeric>>16), byte(numeric>>8), byte(numeric))
}

func isNonPublicWebIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		first, second, third := v4[0], v4[1], v4[2]
		return first == 0 ||
			(first == 100 && second >= 64 && second <= 127) ||
			(first == 192 && second == 0 && (third == 0 || third == 2)) ||
			(first == 198 && (second == 18 || second == 19)) ||
			(first == 198 && second == 51 && third == 100) ||
			(first == 203 && second == 0 && third == 113) ||
			first >= 224
	}
	if v6 := ip.To16(); v6 != nil && v6[0] == 0x20 && v6[1] == 0x01 && v6[2] == 0x0d && v6[3] == 0xb8 {
		return true
	}
	return !ip.IsGlobalUnicast()
}

func stringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}
