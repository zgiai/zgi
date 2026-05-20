package util

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"golang.org/x/crypto/pbkdf2"
)

const pbkdf2Iterations = 10000
const pbkdf2SaltLen = 16

// HashPasswordPBKDF2 generate encrypted password and salt (base64)
func HashPasswordPBKDF2(password string) (string, string, error) {
	salt := make([]byte, pbkdf2SaltLen)
	_, err := rand.Read(salt)
	if err != nil {
		return "", "", err
	}
	dk := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, sha256.Size, sha256.New)
	hexDk := make([]byte, hex.EncodedLen(len(dk)))
	hex.Encode(hexDk, dk)
	passwordBase64 := base64.StdEncoding.EncodeToString(hexDk)
	saltBase64 := base64.StdEncoding.EncodeToString(salt)
	return passwordBase64, saltBase64, nil
}

// ComparePasswordPBKDF2 validate password
func ComparePasswordPBKDF2(password, passwordBase64, saltBase64 string) (bool, error) {
	salt, err := base64.StdEncoding.DecodeString(saltBase64)
	if err != nil {
		return false, err
	}
	dk := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, sha256.Size, sha256.New)
	hexDk := make([]byte, hex.EncodedLen(len(dk)))
	hex.Encode(hexDk, dk)
	newPasswordBase64 := base64.StdEncoding.EncodeToString(hexDk)
	return newPasswordBase64 == passwordBase64, nil
}
