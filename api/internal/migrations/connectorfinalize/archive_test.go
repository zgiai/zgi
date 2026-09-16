package connectorfinalize

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestRetirementArchiveBindsCompleteInventory(t *testing.T) {
	key := bytes.Repeat([]byte{17}, 32)
	data := snapshot{"integration_connections": {{"id": "source", "revision": json.Number("9007199254740993"), "encrypted_credentials": "private-test-envelope"}}}
	r := Report{Version: ReportVersion, DatabaseFingerprint: digest("database")}
	r.seal()
	encrypted, err := encryptArchive(archivePayload{Report: r, Data: data}, key)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("private-test-envelope")) {
		t.Fatal("archive contains plaintext")
	}
	if err := verifyArchive(encrypted, key, r, data); err != nil {
		t.Fatal(err)
	}
	changed := Report{Version: ReportVersion, DatabaseFingerprint: digest("another database")}
	changed.seal()
	if verifyArchive(encrypted, key, changed, data) == nil {
		t.Fatal("wrong database accepted")
	}
	data["integration_connections"][0]["revision"] = json.Number("9007199254740992")
	if verifyArchive(encrypted, key, r, data) == nil {
		t.Fatal("changed source accepted")
	}
	encrypted[len(encrypted)-1] ^= 1
	if verifyArchive(encrypted, key, r, data) == nil {
		t.Fatal("corrupt archive accepted")
	}
}
