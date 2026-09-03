package connectorfinalize

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

const archiveAAD = "zgi/connector-retirement/archive/v2"

type archivePayload struct {
	Report Report   `json:"report"`
	Data   snapshot `json:"data"`
}

// CreateArchive captures the exact reviewed inventory. Its independent random
// 32-byte recovery key must be kept outside the repository, beside the backup.
// No live credential key is repurposed and no plaintext archive is written.
func CreateArchive(ctx context.Context, db *gorm.DB, expected string, key []byte, option Options) ([]byte, error) {
	var payload archivePayload
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, data, err := inspect(tx, option)
		if err != nil {
			return err
		}
		if r.Digest != expected || len(r.Issues) != 0 {
			return fmt.Errorf("archive inventory differs from the reviewed clean report")
		}
		payload = archivePayload{Report: r, Data: data}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	return encryptArchive(payload, key)
}

func archiveCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("archive requires an independent 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func encryptArchive(payload archivePayload, key []byte) ([]byte, error) {
	aead, err := archiveCipher(key)
	if err != nil {
		return nil, err
	}
	plain, err := json.Marshal(payload)
	defer clear(plain)
	if err != nil || len(plain) > 2*maxSnapshotBytes {
		return nil, fmt.Errorf("archive payload exceeds bounded inventory")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plain, []byte(archiveAAD)), nil
}

func verifyArchive(encrypted, key []byte, report Report, data snapshot) error {
	aead, err := archiveCipher(key)
	if err != nil {
		return err
	}
	if len(encrypted) < aead.NonceSize()+aead.Overhead() || len(encrypted) > 2*maxSnapshotBytes+aead.NonceSize()+aead.Overhead() {
		return fmt.Errorf("invalid retirement archive size")
	}
	plain, err := aead.Open(nil, encrypted[:aead.NonceSize()], encrypted[aead.NonceSize():], []byte(archiveAAD))
	defer clear(plain)
	if err != nil {
		return fmt.Errorf("retirement archive cannot be authenticated")
	}
	var archived archivePayload
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if decoder.Decode(&archived) != nil || digest(archived.Report) != digest(report) || digest(archived.Data) != digest(data) {
		return fmt.Errorf("retirement archive does not cover the current reviewed inventory")
	}
	return nil
}

func archiveDigest(encrypted []byte) string {
	hash := sha256.Sum256(encrypted)
	return hex.EncodeToString(hash[:])
}
