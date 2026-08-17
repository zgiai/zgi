package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
	messageMail "github.com/emersion/go-message/mail"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type imapMailbox struct{}
type messageReference struct {
	Version     int    `json:"v"`
	Connection  string `json:"c"`
	Folder      string `json:"f"`
	UIDValidity uint32 `json:"uv"`
	UID         uint32 `json:"u"`
}

type messageSummary struct {
	Ref, Subject, From, To, Date string
	Unread                       bool
	Size                         uint32
}
type messageDetail struct {
	messageSummary
	MessageID, CC, BodyText, ReplyTo, References string
	Attachments                                  []map[string]interface{}
}

func (imapMailbox) connect(ctx context.Context, settings serverSettings) (*imapclient.Client, error) {
	addresses, err := resolvePublicMailHost(ctx, settings.IMAPHost)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	raw, err := dialer.DialContext(ctx, "tcp", serverAddress(addresses[0].String(), settings.IMAPPort))
	if err != nil {
		return nil, upstreamError(err)
	}
	secure := tls.Client(raw, &tls.Config{ServerName: settings.IMAPHost, MinVersion: tls.VersionTLS12})
	if err := secure.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, upstreamError(err)
	}
	client, err := imapclient.New(secure)
	if err != nil {
		secure.Close()
		return nil, upstreamError(err)
	}
	client.Timeout = 20 * time.Second
	if err := client.Login(settings.Username, settings.Password); err != nil {
		client.Logout()
		return nil, authError(err)
	}
	if settings.RequireIMAPID {
		status, idErr := client.Execute(imapClientIDCommand(), nil)
		if idErr != nil {
			client.Logout()
			return nil, upstreamError(idErr)
		}
		if statusErr := status.Err(); statusErr != nil {
			client.Logout()
			return nil, wrapError(integrations.ErrorCodeAccessDenied, "mail provider rejected the required client identity", statusErr)
		}
	}
	return client, nil
}

func imapClientIDCommand() *imap.Command {
	return &imap.Command{
		Name: "ID",
		Arguments: []interface{}{[]interface{}{
			"name", "ZGI",
			"version", "1",
			"vendor", "ZGI",
		}},
	}
}

func (imapMailbox) authenticate(ctx context.Context, settings serverSettings) error {
	client, err := (imapMailbox{}).connect(ctx, settings)
	if err != nil {
		return err
	}
	return client.Logout()
}

func (imapMailbox) listFolders(ctx context.Context, settings serverSettings) ([]map[string]interface{}, error) {
	client, err := (imapMailbox{}).connect(ctx, settings)
	if err != nil {
		return nil, err
	}
	defer client.Logout()
	channel := make(chan *imap.MailboxInfo, 16)
	done := make(chan error, 1)
	go func() { done <- client.List("", "*", channel) }()
	result := make([]map[string]interface{}, 0, 32)
	for mailbox := range channel {
		if mailbox == nil || len(result) >= 100 {
			continue
		}
		attrs := append([]string(nil), mailbox.Attributes...)
		result = append(result, map[string]interface{}{"name": bounded(mailbox.Name, 512), "attributes": attrs})
	}
	if err := <-done; err != nil {
		return nil, upstreamError(err)
	}
	return result, nil
}

func (imapMailbox) search(ctx context.Context, settings serverSettings, input map[string]interface{}) ([]messageSummary, error) {
	client, err := (imapMailbox{}).connect(ctx, settings)
	if err != nil {
		return nil, err
	}
	defer client.Logout()
	folder := strings.TrimSpace(inputString(input, "folder"))
	if folder == "" {
		folder = "INBOX"
	}
	status, err := client.Select(folder, true)
	if err != nil {
		return nil, wrapError(integrations.ErrorCodeProviderRejected, "mail folder could not be opened", err)
	}
	criteria := imap.NewSearchCriteria()
	if value := strings.TrimSpace(inputString(input, "query")); value != "" {
		criteria.Text = []string{value}
	}
	if value := strings.TrimSpace(inputString(input, "from")); value != "" {
		criteria.Header.Set("From", value)
	}
	if value := strings.TrimSpace(inputString(input, "subject")); value != "" {
		criteria.Header.Set("Subject", value)
	}
	if inputBool(input, "unread_only") {
		criteria.WithoutFlags = []string{imap.SeenFlag}
	}
	uids, err := client.UidSearch(criteria)
	if err != nil {
		return nil, upstreamError(err)
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
	limit := inputInt(input, "max_results", 10)
	if limit < 1 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	if len(uids) > limit {
		uids = uids[:limit]
	}
	if len(uids) == 0 {
		return []messageSummary{}, nil
	}
	seqset := new(imap.SeqSet)
	seqset.AddNum(uids...)
	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() {
		done <- client.UidFetch(seqset, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchFlags, imap.FetchRFC822Size}, messages)
	}()
	result := make([]messageSummary, 0, len(uids))
	for msg := range messages {
		if msg == nil || msg.Envelope == nil {
			continue
		}
		result = append(result, summaryFromIMAP(settings.ConnectionID, folder, status.UidValidity, msg))
	}
	if err := <-done; err != nil {
		return nil, upstreamError(err)
	}
	return result, nil
}

func (imapMailbox) get(ctx context.Context, settings serverSettings, encodedRef string) (messageDetail, error) {
	ref, err := decodeMessageRef(encodedRef, settings.ConnectionID)
	if err != nil {
		return messageDetail{}, invalidInput("message reference is invalid", err)
	}
	client, err := (imapMailbox{}).connect(ctx, settings)
	if err != nil {
		return messageDetail{}, err
	}
	defer client.Logout()
	status, err := client.Select(ref.Folder, true)
	if err != nil {
		return messageDetail{}, wrapError(integrations.ErrorCodeProviderRejected, "mail folder could not be opened", err)
	}
	if status.UidValidity != ref.UIDValidity {
		return messageDetail{}, invalidInput("message reference has expired", nil)
	}
	section := &imap.BodySectionName{Peek: true}
	seqset := new(imap.SeqSet)
	seqset.AddNum(ref.UID)
	channel := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- client.UidFetch(seqset, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchFlags, imap.FetchRFC822Size, section.FetchItem()}, channel)
	}()
	var fetched *imap.Message
	for msg := range channel {
		fetched = msg
	}
	if err := <-done; err != nil {
		return messageDetail{}, upstreamError(err)
	}
	if fetched == nil {
		return messageDetail{}, wrapError(integrations.ErrorCodeProviderRejected, "mail message was not found", nil)
	}
	body := fetched.GetBody(section)
	if body == nil {
		return messageDetail{}, wrapError(integrations.ErrorCodeResponseInvalid, "mail message body is unavailable", nil)
	}
	detail := messageDetail{messageSummary: summaryFromIMAP(settings.ConnectionID, ref.Folder, status.UidValidity, fetched)}
	reader, parseErr := messageMail.CreateReader(body)
	if parseErr != nil && reader == nil {
		return messageDetail{}, wrapError(integrations.ErrorCodeResponseInvalid, "mail message could not be parsed", parseErr)
	}
	defer reader.Close()
	detail.Subject = bounded(reader.Header.Get("Subject"), 998)
	detail.From = headerAddresses(reader, "From")
	detail.To = headerAddresses(reader, "To")
	detail.CC = headerAddresses(reader, "Cc")
	detail.ReplyTo = headerAddresses(reader, "Reply-To")
	detail.MessageID = bounded(reader.Header.Get("Message-ID"), 998)
	detail.References = bounded(reader.Header.Get("References"), 4000)
	var bodyText strings.Builder
	attachments := make([]map[string]interface{}, 0)
	for {
		part, partErr := reader.NextPart()
		if partErr == io.EOF {
			break
		}
		if partErr != nil && part == nil {
			continue
		}
		switch header := part.Header.(type) {
		case *messageMail.InlineHeader:
			contentType := strings.ToLower(header.Get("Content-Type"))
			if contentType == "" || strings.HasPrefix(contentType, "text/plain") {
				remaining := 100000 - bodyText.Len()
				if remaining > 0 {
					limited, _ := io.ReadAll(io.LimitReader(part.Body, int64(remaining)))
					bodyText.Write(limited)
				}
			}
		case *messageMail.AttachmentHeader:
			if len(attachments) < 50 {
				filename, _ := header.Filename()
				contentType := header.Get("Content-Type")
				size := int64(0)
				if value := header.Get("Content-Length"); value != "" {
					fmt.Sscan(value, &size)
				}
				attachments = append(attachments, map[string]interface{}{"filename": bounded(filename, 255), "content_type": bounded(contentType, 255), "size": size})
			}
		}
	}
	detail.BodyText = bodyText.String()
	detail.Attachments = attachments
	return detail, nil
}

func summaryFromIMAP(connectionID, folder string, uidValidity uint32, message *imap.Message) messageSummary {
	envelope := message.Envelope
	date := ""
	if !envelope.Date.IsZero() {
		date = envelope.Date.UTC().Format(time.RFC3339)
	}
	return messageSummary{Ref: encodeMessageRef(messageReference{Version: 1, Connection: connectionID, Folder: folder, UIDValidity: uidValidity, UID: message.Uid}), Subject: bounded(envelope.Subject, 998), From: imapAddresses(envelope.From), To: imapAddresses(envelope.To), Date: date, Unread: !containsString(message.Flags, imap.SeenFlag), Size: message.Size}
}
func imapAddresses(values []*imap.Address) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		address := value.MailboxName + "@" + value.HostName
		if parsed, err := mail.ParseAddress(address); err == nil {
			if value.PersonalName != "" {
				parsed.Name = value.PersonalName
			}
			result = append(result, parsed.String())
		}
	}
	return bounded(strings.Join(result, ", "), 4000)
}
func headerAddresses(reader *messageMail.Reader, key string) string {
	values, err := reader.Header.AddressList(key)
	if err != nil {
		return ""
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != nil {
			result = append(result, value.String())
		}
	}
	return bounded(strings.Join(result, ", "), 4000)
}
func encodeMessageRef(ref messageReference) string {
	raw, _ := json.Marshal(ref)
	return base64.RawURLEncoding.EncodeToString(raw)
}
func decodeMessageRef(value, connectionID string) (messageReference, error) {
	var ref messageReference
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return ref, err
	}
	if err = json.Unmarshal(raw, &ref); err != nil {
		return ref, err
	}
	if ref.Version != 1 || ref.Connection != strings.TrimSpace(connectionID) || ref.Folder == "" || len(ref.Folder) > 512 || ref.UIDValidity == 0 || ref.UID == 0 {
		return ref, fmt.Errorf("invalid reference")
	}
	return ref, nil
}
func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}
