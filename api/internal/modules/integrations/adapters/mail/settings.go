package mail

import (
	"fmt"
	"net"
	"net/mail"
	"regexp"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type serverSettings struct {
	IntegrationID string
	ConnectionID  string
	Email         string
	Username      string
	Password      string
	IMAPHost      string
	IMAPPort      int
	IMAPSecurity  string
	SMTPHost      string
	SMTPPort      int
	SMTPSecurity  string
	ReadSupported bool
	ProviderLabel string
	RequireIMAPID bool
}

func resolveSettings(connection *integrations.ResolvedConnection) (serverSettings, error) {
	if connection == nil || !strings.EqualFold(connection.DriverID, DriverID) {
		return serverSettings{}, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "mail connection is invalid", nil)
	}
	emailAddress := strings.TrimSpace(connection.Credentials["email_address"])
	parsed, err := mail.ParseAddress(emailAddress)
	if err != nil || !strings.EqualFold(parsed.Address, emailAddress) || len(emailAddress) > 320 {
		return serverSettings{}, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "mailbox address is invalid", err)
	}
	username := strings.TrimSpace(connection.Credentials["username"])
	if username == "" {
		username = emailAddress
	}
	password := strings.TrimSpace(connection.Credentials["app_password"])
	if len(username) > 320 || strings.ContainsAny(username, "\r\n\x00") || password == "" || len(password) > 1024 || strings.ContainsRune(password, '\x00') {
		return serverSettings{}, integrations.NewError(integrations.ErrorCodeAuthInvalid, "mail credentials are unavailable", nil)
	}
	settings := serverSettings{IntegrationID: strings.ToLower(connection.IntegrationID), ConnectionID: strings.TrimSpace(connection.ID), Email: emailAddress, Username: username, Password: password}
	switch settings.IntegrationID {
	case IntegrationStandardMail:
		preset, presetSelected, presetErr := presetForAuthMethod(connection.AuthMethodID, emailAddress)
		if presetErr != nil {
			return serverSettings{}, presetErr
		}
		if presetSelected {
			settings.Username = emailAddress
			settings.IMAPHost = preset.IMAPHost
			settings.IMAPPort = preset.IMAPPort
			settings.IMAPSecurity = "implicit_tls"
			settings.SMTPHost = preset.SMTPHost
			settings.SMTPPort = preset.SMTPPort
			settings.SMTPSecurity = preset.SMTPSecurity
			settings.ProviderLabel = preset.DisplayName
			settings.RequireIMAPID = preset.RequireIMAPID
		} else {
			settings.IMAPHost = strings.ToLower(strings.TrimSpace(connection.Credentials["imap_host"]))
			settings.IMAPPort, _ = strconv.Atoi(strings.TrimSpace(connection.Credentials["imap_port"]))
			settings.IMAPSecurity = "implicit_tls"
			settings.SMTPHost = strings.ToLower(strings.TrimSpace(connection.Credentials["smtp_host"]))
			settings.SMTPPort, _ = strconv.Atoi(strings.TrimSpace(connection.Credentials["smtp_port"]))
			settings.SMTPSecurity = strings.ToLower(strings.TrimSpace(connection.Credentials["smtp_security"]))
			settings.ProviderLabel = "Standard Mail"
		}
		if err := validateCustomIMAP(settings.IMAPHost, settings.IMAPPort); err != nil {
			return serverSettings{}, err
		}
		if err := validateCustomSMTP(settings.SMTPHost, settings.SMTPPort, settings.SMTPSecurity); err != nil {
			return serverSettings{}, err
		}
		settings.ReadSupported = true
	default:
		return serverSettings{}, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "mail provider is invalid", nil)
	}
	return settings, nil
}

func validateCustomIMAP(host string, port int) error {
	if err := validatePublicMailHostName(host, "IMAP"); err != nil {
		return err
	}
	if port != 993 {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "IMAP port must be 993 with implicit TLS", nil)
	}
	return nil
}

func validateCustomSMTP(host string, port int, security string) error {
	if err := validatePublicMailHostName(host, "SMTP"); err != nil {
		return err
	}
	if port != 465 && port != 587 {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "SMTP port must be 465 or 587", nil)
	}
	if security != "implicit_tls" && security != "starttls" {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "SMTP security must use TLS", nil)
	}
	return nil
}

func validatePublicMailHostName(host, protocol string) error {
	if host == "" || len(host) > 253 || net.ParseIP(host) != nil || !dnsNamePattern.MatchString(host) {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, protocol+" host must be a public DNS name", nil)
	}
	return nil
}

func serverAddress(host string, port int) string { return net.JoinHostPort(host, strconv.Itoa(port)) }
func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
func inputString(input map[string]interface{}, key string) string {
	value, _ := input[key].(string)
	return value
}
func inputBool(input map[string]interface{}, key string) bool {
	value, _ := input[key].(bool)
	return value
}
func inputInt(input map[string]interface{}, key string, fallback int) int {
	switch value := input[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	case jsonNumber:
		parsed, _ := strconv.Atoi(string(value))
		return parsed
	}
	return fallback
}

type jsonNumber string

var dnsNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

func invalidInput(message string, err error) error {
	return integrations.NewError(integrations.ErrorCodeInvalidInput, message, err)
}
func wrapError(code, message string, err error) error {
	return integrations.NewError(code, message, err)
}
func authError(err error) error {
	return wrapError(integrations.ErrorCodeAuthInvalid, "mail authentication failed", err)
}
func upstreamError(err error) error {
	return wrapError(integrations.ErrorCodeUpstream, "mail service is unavailable", err)
}
func validateServerName(host string) error {
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("host is empty")
	}
	return nil
}
