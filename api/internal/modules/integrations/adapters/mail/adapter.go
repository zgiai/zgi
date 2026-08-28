package mail

import (
	"context"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type Adapter struct {
	smtp *smtpSender
	imap imapMailbox
}

func New() *Adapter                       { return &Adapter{smtp: newSMTPSender(), imap: imapMailbox{}} }
func (adapter *Adapter) DriverID() string { return DriverID }

func (adapter *Adapter) ValidateCredentials(_ context.Context, request integrations.CredentialValidationRequest) error {
	_, err := resolveSettings(&integrations.ResolvedConnection{
		IntegrationID: request.IntegrationID,
		DriverID:      request.DriverID,
		AuthMethodID:  request.AuthMethodID,
		Credentials:   request.Credentials,
		Config:        request.Config,
	})
	return err
}

func (adapter *Adapter) Execute(ctx context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	settings, err := resolveSettings(request.Connection)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(settings.IntegrationID, request.IntegrationID) {
		return nil, wrapError(integrations.ErrorCodeConnectionInvalid, "mail connection does not match the requested provider", nil)
	}
	switch request.ActionID {
	case ActionAccountGet:
		output := map[string]interface{}{"provider": settings.IntegrationID, "account": map[string]interface{}{"email": settings.Email, "display_name": settings.ProviderLabel, "read_supported": settings.ReadSupported, "send_supported": true}}
		return result(output, 1), nil
	case ActionFolderList:
		if !settings.ReadSupported {
			return nil, invalidInput("this mail connection does not support reading", nil)
		}
		folders, err := adapter.imap.listFolders(ctx, settings)
		if err != nil {
			return nil, err
		}
		return result(map[string]interface{}{"provider": settings.IntegrationID, "folders": folders}, len(folders)), nil
	case ActionMessageSearch:
		if !settings.ReadSupported {
			return nil, invalidInput("this mail connection does not support reading", nil)
		}
		messages, err := adapter.imap.search(ctx, settings, request.Input)
		if err != nil {
			return nil, err
		}
		items := make([]map[string]interface{}, 0, len(messages))
		for _, message := range messages {
			items = append(items, summaryOutput(message))
		}
		return result(map[string]interface{}{"provider": settings.IntegrationID, "messages": items}, len(items)), nil
	case ActionMessageGet:
		if !settings.ReadSupported {
			return nil, invalidInput("this mail connection does not support reading", nil)
		}
		message, err := adapter.imap.get(ctx, settings, inputString(request.Input, "message_ref"))
		if err != nil {
			return nil, err
		}
		return result(map[string]interface{}{"provider": settings.IntegrationID, "message": detailOutput(message)}, 1), nil
	case ActionMessageSend:
		message, err := messageFromInput(request.Input)
		if err != nil {
			return nil, err
		}
		sent, err := adapter.smtp.send(ctx, settings, message)
		if err != nil {
			return nil, err
		}
		return result(sentOutput(settings.IntegrationID, sent), len(sent.Accepted)), nil
	case ActionMessageReply:
		if !settings.ReadSupported {
			return nil, invalidInput("this mail connection does not support replying", nil)
		}
		source, err := adapter.imap.get(ctx, settings, inputString(request.Input, "message_ref"))
		if err != nil {
			return nil, err
		}
		reply, err := replyFromSource(settings.Email, source, request.Input)
		if err != nil {
			return nil, err
		}
		sent, err := adapter.smtp.send(ctx, settings, reply)
		if err != nil {
			return nil, err
		}
		return result(sentOutput(settings.IntegrationID, sent), len(sent.Accepted)), nil
	default:
		return nil, invalidInput("mail action is not supported", nil)
	}
}

func (adapter *Adapter) ValidateConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.ConnectionProfile, error) {
	settings, err := resolveSettings(connection)
	if err != nil {
		return nil, err
	}
	if settings.ReadSupported {
		if err := adapter.imap.authenticate(ctx, settings); err != nil {
			return nil, err
		}
	}
	if err := adapter.smtp.authenticate(ctx, settings); err != nil {
		return nil, err
	}
	scopes := []string{ScopeIdentity, ScopeSend}
	if settings.ReadSupported {
		scopes = append(scopes, ScopeRead)
	}
	return &integrations.ConnectionProfile{
		AccountID: settings.Email, DisplayName: settings.Email, GrantedScopes: scopes,
		ScopeEvidence: integrations.AuthScopeEvidenceConnectorDeclared,
	}, nil
}
func (adapter *Adapter) ProbeConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.HealthProbeReport, error) {
	profile, err := adapter.ValidateConnection(ctx, connection)
	if err != nil {
		status := integrations.HealthProbeStatusUnhealthy
		if code := integrations.ErrorCode(err); code == integrations.ErrorCodeTimeout || code == integrations.ErrorCodeUpstream {
			status = integrations.HealthProbeStatusDegraded
		}
		return &integrations.HealthProbeReport{Status: status, Checks: []integrations.HealthProbeCheck{{Code: integrations.ErrorCode(err), Status: status, Message: "Mail connection check failed"}}}, err
	}
	return &integrations.HealthProbeReport{Status: integrations.HealthProbeStatusHealthy, Profile: profile, Checks: []integrations.HealthProbeCheck{{Code: "mail_authenticated", Status: integrations.HealthProbeStatusHealthy}}}, nil
}

func messageFromInput(input map[string]interface{}) (outboundMessage, error) {
	to, err := parseAddressList(inputString(input, "to"), 20)
	if err != nil {
		return outboundMessage{}, invalidInput("email recipient list is invalid", err)
	}
	cc, err := parseAddressList(inputString(input, "cc"), 20)
	if err != nil {
		return outboundMessage{}, invalidInput("email CC list is invalid", err)
	}
	bcc, err := parseAddressList(inputString(input, "bcc"), 20)
	if err != nil {
		return outboundMessage{}, invalidInput("email BCC list is invalid", err)
	}
	return outboundMessage{To: to, CC: cc, BCC: bcc, Subject: strings.TrimSpace(inputString(input, "subject")), Body: inputString(input, "body_text")}, nil
}
func replyFromSource(own string, source messageDetail, input map[string]interface{}) (outboundMessage, error) {
	recipient := source.ReplyTo
	if recipient == "" {
		recipient = source.From
	}
	to, err := parseAddressList(recipient, 20)
	if err != nil {
		return outboundMessage{}, invalidInput("reply recipient is invalid", err)
	}
	cc := []string{}
	if inputBool(input, "reply_all") {
		candidates, _ := parseAddressList(source.To+","+source.CC, 40)
		for _, candidate := range candidates {
			if !strings.EqualFold(candidate, own) && !containsString(cc, candidate) && !containsString(to, candidate) {
				cc = append(cc, candidate)
			}
		}
	}
	subject := source.Subject
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), "re:") {
		subject = "Re: " + subject
	}
	references := strings.TrimSpace(source.References + " " + source.MessageID)
	return outboundMessage{To: to, CC: cc, Subject: subject, Body: inputString(input, "body_text"), InReplyTo: source.MessageID, References: references}, nil
}
func result(output map[string]interface{}, count int) *integrations.ActionResult {
	return &integrations.ActionResult{Output: output, ResultCount: count, AttemptCount: 1}
}
func summaryOutput(message messageSummary) map[string]interface{} {
	return map[string]interface{}{"message_ref": message.Ref, "subject": message.Subject, "from": message.From, "to": message.To, "date": message.Date, "unread": message.Unread, "size": int(message.Size)}
}
func detailOutput(message messageDetail) map[string]interface{} {
	return map[string]interface{}{"message_ref": message.Ref, "message_id": message.MessageID, "subject": message.Subject, "from": message.From, "to": message.To, "cc": message.CC, "date": message.Date, "body_text": message.BodyText, "attachments": message.Attachments}
}
func sentOutput(provider string, sent smtpResult) map[string]interface{} {
	return map[string]interface{}{"provider": provider, "message": map[string]interface{}{"message_id": sent.MessageID, "accepted_recipients": sent.Accepted, "smtp_accepted": true}}
}
