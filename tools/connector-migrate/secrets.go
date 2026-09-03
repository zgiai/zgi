package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var errSecret = errors.New("secret conversion failed; check the source identity and keyring")

type keyrings struct {
	Active   string            `json:"active"`
	Native   map[string]string `json:"native"`
	Previous map[string]string `json:"previous"`
}
type secretSubject struct {
	Org, ID, Connector, Purpose string
	Version                     int
}

func newAEAD(master string, native bool) (cipher.AEAD, error) {
	if len(master) != 32 {
		return nil, errSecret
	}
	var key []byte
	if native {
		var err error
		key, err = hkdf.Key(sha256.New, []byte(master), []byte("zgi/connector-secrets/c1"), "aes-256-gcm", 32)
		if err != nil {
			return nil, errSecret
		}
	} else {
		mac := hmac.New(sha256.New, []byte(master))
		mac.Write([]byte("zgi/integration-credentials/aes-256-gcm/v1"))
		key = mac.Sum(nil)
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errSecret
	}
	return cipher.NewGCM(block)
}
func nativeAAD(s secretSubject, key string) []byte {
	data, _ := json.Marshal([]any{"zgi/connector-secrets", 1, s.Purpose, key, s.Org, s.ID, s.Connector, s.Version})
	return data
}
func previousAAD(s secretSubject, version, key string) []byte {
	prefix := "zgi/integration-credentials/" + version + "\n"
	if version == "v2" {
		prefix += "key=" + key + "\n"
	}
	return []byte(fmt.Sprintf("%sorganization=%s\nconnection=%s\nintegration=%s\nversion=%d", prefix, s.Org, s.ID, s.Connector, s.Version))
}
func decryptEnvelope(keys keyrings, s secretSubject, envelope string) ([]byte, error) {
	if len(envelope) > 1024*1024 {
		return nil, errSecret
	}
	if strings.HasPrefix(envelope, "c1.") {
		parts := strings.Split(envelope, ".")
		if len(parts) != 3 {
			return nil, errSecret
		}
		id, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, errSecret
		}
		return openPayload(keys.Native[string(id)], true, parts[2], nativeAAD(s, string(id)))
	}
	if strings.HasPrefix(envelope, "v2.") {
		// The historical key id was unencoded and may itself contain dots. Match
		// the longest known id rather than splitting at an ambiguous separator.
		ids := make([]string, 0, len(keys.Previous))
		for id := range keys.Previous {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return len(ids[i]) > len(ids[j]) })
		for _, id := range ids {
			prefix := "v2." + id + "."
			if strings.HasPrefix(envelope, prefix) {
				return openPayload(keys.Previous[id], false, strings.TrimPrefix(envelope, prefix), previousAAD(s, "v2", id))
			}
		}
		return nil, errSecret
	}
	if strings.HasPrefix(envelope, "v1.") {
		var result []byte
		for _, master := range keys.Previous {
			data, err := openPayload(master, false, strings.TrimPrefix(envelope, "v1."), previousAAD(s, "v1", ""))
			if err == nil {
				if result != nil {
					clear(data)
					clear(result)
					return nil, errSecret
				}
				result = data
			}
		}
		if result != nil {
			return result, nil
		}
	}
	return nil, errSecret
}
func openPayload(master string, native bool, encoded string, aad []byte) ([]byte, error) {
	aead, err := newAEAD(master, native)
	if err != nil {
		return nil, errSecret
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(payload) < aead.NonceSize()+aead.Overhead()+1 {
		return nil, errSecret
	}
	data, err := aead.Open(nil, payload[:aead.NonceSize()], payload[aead.NonceSize():], aad)
	if err != nil {
		return nil, errSecret
	}
	return data, nil
}
func encryptNative(keys keyrings, s secretSubject, data []byte) (string, error) {
	if !keyIDPattern.MatchString(keys.Active) || len(data) == 0 || len(data) > 256*1024 {
		return "", errSecret
	}
	aead, err := newAEAD(keys.Native[keys.Active], true)
	if err != nil {
		return "", errSecret
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", errSecret
	}
	payload := aead.Seal(nonce, nonce, data, nativeAAD(s, keys.Active))
	return "c1." + base64.RawURLEncoding.EncodeToString([]byte(keys.Active)) + "." + base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeContinuation(data []byte) ([]byte, error) {
	var direct map[string]json.RawMessage
	if json.Unmarshal(data, &direct) != nil {
		return nil, errSecret
	}
	if _, ok := direct["accepted_operation"]; ok {
		return append([]byte{}, data...), nil
	}
	var chunks map[string]string
	if json.Unmarshal(data, &chunks) != nil || len(chunks) == 0 {
		return nil, errSecret
	}
	names := make([]string, 0, len(chunks))
	for k := range chunks {
		names = append(names, k)
	}
	sort.Strings(names)
	var out strings.Builder
	for _, name := range names {
		value := chunks[name]
		if !strings.HasPrefix(value, "v2:") {
			return nil, errSecret
		}
		out.WriteString(strings.TrimPrefix(value, "v2:"))
	}
	result := []byte(out.String())
	if len(result) > 256*1024 || json.Unmarshal(result, &direct) != nil || direct["accepted_operation"] == nil {
		return nil, errSecret
	}
	return result, nil
}
