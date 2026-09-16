package mailprotocol

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message"
	_ "github.com/emersion/go-message/charset"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"golang.org/x/net/html"
)

type reference struct {
	Kind     string `json:"kind"`
	Scope    string `json:"scope"`
	Mailbox  string `json:"mailbox"`
	Validity uint32 `json:"validity"`
	UID      uint32 `json:"uid"`
	Before   uint32 `json:"before"`
	Expires  int64  `json:"expires"`
}

func scopeHash(scope providers.ResourceScope) string {
	// A new invocation may consume a prior handle, but the accepted binding,
	// connection, credential generation and target must remain identical.
	scope.LogicalOperationID = ""
	encoded, _ := json.Marshal(scope)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
func (c *Client) sealReference(in providers.CredentialInput, r reference) (string, error) {
	r.Scope = scopeHash(in.ResourceScope)
	r.Expires = c.now().Add(15 * time.Minute).Unix()
	encoded, err := json.Marshal(r)
	if err != nil {
		return "", providers.Invalid()
	}
	nonce := make([]byte, c.handles.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", providers.Invalid()
	}
	return base64.RawURLEncoding.EncodeToString(c.handles.Seal(nonce, nonce, encoded, []byte(c.profile.ID+"-reference-v1"))), nil
}
func (c *Client) openReference(in providers.CredentialInput, value, kind string) (reference, error) {
	var r reference
	if len(value) > 8192 {
		return r, expiredReference()
	}
	encoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(encoded) < c.handles.NonceSize() {
		return r, expiredReference()
	}
	n := c.handles.NonceSize()
	plain, err := c.handles.Open(nil, encoded[:n], encoded[n:], []byte(c.profile.ID+"-reference-v1"))
	if err != nil || json.Unmarshal(plain, &r) != nil || r.Kind != kind || r.Scope != scopeHash(in.ResourceScope) || r.Expires <= c.now().Unix() || r.Validity == 0 || !validMailbox(r.Mailbox) {
		return reference{}, expiredReference()
	}
	return r, nil
}
func expiredReference() error { return &providers.Failure{Code: core.ErrorCodeMailReferenceExpired} }
func validMailbox(value string) bool {
	return value != "" && len(value) <= 1024 && !strings.ContainsAny(value, "\r\n\x00")
}

func (c *Client) read(ctx context.Context, in providers.CredentialInput, operation string, args map[string]any) (map[string]any, error) {
	mailbox := providers.Text(args, "mailbox")
	if mailbox == "" {
		mailbox = "INBOX"
	}
	if !validMailbox(mailbox) {
		return nil, providers.Invalid()
	}
	var ref reference
	var err error
	if operation == c.profile.ID+".message.get" {
		ref, err = c.openReference(in, providers.Text(args, "message_handle"), "message")
		mailbox = ref.Mailbox
	}
	if cursor := providers.Text(args, "cursor"); cursor != "" {
		ref, err = c.openReference(in, cursor, "page")
		if providers.Text(args, "mailbox") != "" && mailbox != ref.Mailbox {
			return nil, expiredReference()
		}
		mailbox = ref.Mailbox
	}
	if err != nil {
		return nil, err
	}
	cl, closeSession, err := c.openIMAP(ctx, in)
	if err != nil {
		return nil, err
	}
	defer closeSession()
	if operation == c.profile.ID+".folder.list" {
		return listFolders(cl, closeSession)
	}
	status, err := cl.Select(mailbox, true)
	if err != nil {
		return nil, c.readFailure(err)
	}
	if status.UidValidity == 0 {
		return nil, expiredReference()
	}
	if ref.Validity != 0 && ref.Validity != status.UidValidity {
		return nil, expiredReference()
	}
	switch operation {
	case c.profile.ID + ".message.list":
		return c.listMessages(cl, closeSession, in, mailbox, status, ref, providers.Number(args, "limit"))
	case c.profile.ID + ".message.get":
		return getMessage(cl, closeSession, ref.UID)
	default:
		return nil, providers.Invalid()
	}
}
func listFolders(cl *client.Client, closeSession func()) (map[string]any, error) {
	ch := make(chan *imap.MailboxInfo)
	done := make(chan error, 1)
	go func() { done <- cl.List("", "*", ch) }()
	folders := []map[string]any{}
	limited := false
	for item := range ch {
		if len(folders) >= 200 || !validMailbox(item.Name) {
			limited = true
			closeSession()
			continue
		}
		folders = append(folders, map[string]any{"name": item.Name})
	}
	err := <-done
	if limited {
		return nil, &providers.Failure{Code: core.ErrorCodeMailSizeExceeded}
	}
	if err != nil {
		return nil, protocolFailure(err, false)
	}
	return map[string]any{"folders": folders}, nil
}
func fetch(cl *client.Client, closeSession func(), set *imap.SeqSet, items []imap.FetchItem, max int) ([]*imap.Message, error) {
	ch := make(chan *imap.Message)
	done := make(chan error, 1)
	go func() { done <- cl.UidFetch(set, items, ch) }()
	result := []*imap.Message{}
	limited := false
	for msg := range ch {
		if len(result) >= max {
			limited = true
			closeSession()
			continue
		}
		result = append(result, msg)
	}
	err := <-done
	if limited {
		return nil, &providers.Failure{Code: core.ErrorCodeMailSizeExceeded}
	}
	if err != nil {
		return nil, protocolFailure(err, false)
	}
	return result, nil
}
func (c *Client) listMessages(cl *client.Client, closeSession func(), in providers.CredentialInput, mailbox string, status *imap.MailboxStatus, ref reference, limit int) (map[string]any, error) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 50 {
		return nil, providers.Invalid()
	}
	result := []map[string]any{}
	before := status.UidNext
	if ref.Before > 0 {
		before = ref.Before
	}
	if status.Messages == 0 {
		return map[string]any{"messages": result, "next_cursor": ""}, nil
	}
	if before == 0 {
		return nil, expiredReference()
	}
	// UID windows keep the protocol response bounded even for huge mailboxes.
	// Sparse windows can return an empty page with a continuation cursor.
	for window := 0; window < 20 && before > 1 && len(result) < limit; window++ {
		high := before - 1
		low := uint32(1)
		if high > 255 {
			low = high - 255
		}
		set := new(imap.SeqSet)
		set.AddRange(low, high)
		messages, err := fetch(cl, closeSession, set, []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope}, 256)
		if err != nil {
			return nil, err
		}
		sort.Slice(messages, func(i, j int) bool { return messages[i].Uid > messages[j].Uid })
		before = low
		for _, m := range messages {
			if m.Uid < low || m.Uid > high || m.Envelope == nil {
				return nil, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
			}
			token, err := c.sealReference(in, reference{Kind: "message", Mailbox: mailbox, Validity: status.UidValidity, UID: m.Uid})
			if err != nil {
				return nil, err
			}
			from := ""
			if len(m.Envelope.From) > 0 {
				from = m.Envelope.From[0].Address()
			}
			subject, decodeErr := (&mime.WordDecoder{CharsetReader: message.CharsetReader}).DecodeHeader(m.Envelope.Subject)
			if decodeErr != nil {
				subject = m.Envelope.Subject
			}
			result = append(result, map[string]any{"message_handle": token, "subject": truncate(subject, 998), "from": truncate(from, 1024), "date": m.Envelope.Date.UTC().Format(time.RFC3339)})
			if len(result) == limit {
				before = m.Uid
				break
			}
		}
	}
	next := ""
	if before > 1 {
		var err error
		next, err = c.sealReference(in, reference{Kind: "page", Mailbox: mailbox, Validity: status.UidValidity, Before: before})
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"messages": result, "next_cursor": next}, nil
}
func getMessage(cl *client.Client, closeSession func(), uid uint32) (map[string]any, error) {
	if uid == 0 {
		return nil, expiredReference()
	}
	set := new(imap.SeqSet)
	set.AddNum(uid)
	metadata, err := fetch(cl, closeSession, set, []imap.FetchItem{imap.FetchUid, imap.FetchRFC822Size}, 1)
	if err != nil {
		return nil, err
	}
	if len(metadata) != 1 || metadata[0].Uid != uid {
		return nil, expiredReference()
	}
	if metadata[0].Size > maxMessageBytes {
		return nil, &providers.Failure{Code: core.ErrorCodeMailSizeExceeded}
	}
	section := &imap.BodySectionName{Peek: true, Partial: []int{0, maxMessageBytes}}
	messages, err := fetch(cl, closeSession, set, []imap.FetchItem{imap.FetchUid, section.FetchItem()}, 1)
	if err != nil {
		return nil, err
	}
	if len(messages) != 1 || messages[0].Uid != uid {
		return nil, expiredReference()
	}
	body := messages[0].GetBody(section)
	if body == nil {
		return nil, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	raw, err := io.ReadAll(io.LimitReader(body, maxMessageBytes+1))
	if err != nil || len(raw) > maxMessageBytes {
		return nil, &providers.Failure{Code: core.ErrorCodeMailSizeExceeded}
	}
	text, truncated, err := plainText(raw)
	if err != nil {
		return nil, err
	}
	return map[string]any{"text": text, "truncated": truncated}, nil
}

var errTextComplete = errors.New("mail text limit reached")

func plainText(raw []byte) (string, bool, error) {
	entity, err := message.ReadWithOptions(bytes.NewReader(raw), &message.ReadOptions{MaxHeaderBytes: maxHeaderBytes})
	if err != nil {
		return "", false, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	var output strings.Builder
	var htmlFallback strings.Builder
	truncated := false
	parts := 0
	err = entity.Walk(func(path []int, part *message.Entity, partErr error) error {
		parts++
		if len(path) > 16 || parts > 100 {
			return errResponseLimit
		}
		if partErr != nil {
			return partErr
		}
		disposition, _, _ := part.Header.ContentDisposition()
		media, _, _ := part.Header.ContentType()
		if disposition == "attachment" || (media != "text/plain" && media != "text/html") {
			return nil
		}
		if media == "text/html" {
			if htmlFallback.Len() == 0 {
				data, err := io.ReadAll(io.LimitReader(part.Body, maxMessageBytes+1))
				if err != nil {
					return err
				}
				if len(data) > maxMessageBytes {
					return errResponseLimit
				}
				safe, cut, err := htmlText(data)
				if err != nil {
					return err
				}
				htmlFallback.WriteString(safe)
				truncated = truncated || cut
			}
			return nil
		}
		remaining := maxTextBytes - output.Len()
		data, err := io.ReadAll(io.LimitReader(part.Body, int64(remaining+1)))
		if err != nil {
			return err
		}
		if len(data) > remaining {
			data = data[:remaining]
			truncated = true
		}
		output.WriteString(strings.ToValidUTF8(string(data), "�"))
		if truncated {
			return errTextComplete
		}
		return nil
	})
	if err != nil && !errors.Is(err, errTextComplete) {
		return "", false, protocolFailure(err, false)
	}
	text := output.String()
	if text == "" {
		text = htmlFallback.String()
	}
	if len(text) > maxTextBytes {
		text = truncate(text, maxTextBytes)
		truncated = true
	}
	return text, truncated, nil
}

// HTML is converted to text; script/style content and all resource elements are
// discarded. No remote URL is fetched or emitted as executable markup.
func htmlText(raw []byte) (string, bool, error) {
	tokens := html.NewTokenizer(bytes.NewReader(raw))
	tokens.SetMaxBuf(maxHeaderBytes)
	var result strings.Builder
	skip := 0
	for {
		kind := tokens.Next()
		if kind == html.ErrorToken {
			if err := tokens.Err(); err != io.EOF {
				return "", false, err
			}
			return result.String(), false, nil
		}
		token := tokens.Token()
		switch kind {
		case html.StartTagToken:
			if token.Data == "script" || token.Data == "style" {
				skip++
			}
			if token.Data == "br" || token.Data == "p" || token.Data == "div" {
				if skip == 0 {
					result.WriteByte('\n')
				}
			}
		case html.EndTagToken:
			if (token.Data == "script" || token.Data == "style") && skip > 0 {
				skip--
			}
		case html.TextToken:
			if skip == 0 {
				result.WriteString(token.Data)
			}
		}
		if result.Len() > maxTextBytes {
			return truncate(result.String(), maxTextBytes), true, nil
		}
	}
}
func truncate(value string, max int) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= max {
		return value
	}
	value = value[:max]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
