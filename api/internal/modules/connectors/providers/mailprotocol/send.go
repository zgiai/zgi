package mailprotocol

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	messageMail "github.com/emersion/go-message/mail"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

func (c *Client) send(ctx context.Context, in providers.CredentialInput, args map[string]any) (map[string]any, error) {
	addresses := providers.TextList(args, "to")
	subject, text := providers.Text(args, "subject"), providers.Text(args, "text")
	if len(addresses) == 0 || len(addresses) > 20 || strings.ContainsAny(subject, "\r\n\x00") || len(subject) > 998 || len(text) > maxTextBytes {
		return nil, providers.Invalid()
	}
	to := make([]*mail.Address, 0, len(addresses))
	for _, raw := range addresses {
		address, err := mail.ParseAddress(raw)
		if err != nil || address.Name != "" || address.Address != raw || strings.ContainsAny(raw, "\r\n\x00") {
			return nil, providers.Invalid()
		}
		to = append(to, address)
	}
	from, err := mail.ParseAddress(in.Values["email"])
	if err != nil {
		return nil, providers.Invalid()
	}
	messageID := uuid.NewString() + "@zgi.invalid"
	var header messageMail.Header
	header.SetAddressList("From", []*mail.Address{from})
	header.SetAddressList("To", to)
	header.SetSubject(subject)
	header.SetDate(c.now().UTC())
	header.SetMessageID(messageID)
	header.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	var encoded bytes.Buffer
	writer, err := messageMail.CreateSingleInlineWriter(&encoded, header)
	if err != nil {
		return nil, providers.Invalid()
	}
	if _, err = io.WriteString(writer, text); err != nil {
		return nil, providers.Invalid()
	}
	if err = writer.Close(); err != nil {
		return nil, providers.Invalid()
	}
	if encoded.Len() > maxMessageBytes {
		return nil, &providers.Failure{Code: core.ErrorCodeMailSizeExceeded}
	}
	cl, conn, closeSession, err := c.openSMTP(ctx, in)
	if err != nil {
		return nil, err
	}
	defer closeSession()
	setDeadline := func(duration time.Duration) {
		deadline := time.Now().Add(duration)
		if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
			deadline = end
		}
		_ = conn.SetDeadline(deadline)
	}
	setDeadline(2 * time.Minute)
	if err = cl.Mail(from.Address); err != nil {
		return nil, protocolFailure(err, false)
	}
	for _, address := range to {
		if err = cl.Rcpt(address.Address); err != nil {
			_ = cl.Reset()
			var p *textproto.Error
			if errors.As(err, &p) {
				return nil, &providers.Failure{Code: core.ErrorCodeMailRecipientRejected, Retryable: p.Code/100 == 4}
			}
			return nil, protocolFailure(err, false)
		}
	}
	w, err := cl.Data()
	if err != nil {
		return nil, protocolFailure(err, false)
	}
	if _, err = w.Write(encoded.Bytes()); err != nil {
		return nil, protocolFailure(err, false)
	}
	// Close writes the DATA terminator and waits for the final server response.
	// Once attempted, a transport error cannot prove that Gmail rejected mail.
	setDeadline(10 * time.Minute)
	if err = w.Close(); err != nil {
		return nil, protocolFailure(err, true)
	}
	// A final 250 is server acceptance, regardless of a later QUIT failure.
	setDeadline(5 * time.Second)
	_ = cl.Quit()
	return map[string]any{"status": "accepted_by_server", "message_id": messageID}, nil
}
