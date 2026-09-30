package workspacecheckpoint_test

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/runtimeagent"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

const checkpointKey = "95fc392d1c9e35e57b4452fd463cd770e735fa2113592a7e708492d053bb7cc4"

func TestCheckpointRoundTripAcrossRuntimeAndStoreRestart(t *testing.T) {
	checkpointDirectory := t.TempDir()
	firstWorkspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(firstWorkspace, "reports"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(firstWorkspace, "reports", "result.csv"), []byte("name,total\nA,3\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	firstAgent, err := runtimeagent.New(runtimeagent.Options{WorkspaceRoot: firstWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	firstHandler, err := runtimeagent.NewHTTPServer(firstAgent, runtimeagent.HTTPServerOptions{Token: "checkpoint-test"})
	if err != nil {
		t.Fatal(err)
	}
	firstServer := httptest.NewServer(firstHandler)
	firstRemote, err := runtimeagent.NewRemote(runtimeagent.RemoteOptions{BaseURL: firstServer.URL, Token: "checkpoint-test"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := workspacecheckpoint.NewFileStore(checkpointDirectory)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := firstRemote.ExportWorkspace(context.Background(), agentexecution.Allocation{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), checkpointKey, stream); err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	_ = store.Close()
	firstServer.Close()
	_ = firstHandler.Close()
	_ = firstAgent.Close()

	// Both the execution environment and the checkpoint store are reopened.
	store, err = workspacecheckpoint.NewFileStore(checkpointDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secondWorkspace := t.TempDir()
	secondAgent, err := runtimeagent.New(runtimeagent.Options{WorkspaceRoot: secondWorkspace})
	if err != nil {
		t.Fatal(err)
	}
	defer secondAgent.Close()
	secondHandler, err := runtimeagent.NewHTTPServer(secondAgent, runtimeagent.HTTPServerOptions{Token: "checkpoint-test"})
	if err != nil {
		t.Fatal(err)
	}
	defer secondHandler.Close()
	secondServer := httptest.NewServer(secondHandler)
	defer secondServer.Close()
	secondRemote, err := runtimeagent.NewRemote(runtimeagent.RemoteOptions{BaseURL: secondServer.URL, Token: "checkpoint-test"})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := store.Open(context.Background(), checkpointKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondRemote.ImportWorkspace(context.Background(), agentexecution.Allocation{}, archive); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(secondWorkspace, "reports", "result.csv"))
	if err != nil || string(content) != "name,total\nA,3\n" {
		t.Fatalf("restored content = %q, %v", content, err)
	}
	reader, info, err := secondRemote.DownloadFile(context.Background(), agentexecution.Allocation{}, "reports/result.csv")
	if err != nil {
		t.Fatal(err)
	}
	downloaded, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || info.Size != int64(len(downloaded)) || string(downloaded) != string(content) {
		t.Fatalf("downloaded content = %q, info = %+v, %v", downloaded, info, err)
	}
}
