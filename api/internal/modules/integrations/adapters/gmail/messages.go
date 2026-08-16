package gmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

const (
	defaultSearchResults    = 10
	maxSearchResults        = 20
	defaultBodyCharacters   = 20_000
	maxBodyCharacters       = 50_000
	maxMIMETraversalDepth   = 12
	maxMIMEParts            = 100
	maxDecodedMessageBytes  = maxResponseBytes
	maxReferencesCharacters = 8_000
)

var gmailMetadataHeaders = []string{"Subject", "From", "To", "Cc", "Date", "Message-ID", "References", "Reply-To", "Content-Type"}

type gmailListResponse struct {
	Messages           []gmailMessageReference `json:"messages"`
	NextPageToken      string                  `json:"nextPageToken"`
	ResultSizeEstimate int                     `json:"resultSizeEstimate"`
}

type gmailMessageReference struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

type gmailMessage struct {
	ID           string           `json:"id"`
	ThreadID     string           `json:"threadId"`
	LabelIDs     []string         `json:"labelIds"`
	Snippet      string           `json:"snippet"`
	Payload      gmailMessagePart `json:"payload"`
	SizeEstimate int              `json:"sizeEstimate"`
}

type gmailMessagePart struct {
	PartID   string             `json:"partId"`
	MimeType string             `json:"mimeType"`
	Filename string             `json:"filename"`
	Headers  []gmailHeader      `json:"headers"`
	Body     gmailMessageBody   `json:"body"`
	Parts    []gmailMessagePart `json:"parts"`
}

type gmailHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type gmailMessageBody struct {
	Data string `json:"data"`
	Size int    `json:"size"`
}

type gmailDraftResponse struct {
	ID      string            `json:"id"`
	Message gmailSendResponse `json:"message"`
}

func (adapter *Adapter) searchMail(
	ctx context.Context,
	accessToken string,
	input map[string]interface{},
) (map[string]interface{}, responseMeta, int, error) {
	query := strings.TrimSpace(inputString(input, "query"))
	if query == "" || len([]rune(query)) > 2048 {
		return nil, responseMeta{}, 0, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail search query is invalid", nil)
	}
	pageToken, err := optionalNonBlankString(input, "page_token", 2048)
	if err != nil {
		return nil, responseMeta{}, 0, err
	}
	maxResults, err := boundedInputInt(input, "max_results", defaultSearchResults, 1, maxSearchResults)
	if err != nil {
		return nil, responseMeta{}, 0, err
	}
	includeSpamTrash, err := optionalInputBool(input, "include_spam_trash")
	if err != nil {
		return nil, responseMeta{}, 0, err
	}

	var listed gmailListResponse
	meta, err := adapter.client.listMessages(ctx, accessToken, query, pageToken, maxResults, includeSpamTrash, &listed)
	if err != nil {
		return nil, meta, 0, err
	}
	if len(listed.Messages) > maxResults {
		listed.Messages = listed.Messages[:maxResults]
	}
	messages := make([]interface{}, 0, len(listed.Messages))
	for _, reference := range listed.Messages {
		if !validGmailID(reference.ID) {
			return nil, meta, 0, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail search response contained an invalid message ID", nil)
		}
		var message gmailMessage
		messageMeta, getErr := adapter.client.getMessage(
			ctx, accessToken, reference.ID, "metadata", gmailMetadataHeaders, true, &message,
		)
		meta = mergeResponseMeta(meta, messageMeta)
		if getErr != nil {
			return nil, meta, 0, getErr
		}
		if strings.TrimSpace(message.ID) == "" {
			message.ID = reference.ID
		}
		if strings.TrimSpace(message.ThreadID) == "" {
			message.ThreadID = reference.ThreadID
		}
		messages = append(messages, gmailMessageSummary(message))
	}
	resultEstimate := listed.ResultSizeEstimate
	if resultEstimate < 0 {
		resultEstimate = 0
	}
	return map[string]interface{}{
		"provider":             IntegrationID,
		"request_id":           bounded(meta.RequestID, 128),
		"messages":             messages,
		"next_page_token":      bounded(listed.NextPageToken, 2048),
		"result_size_estimate": resultEstimate,
	}, meta, len(messages), nil
}

func (adapter *Adapter) getMail(
	ctx context.Context,
	accessToken string,
	input map[string]interface{},
) (map[string]interface{}, responseMeta, error) {
	messageID := strings.TrimSpace(inputString(input, "message_id"))
	if !validGmailID(messageID) {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail message ID is invalid", nil)
	}
	maxCharacters, err := boundedInputInt(input, "max_body_characters", defaultBodyCharacters, 1000, maxBodyCharacters)
	if err != nil {
		return nil, responseMeta{}, err
	}
	var message gmailMessage
	meta, err := adapter.client.getMessage(ctx, accessToken, messageID, "full", nil, true, &message)
	if err != nil {
		return nil, meta, err
	}
	if !validGmailID(message.ID) || !validGmailID(message.ThreadID) {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail message response is incomplete", nil)
	}
	body, mimeType, truncated, err := gmailMessageText(message.Payload, maxCharacters)
	if err != nil {
		return nil, meta, err
	}
	return map[string]interface{}{
		"provider":   IntegrationID,
		"request_id": bounded(meta.RequestID, 128),
		"message":    gmailMessageDetail(message, body, mimeType, truncated),
	}, meta, nil
}

func (adapter *Adapter) replyMail(
	ctx context.Context,
	accessToken string,
	input map[string]interface{},
) (map[string]interface{}, responseMeta, error) {
	messageID := strings.TrimSpace(inputString(input, "message_id"))
	if !validGmailID(messageID) {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail message ID is invalid", nil)
	}
	bodyText := inputString(input, "body_text")
	if strings.TrimSpace(bodyText) == "" || len([]rune(bodyText)) > 100_000 {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail reply body is invalid", nil)
	}

	var original gmailMessage
	meta, err := adapter.client.getMessage(ctx, accessToken, messageID, "metadata", gmailMetadataHeaders, false, &original)
	if err != nil {
		return nil, meta, err
	}
	if !validGmailID(original.ThreadID) {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail source message has no valid thread", nil)
	}
	rawMessage, err := buildReplyRFC2822Message(original, bodyText)
	if err != nil {
		return nil, meta, err
	}
	var sent gmailSendResponse
	sendMeta, err := adapter.client.sendMessageInThread(
		ctx, accessToken, base64.RawURLEncoding.EncodeToString(rawMessage), original.ThreadID, &sent,
	)
	meta = mergeResponseMeta(meta, sendMeta)
	if err != nil {
		return nil, meta, err
	}
	if !validGmailID(sent.ID) || !validGmailID(sent.ThreadID) {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail reply response is incomplete", nil)
	}
	return map[string]interface{}{
		"provider":   IntegrationID,
		"request_id": bounded(meta.RequestID, 128),
		"message":    gmailSentMessage(sent),
	}, meta, nil
}

func (adapter *Adapter) createDraft(
	ctx context.Context,
	accessToken string,
	input map[string]interface{},
) (map[string]interface{}, responseMeta, error) {
	message, err := buildRFC2822Message(input)
	if err != nil {
		return nil, responseMeta{}, err
	}
	var draft gmailDraftResponse
	meta, err := adapter.client.createDraft(ctx, accessToken, base64.RawURLEncoding.EncodeToString(message), &draft)
	if err != nil {
		return nil, meta, err
	}
	if !validGmailID(draft.ID) || !validGmailID(draft.Message.ID) {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail draft response is incomplete", nil)
	}
	return map[string]interface{}{
		"provider":   IntegrationID,
		"request_id": bounded(meta.RequestID, 128),
		"draft": map[string]interface{}{
			"id": draft.ID, "message": gmailSentMessage(draft.Message),
		},
	}, meta, nil
}

func gmailMessageSummary(message gmailMessage) map[string]interface{} {
	return map[string]interface{}{
		"id":        bounded(message.ID, 255),
		"thread_id": bounded(message.ThreadID, 255),
		"subject":   decodedGmailHeader(message.Payload.Headers, "Subject", 998),
		"from":      decodedGmailHeader(message.Payload.Headers, "From", 4000),
		"to":        decodedGmailHeader(message.Payload.Headers, "To", 4000),
		"date":      decodedGmailHeader(message.Payload.Headers, "Date", 255),
		"snippet":   bounded(message.Snippet, 1000),
		"label_ids": boundedLabels(message.LabelIDs, 50),
	}
}

func gmailMessageDetail(message gmailMessage, body, mimeType string, truncated bool) map[string]interface{} {
	detail := gmailMessageSummary(message)
	detail["cc"] = decodedGmailHeader(message.Payload.Headers, "Cc", 4000)
	detail["mime_type"] = bounded(mimeType, 255)
	detail["body_text"] = body
	detail["body_truncated"] = truncated
	return detail
}

func gmailSentMessage(message gmailSendResponse) map[string]interface{} {
	return map[string]interface{}{
		"id": bounded(message.ID, 255), "thread_id": bounded(message.ThreadID, 255),
		"label_ids": boundedLabels(message.LabelIDs, 20),
	}
}

func boundedLabels(values []string, limit int) []interface{} {
	result := make([]interface{}, 0, min(len(values), limit))
	for _, value := range values {
		if value = bounded(value, 100); value != "" && len(result) < limit {
			result = append(result, value)
		}
	}
	return result
}

func decodedGmailHeader(headers []gmailHeader, name string, limit int) string {
	raw := gmailHeaderValue(headers, name)
	if raw == "" {
		return ""
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(raw)
	if err != nil {
		decoded = raw
	}
	return bounded(decoded, limit)
}

func gmailHeaderValue(headers []gmailHeader, name string) string {
	for _, header := range headers {
		if strings.EqualFold(strings.TrimSpace(header.Name), name) {
			return strings.TrimSpace(header.Value)
		}
	}
	return ""
}

func gmailMessageText(payload gmailMessagePart, maxCharacters int) (string, string, bool, error) {
	state := &mimeTraversalState{}
	if err := collectGmailTextParts(payload, 0, state); err != nil {
		return "", "", false, err
	}
	text := strings.TrimSpace(strings.Join(state.plain, "\n\n"))
	mimeType := "text/plain"
	if text == "" && len(state.html) > 0 {
		text = strings.TrimSpace(htmlToPlainText(strings.Join(state.html, "\n")))
		mimeType = "text/html"
	}
	if text == "" {
		mimeType = bounded(payload.MimeType, 255)
	}
	runes := []rune(text)
	truncated := len(runes) > maxCharacters
	if truncated {
		runes = runes[:maxCharacters]
	}
	return string(runes), mimeType, truncated, nil
}

type mimeTraversalState struct {
	parts int
	plain []string
	html  []string
}

func collectGmailTextParts(part gmailMessagePart, depth int, state *mimeTraversalState) error {
	if depth > maxMIMETraversalDepth {
		return integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail MIME structure is too deeply nested", nil)
	}
	state.parts++
	if state.parts > maxMIMEParts {
		return integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail MIME structure contains too many parts", nil)
	}
	mediaType := strings.ToLower(strings.TrimSpace(part.MimeType))
	disposition := strings.ToLower(strings.TrimSpace(gmailHeaderValue(part.Headers, "Content-Disposition")))
	isAttachment := part.Filename != "" || strings.HasPrefix(disposition, "attachment")
	if !isAttachment && (mediaType == "text/plain" || mediaType == "text/html") && part.Body.Data != "" {
		decoded, err := decodeGmailBody(part.Body.Data)
		if err != nil {
			return integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail message body could not be decoded", err)
		}
		decoded, err = decodeGmailCharset(decoded, gmailHeaderValue(part.Headers, "Content-Type"))
		if err != nil {
			return integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail message character encoding is invalid", err)
		}
		if mediaType == "text/plain" {
			state.plain = append(state.plain, string(decoded))
		} else {
			state.html = append(state.html, string(decoded))
		}
	}
	for _, child := range part.Parts {
		if err := collectGmailTextParts(child, depth+1, state); err != nil {
			return err
		}
	}
	return nil
}

func decodeGmailBody(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(value)
}

func decodeGmailCharset(value []byte, contentType string) ([]byte, error) {
	if strings.TrimSpace(contentType) == "" {
		return value, nil
	}
	reader, err := charset.NewReader(bytes.NewReader(value), contentType)
	if err != nil {
		return nil, err
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, maxDecodedMessageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(decoded) > maxDecodedMessageBytes {
		return nil, fmt.Errorf("decoded message body exceeded the platform limit")
	}
	return decoded, nil
}

func htmlToPlainText(value string) string {
	tokenizer := html.NewTokenizer(strings.NewReader(value))
	var text strings.Builder
	skipDepth := 0
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(text.String()), " ")
		case html.StartTagToken:
			name, _ := tokenizer.TagName()
			if bytes.EqualFold(name, []byte("script")) || bytes.EqualFold(name, []byte("style")) {
				skipDepth++
			}
		case html.EndTagToken:
			name, _ := tokenizer.TagName()
			if (bytes.EqualFold(name, []byte("script")) || bytes.EqualFold(name, []byte("style"))) && skipDepth > 0 {
				skipDepth--
			}
		case html.TextToken:
			if skipDepth == 0 {
				text.Write(tokenizer.Text())
				text.WriteByte(' ')
			}
		}
	}
}

func buildReplyRFC2822Message(original gmailMessage, bodyText string) ([]byte, error) {
	recipientValue := firstNonEmpty(
		gmailHeaderValue(original.Payload.Headers, "Reply-To"),
		gmailHeaderValue(original.Payload.Headers, "From"),
	)
	recipients, err := normalizedAddressList(recipientValue, 20)
	if err != nil || len(recipients) == 0 {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail reply recipient is unavailable", err)
	}
	subject := decodedGmailHeader(original.Payload.Headers, "Subject", 990)
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(subject)), "re:") {
		subject = "Re: " + subject
	}
	if strings.TrimSpace(subject) == "" || len([]rune(subject)) > 998 || containsHeaderBreak(subject) {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail reply subject is invalid", nil)
	}
	messageID := strings.TrimSpace(gmailHeaderValue(original.Payload.Headers, "Message-ID"))
	if messageID == "" || len([]rune(messageID)) > 998 || containsHeaderBreak(messageID) {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Gmail source message has no valid Message-ID", nil)
	}
	references := strings.TrimSpace(gmailHeaderValue(original.Payload.Headers, "References"))
	if containsHeaderBreak(references) {
		references = ""
	}
	if references != "" {
		references += " "
	}
	references += messageID
	if len([]rune(references)) > maxReferencesCharacters {
		references = messageID
	}

	var builder strings.Builder
	builder.WriteString("To: " + strings.Join(recipients, ", ") + "\r\n")
	builder.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\n")
	builder.WriteString("In-Reply-To: " + messageID + "\r\n")
	builder.WriteString("References: " + references + "\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	builder.WriteString(wrapBase64(base64.StdEncoding.EncodeToString([]byte(bodyText)), 76))
	builder.WriteString("\r\n")
	return []byte(builder.String()), nil
}

func validGmailID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 255 {
		return false
	}
	for _, char := range value {
		if !(unicode.IsLetter(char) || unicode.IsDigit(char) || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

func optionalNonBlankString(input map[string]interface{}, key string, maxLength int) (string, error) {
	value, exists := input[key]
	if !exists || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" || len([]rune(strings.TrimSpace(text))) > maxLength {
		return "", integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail "+key+" is invalid", nil)
	}
	return strings.TrimSpace(text), nil
}

func boundedInputInt(input map[string]interface{}, key string, defaultValue, minimum, maximum int) (int, error) {
	value, exists := input[key]
	if !exists || value == nil {
		return defaultValue, nil
	}
	parsed := 0
	switch typed := value.(type) {
	case int:
		parsed = typed
	case int32:
		parsed = int(typed)
	case int64:
		parsed = int(typed)
	case float64:
		if typed != float64(int(typed)) {
			return 0, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail "+key+" is invalid", nil)
		}
		parsed = int(typed)
	case json.Number:
		value, err := typed.Int64()
		if err != nil {
			return 0, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail "+key+" is invalid", err)
		}
		parsed = int(value)
	default:
		return 0, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail "+key+" is invalid", nil)
	}
	if parsed < minimum || parsed > maximum {
		return 0, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail "+key+" is out of range", nil)
	}
	return parsed, nil
}

func optionalInputBool(input map[string]interface{}, key string) (bool, error) {
	value, exists := input[key]
	if !exists || value == nil {
		return false, nil
	}
	parsed, ok := value.(bool)
	if !ok {
		return false, integrations.NewError(integrations.ErrorCodeInvalidInput, "Gmail "+key+" is invalid", nil)
	}
	return parsed, nil
}

func mergeResponseMeta(base, next responseMeta) responseMeta {
	base.Attempts += next.Attempts
	if base.RequestID == "" {
		base.RequestID = next.RequestID
	}
	if next.Diagnostics != (integrations.ProviderDiagnostics{}) {
		base.Diagnostics = next.Diagnostics
	}
	return base
}
