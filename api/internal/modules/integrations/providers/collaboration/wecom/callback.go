package wecom

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

type CallbackEvent struct {
	InfoType   string
	SuiteID    string
	Ticket     string
	AuthCode   string
	AuthCorpID string
	State      string
	Timestamp  int64
}

type encryptedCallbackEnvelope struct {
	Encrypt string `xml:"Encrypt"`
}

type callbackEventXML struct {
	SuiteID     string `xml:"SuiteId"`
	InfoType    string `xml:"InfoType"`
	SuiteTicket string `xml:"SuiteTicket"`
	AuthCode    string `xml:"AuthCode"`
	AuthCorpID  string `xml:"AuthCorpId"`
	State       string `xml:"State"`
	Timestamp   int64  `xml:"TimeStamp"`
}

const (
	callbackFailureStageUnknown                  = "unknown"
	callbackFailureStageConfigurationUnavailable = "configuration_unavailable"
	callbackFailureStageReceiverInvalid          = "receiver_invalid"
	callbackFailureStageInputIncomplete          = "input_incomplete"
	callbackFailureStageSignatureMismatch        = "signature_mismatch"
	callbackFailureStageAESKeyInvalid            = "aes_key_invalid"
	callbackFailureStageCiphertextInvalid        = "ciphertext_invalid"
	callbackFailureStageCipherUnavailable        = "cipher_unavailable"
	callbackFailureStagePaddingInvalid           = "padding_invalid"
	callbackFailureStagePlaintextTooShort        = "plaintext_too_short"
	callbackFailureStageMessageLengthInvalid     = "message_length_invalid"
	callbackFailureStageReceiverMismatch         = "receiver_mismatch"
)

type callbackVerificationFailure struct {
	stage string
	err   error
}

func (failure *callbackVerificationFailure) Error() string {
	if failure == nil || failure.err == nil {
		return "WeCom callback verification failed"
	}
	return failure.err.Error()
}

func (failure *callbackVerificationFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.err
}

func newCallbackVerificationFailure(stage string, err error) error {
	return &callbackVerificationFailure{stage: stage, err: err}
}

// CallbackFailureStage returns a bounded, non-sensitive diagnostic stage for
// callback verification failures. It deliberately excludes callback values,
// credentials, ciphertext, and receiver identifiers.
func CallbackFailureStage(err error) string {
	if err == nil {
		return ""
	}
	var failure *callbackVerificationFailure
	if errors.As(err, &failure) && failure != nil && failure.stage != "" {
		return failure.stage
	}
	return callbackFailureStageUnknown
}

func (adapter *Adapter) VerifySuiteCallbackURL(signature, timestamp, nonce, echo string) (string, error) {
	if adapter == nil {
		return "", newCallbackVerificationFailure(
			callbackFailureStageConfigurationUnavailable,
			wecomError(integrations.ErrorCodeDisabled, "WeCom third-party callback is not configured", nil),
		)
	}
	// WeCom validates both third-party callback URLs with the service
	// provider CorpID. Later POST command events are encrypted for SuiteID.
	plaintext, err := adapter.decryptCallback(adapter.suite.ProviderCorpID, signature, timestamp, nonce, normalizeCallbackCiphertext(echo))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (adapter *Adapter) VerifyDataCallbackURL(receiverID, signature, timestamp, nonce, echo string) (string, error) {
	plaintext, err := adapter.decryptCallback(receiverID, signature, timestamp, nonce, normalizeCallbackCiphertext(echo))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (adapter *Adapter) ReceiveSuiteCallback(ctx context.Context, signature, timestamp, nonce string, body []byte) (CallbackEvent, error) {
	if adapter == nil || !adapter.suite.Configured() || adapter.suiteState == nil {
		return CallbackEvent{}, wecomError(integrations.ErrorCodeDisabled, "WeCom third-party callback is not configured", nil)
	}
	if len(body) == 0 || len(body) > 2<<20 {
		return CallbackEvent{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom callback body is invalid", nil)
	}
	var envelope encryptedCallbackEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil || strings.TrimSpace(envelope.Encrypt) == "" {
		return CallbackEvent{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom callback envelope is invalid", err)
	}
	plaintext, err := adapter.decryptCallback(adapter.suite.SuiteID, signature, timestamp, nonce, strings.TrimSpace(envelope.Encrypt))
	if err != nil {
		return CallbackEvent{}, err
	}
	var payload callbackEventXML
	if err := xml.Unmarshal(plaintext, &payload); err != nil {
		return CallbackEvent{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom callback payload is invalid", err)
	}
	event := CallbackEvent{
		InfoType: strings.ToLower(strings.TrimSpace(payload.InfoType)), SuiteID: strings.TrimSpace(payload.SuiteID),
		Ticket: strings.TrimSpace(payload.SuiteTicket), AuthCode: strings.TrimSpace(payload.AuthCode),
		AuthCorpID: strings.TrimSpace(payload.AuthCorpID), State: strings.TrimSpace(payload.State), Timestamp: payload.Timestamp,
	}
	if event.SuiteID != strings.TrimSpace(adapter.suite.SuiteID) || event.InfoType == "" {
		return CallbackEvent{}, wecomError(integrations.ErrorCodeAuthInvalid, "WeCom callback recipient is invalid", nil)
	}
	if event.InfoType == "suite_ticket" {
		// Ticket validity is measured from successful receipt. The event timestamp
		// is provider-controlled metadata and may be delayed during retries.
		if err := adapter.suiteState.SaveTicket(ctx, event.SuiteID, event.Ticket, time.Now().UTC()); err != nil {
			return CallbackEvent{}, err
		}
	}
	return event, nil
}

func (adapter *Adapter) ReceiveDataCallback(receiverID, signature, timestamp, nonce string, body []byte) error {
	if adapter == nil || !adapter.suite.Configured() {
		return wecomError(integrations.ErrorCodeDisabled, "WeCom third-party callback is not configured", nil)
	}
	if len(body) == 0 || len(body) > 2<<20 {
		return wecomError(integrations.ErrorCodeResponseInvalid, "WeCom callback body is invalid", nil)
	}
	var envelope encryptedCallbackEnvelope
	if err := xml.Unmarshal(body, &envelope); err != nil || strings.TrimSpace(envelope.Encrypt) == "" {
		return wecomError(integrations.ErrorCodeResponseInvalid, "WeCom callback envelope is invalid", err)
	}
	plaintext, err := adapter.decryptCallback(receiverID, signature, timestamp, nonce, strings.TrimSpace(envelope.Encrypt))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(plaintext)) == 0 {
		return wecomError(integrations.ErrorCodeResponseInvalid, "WeCom callback payload is invalid", nil)
	}
	return nil
}

func (adapter *Adapter) decryptCallback(receiverID, signature, timestamp, nonce, encrypted string) ([]byte, error) {
	if adapter == nil || !adapter.suite.Configured() {
		return nil, newCallbackVerificationFailure(
			callbackFailureStageConfigurationUnavailable,
			wecomError(integrations.ErrorCodeDisabled, "WeCom third-party callback is not configured", nil),
		)
	}
	receiverID = strings.TrimSpace(receiverID)
	if receiverID == "" || len(receiverID) > 128 {
		return nil, newCallbackVerificationFailure(
			callbackFailureStageReceiverInvalid,
			wecomError(integrations.ErrorCodeAuthInvalid, "WeCom callback receiver is invalid", nil),
		)
	}
	if strings.TrimSpace(signature) == "" || strings.TrimSpace(timestamp) == "" || strings.TrimSpace(nonce) == "" || strings.TrimSpace(encrypted) == "" {
		return nil, newCallbackVerificationFailure(
			callbackFailureStageInputIncomplete,
			wecomError(integrations.ErrorCodeAuthInvalid, "WeCom callback signature is incomplete", nil),
		)
	}
	expected := callbackSignature(adapter.suite.CallbackToken, timestamp, nonce, encrypted)
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(strings.TrimSpace(signature))), []byte(expected)) != 1 {
		return nil, newCallbackVerificationFailure(
			callbackFailureStageSignatureMismatch,
			wecomError(integrations.ErrorCodeAuthInvalid, "WeCom callback signature is invalid", nil),
		)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(adapter.suite.EncodingAESKey) + "=")
	if err != nil || len(key) != 32 {
		return nil, newCallbackVerificationFailure(
			callbackFailureStageAESKeyInvalid,
			wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom callback AES key is invalid", err),
		)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encrypted))
	if err != nil || len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, newCallbackVerificationFailure(
			callbackFailureStageCiphertextInvalid,
			wecomError(integrations.ErrorCodeResponseInvalid, "WeCom callback ciphertext is invalid", err),
		)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, newCallbackVerificationFailure(
			callbackFailureStageCipherUnavailable,
			wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom callback cipher is unavailable", err),
		)
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, key[:aes.BlockSize]).CryptBlocks(plaintext, ciphertext)
	plaintext, err = removeWeComPKCS7Padding(plaintext)
	if err != nil {
		return nil, newCallbackVerificationFailure(
			callbackFailureStagePaddingInvalid,
			wecomError(integrations.ErrorCodeResponseInvalid, "WeCom callback plaintext is invalid", err),
		)
	}
	if len(plaintext) < 20 {
		return nil, newCallbackVerificationFailure(
			callbackFailureStagePlaintextTooShort,
			wecomError(integrations.ErrorCodeResponseInvalid, "WeCom callback plaintext is invalid", nil),
		)
	}
	messageLength := int(binary.BigEndian.Uint32(plaintext[16:20]))
	if messageLength < 0 || 20+messageLength > len(plaintext) {
		return nil, newCallbackVerificationFailure(
			callbackFailureStageMessageLengthInvalid,
			wecomError(integrations.ErrorCodeResponseInvalid, "WeCom callback message length is invalid", nil),
		)
	}
	message := append([]byte(nil), plaintext[20:20+messageLength]...)
	actualReceiverID := strings.TrimSpace(string(plaintext[20+messageLength:]))
	if actualReceiverID != receiverID {
		return nil, newCallbackVerificationFailure(
			callbackFailureStageReceiverMismatch,
			wecomError(integrations.ErrorCodeAuthInvalid, "WeCom callback receiver is invalid", nil),
		)
	}
	return message, nil
}

func callbackSignature(token, timestamp, nonce, encrypted string) string {
	parts := []string{strings.TrimSpace(token), strings.TrimSpace(timestamp), strings.TrimSpace(nonce), strings.TrimSpace(encrypted)}
	sort.Strings(parts)
	digest := sha1.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(digest[:])
}

// Query decoders treat a literal '+' as a space. The callback ciphertext is
// standard Base64, where spaces are invalid and '+' is valid, so restoring the
// Base64 character is safe and keeps signature verification strict across
// reverse proxies that decode the query before forwarding it.
func normalizeCallbackCiphertext(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, " ", "+"))
}

func removeWeComPKCS7Padding(value []byte) ([]byte, error) {
	if len(value) == 0 {
		return nil, fmt.Errorf("empty plaintext")
	}
	padding := int(value[len(value)-1])
	if padding < 1 || padding > 32 || padding > len(value) {
		return nil, fmt.Errorf("invalid padding")
	}
	if !bytes.Equal(value[len(value)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return nil, fmt.Errorf("invalid padding")
	}
	return value[:len(value)-padding], nil
}
