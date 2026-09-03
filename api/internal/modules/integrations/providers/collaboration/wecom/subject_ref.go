package wecom

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	SubjectTypeMember          = "member"
	SubjectTypeCustomer        = "customer"
	SubjectTypeCustomerGroup   = "customer_group"
	SubjectTypeMassMessageTask = "customer_mass_message_task"

	SubjectIDKindUserID         = "userid"
	SubjectIDKindOpenUserID     = "open_userid"
	SubjectIDKindExternalUserID = "external_userid"
	SubjectIDKindChatID         = "chat_id"
	SubjectIDKindMsgID          = "msgid"
)

const subjectReferenceVersion = 2

var subjectReferenceAAD = []byte("zgi/wecom/subject-reference/v2")

// subjectReferenceClaims is the provider-owned identity contract carried by
// recipient and customer references. Provider identifiers remain behind the
// opaque, authenticated reference and are never accepted as action input on
// their own.
type subjectReferenceClaims struct {
	Version         int    `json:"v"`
	SubjectType     string `json:"subject_type"`
	IDKind          string `json:"id_kind"`
	SubjectID       string `json:"subject_id"`
	AlternateIDKind string `json:"alternate_id_kind,omitempty"`
	AlternateID     string `json:"alternate_id,omitempty"`
	ConnectionBind  string `json:"connection_bind"`
	TenantBind      string `json:"tenant_bind"`
	AgentBind       string `json:"agent_bind"`
}

func (adapter *Adapter) encodeSubjectRef(
	creds credentials,
	subjectType string,
	idKind string,
	subjectID string,
	alternateIDKind string,
	alternateID string,
) (string, error) {
	claims := subjectReferenceClaims{
		Version:         subjectReferenceVersion,
		SubjectType:     strings.TrimSpace(subjectType),
		IDKind:          strings.TrimSpace(idKind),
		SubjectID:       strings.TrimSpace(subjectID),
		AlternateIDKind: strings.TrimSpace(alternateIDKind),
		AlternateID:     strings.TrimSpace(alternateID),
		ConnectionBind:  adapter.subjectBinding("connection", creds.ConnectionID),
		TenantBind:      adapter.subjectBinding("tenant", creds.AuthCorpID),
		AgentBind:       adapter.subjectBinding("agent", creds.AgentID),
	}
	if err := validateSubjectReferenceClaims(claims); err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal WeCom subject reference: %w", err)
	}
	aead, err := adapter.subjectReferenceAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("create WeCom subject reference nonce: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, payload, subjectReferenceAAD)
	return "v2." + base64.RawURLEncoding.EncodeToString(nonce) + "." + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// decodeSubjectRef authenticates and decrypts the reference, then verifies its
// version, connection, tenant, agent, subject type, and identifier domain
// before returning provider identifiers.
func (adapter *Adapter) decodeSubjectRef(
	value string,
	creds credentials,
	expectedSubjectType string,
	expectedIDKind string,
) (subjectReferenceClaims, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 4096 {
		return subjectReferenceClaims{}, fmt.Errorf("invalid subject reference")
	}
	if !strings.HasPrefix(value, "v2.") {
		return subjectReferenceClaims{}, fmt.Errorf("legacy subject reference is not accepted")
	}

	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "v2" {
		return subjectReferenceClaims{}, fmt.Errorf("invalid subject reference")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return subjectReferenceClaims{}, fmt.Errorf("invalid subject reference nonce")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(ciphertext) == 0 || len(ciphertext) > 3072 {
		return subjectReferenceClaims{}, fmt.Errorf("invalid subject reference ciphertext")
	}
	aead, err := adapter.subjectReferenceAEAD()
	if err != nil {
		return subjectReferenceClaims{}, err
	}
	if len(nonce) != aead.NonceSize() {
		return subjectReferenceClaims{}, fmt.Errorf("invalid subject reference nonce")
	}
	payload, err := aead.Open(nil, nonce, ciphertext, subjectReferenceAAD)
	if err != nil || len(payload) == 0 || len(payload) > 3072 {
		return subjectReferenceClaims{}, fmt.Errorf("invalid subject reference ciphertext")
	}

	var claims subjectReferenceClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return subjectReferenceClaims{}, fmt.Errorf("invalid subject reference payload")
	}
	if err := validateSubjectReferenceClaims(claims); err != nil {
		return subjectReferenceClaims{}, err
	}
	if expectedSubjectType != "" && claims.SubjectType != expectedSubjectType {
		return subjectReferenceClaims{}, fmt.Errorf("unexpected subject type")
	}
	if expectedIDKind != "" && claims.IDKind != expectedIDKind {
		return subjectReferenceClaims{}, fmt.Errorf("unexpected subject identifier kind")
	}
	if claims.ConnectionBind != adapter.subjectBinding("connection", creds.ConnectionID) ||
		claims.TenantBind != adapter.subjectBinding("tenant", creds.AuthCorpID) ||
		claims.AgentBind != adapter.subjectBinding("agent", creds.AgentID) {
		return subjectReferenceClaims{}, fmt.Errorf("subject reference binding mismatch")
	}
	return claims, nil
}

func (adapter *Adapter) subjectReferenceAEAD() (cipher.AEAD, error) {
	secret := strings.TrimSpace(adapter.suite.SuiteSecret)
	if secret == "" {
		return nil, fmt.Errorf("WeCom subject reference encryption key is unavailable")
	}
	key := sha256.Sum256([]byte("zgi/wecom/subject-reference/encryption/v2\x00" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create WeCom subject reference cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create WeCom subject reference AEAD: %w", err)
	}
	return aead, nil
}

func (adapter *Adapter) subjectBinding(kind, value string) string {
	secret := strings.TrimSpace(adapter.suite.SuiteSecret)
	if secret == "" || strings.TrimSpace(value) == "" {
		return ""
	}
	key := sha256.Sum256([]byte("zgi/wecom/subject-reference/binding/v2\x00" + secret))
	digest := hmac.New(sha256.New, key[:])
	_, _ = digest.Write([]byte(kind + "\x00" + strings.TrimSpace(value)))
	return base64.RawURLEncoding.EncodeToString(digest.Sum(nil)[:18])
}

func validateSubjectReferenceClaims(claims subjectReferenceClaims) error {
	if claims.Version != subjectReferenceVersion ||
		!validSubjectType(claims.SubjectType) ||
		!validSubjectIDKind(claims.IDKind) ||
		!validProviderSubjectID(claims.SubjectID) ||
		claims.ConnectionBind == "" || claims.TenantBind == "" || claims.AgentBind == "" {
		return fmt.Errorf("invalid subject reference claims")
	}
	if (claims.AlternateID == "") != (claims.AlternateIDKind == "") {
		return fmt.Errorf("invalid alternate subject identifier")
	}
	if claims.AlternateID != "" && (!validSubjectIDKind(claims.AlternateIDKind) || !validProviderSubjectID(claims.AlternateID)) {
		return fmt.Errorf("invalid alternate subject identifier")
	}
	return nil
}

func validSubjectType(value string) bool {
	switch value {
	case SubjectTypeMember, SubjectTypeCustomer, SubjectTypeCustomerGroup, SubjectTypeMassMessageTask:
		return true
	default:
		return false
	}
}

func validSubjectIDKind(value string) bool {
	switch value {
	case SubjectIDKindUserID, SubjectIDKindOpenUserID, SubjectIDKindExternalUserID, SubjectIDKindChatID, SubjectIDKindMsgID:
		return true
	default:
		return false
	}
}

func validProviderSubjectID(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 256 && !strings.ContainsAny(value, "|\r\n")
}
