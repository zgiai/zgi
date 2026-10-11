package runtimeagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestArchiveResponseRequiresCompleteVerifiedBody(t *testing.T) {
	payload := bytes.Repeat([]byte("workspace archive bytes"), 128)
	digest := sha256.Sum256(payload)
	framed := append(bytes.Clone(payload), []byte(workspaceArchiveFooterMagic)...)
	framed = append(framed, digest[:]...)
	corrupted := bytes.Clone(framed)
	corrupted[0] ^= 0xff

	for _, test := range []struct {
		name    string
		body    []byte
		wantErr bool
	}{
		{name: "complete", body: framed},
		{name: "missing footer", body: payload, wantErr: true},
		{name: "truncated footer", body: framed[:len(framed)-1], wantErr: true},
		{name: "corrupted payload", body: corrupted, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", "application/x-tar")
				_, _ = writer.Write(test.body)
			}))
			defer server.Close()
			remote, err := NewRemote(RemoteOptions{BaseURL: server.URL, Token: "archive-test"})
			if err != nil {
				t.Fatal(err)
			}
			archive, err := remote.ExportWorkspace(context.Background(), agentexecution.Allocation{})
			if err != nil {
				t.Fatal(err)
			}
			got, readErr := io.ReadAll(archive)
			closeErr := archive.Close()
			if closeErr != nil {
				t.Fatal(closeErr)
			}
			if test.wantErr {
				if !errors.Is(readErr, agentexecution.ErrBackendUnavailable) {
					t.Fatalf("read error = %v, want backend unavailable", readErr)
				}
				return
			}
			if readErr != nil || !bytes.Equal(got, payload) {
				t.Fatalf("archive = %q, error = %v", got, readErr)
			}
		})
	}
}
