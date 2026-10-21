package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
)

type resourceReference struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Scope   string `json:"scope"`
	Query   string `json:"query,omitempty"`
	Expires int64  `json:"expires"`
}

func NewWithHandleKey(client *http.Client, enabled bool, store messaging.Store, key string) (*Client, error) {
	c, err := New(client, enabled, store)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, providers.Invalid()
	}
	digest := sha256.Sum256([]byte("zgi/wecom/resources/v1\x00" + key))
	block, err := aes.NewCipher(digest[:])
	if err != nil {
		return nil, err
	}
	c.handles, err = cipher.NewGCM(block)
	return c, err
}

func resourceScope(in providers.CredentialInput) string {
	scope := in.ResourceScope
	scope.LogicalOperationID = ""
	raw, _ := json.Marshal(struct {
		Scope            providers.ResourceScope
		CredentialDigest string
	}{scope, messaging.CredentialDigest(in.Values)})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func validReadScope(in providers.CredentialInput) bool {
	s := in.ResourceScope
	return s.OrganizationID != uuid.Nil && s.OwnerAccountID != uuid.Nil && s.ConnectionID != uuid.Nil && s.CredentialVersion > 0 && s.BindingID != uuid.Nil && s.BindingRevision > 0 && s.Target.Validate() == nil
}
func (c *Client) sealResource(in providers.CredentialInput, kind, id, query string) (string, error) {
	if c.handles == nil || !validReadScope(in) || id == "" || len(id) > 4096 || strings.ContainsAny(id, "\r\n\x00") {
		return "", providers.Invalid()
	}
	r := resourceReference{Kind: kind, ID: id, Query: query, Scope: resourceScope(in), Expires: time.Now().Add(15 * time.Minute).Unix()}
	raw, _ := json.Marshal(r)
	nonce := make([]byte, c.handles.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", providers.Invalid()
	}
	return base64.RawURLEncoding.EncodeToString(c.handles.Seal(nonce, nonce, raw, []byte("wecom-resource-v1"))), nil
}
func (c *Client) openResource(in providers.CredentialInput, value, kind, query string) (string, error) {
	if c.handles == nil || !validReadScope(in) || len(value) > 8192 {
		return "", providers.Invalid()
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	n := c.handles.NonceSize()
	if err != nil || len(raw) < n {
		return "", providers.Invalid()
	}
	plain, err := c.handles.Open(nil, raw[:n], raw[n:], []byte("wecom-resource-v1"))
	if err != nil {
		return "", providers.Invalid()
	}
	var r resourceReference
	if json.Unmarshal(plain, &r) != nil || r.Kind != kind || r.Query != query || r.Scope != resourceScope(in) || r.Expires <= time.Now().Unix() || r.ID == "" {
		return "", providers.Invalid()
	}
	return r.ID, nil
}
