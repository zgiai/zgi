package mailprotocol

import (
	"errors"
	"net/smtp"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

func (c *Client) smtpAuth(cl *smtp.Client, login, secret string) (smtp.Auth, error) {
	ok, advertised := cl.Extension("AUTH")
	if ok {
		mechanisms := strings.Fields(strings.ToUpper(advertised))
		for _, mechanism := range mechanisms {
			if mechanism == "PLAIN" {
				return smtp.PlainAuth("", login, secret, c.profile.SMTPHost), nil
			}
		}
		for _, mechanism := range mechanisms {
			if mechanism == "LOGIN" {
				return &loginAuth{host: c.profile.SMTPHost, login: login, secret: secret}, nil
			}
		}
	}
	return nil, &providers.Failure{Code: core.ErrorCodeMailAuthMechanismUnsupported}
}

// LOGIN is used only when advertised after TLS. Neither challenges nor secrets
// are exposed as errors, and rejection never triggers another authentication.
type loginAuth struct {
	host, login, secret string
	step                int
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS || server.Name != a.host {
		return "", nil, errors.New("SMTP authentication requires the configured TLS peer")
	}
	a.step = 0
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(challenge []byte, more bool) ([]byte, error) {
	if !more {
		if a.step != 2 {
			return nil, errors.New("incomplete SMTP authentication exchange")
		}
		return nil, nil
	}
	if len(challenge) > 1024 {
		return nil, errors.New("invalid SMTP authentication challenge")
	}
	a.step++
	switch a.step {
	case 1:
		return []byte(a.login), nil
	case 2:
		return []byte(a.secret), nil
	default:
		return nil, errors.New("unexpected SMTP authentication challenge")
	}
}
