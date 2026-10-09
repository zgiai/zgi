package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

func TestNativeChannelFileDeliveryAndPartialFailure(t *testing.T) {
	for _, mode := range []string{"success", "upload rejected", "partial send"} {
		t.Run(mode, func(t *testing.T) {
			var uploads int
			var messages []channelMessageBody
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/open-apis/auth/v3/tenant_access_token/internal":
					_, _ = io.WriteString(w, `{"code":0,"tenant_access_token":"tenant-token","expire":7200}`)
				case "/open-apis/im/v1/files":
					uploads++
					if r.Header.Get("Authorization") != "Bearer tenant-token" {
						t.Error("missing tenant authorization")
					}
					if err := r.ParseMultipartForm(1024); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					if r.FormValue("file_type") != "stream" || r.FormValue("file_name") != "result.txt" {
						t.Error("invalid upload metadata")
					}
					file, _, err := r.FormFile("file")
					if err != nil {
						t.Error(err)
						return
					}
					data, _ := io.ReadAll(file)
					_ = file.Close()
					if string(data) != "result" {
						t.Errorf("file=%q", data)
					}
					if mode == "upload rejected" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					_, _ = io.WriteString(w, `{"code":0,"data":{"file_key":"file_test_key"}}`)
				case "/open-apis/im/v1/messages":
					var body channelMessageBody
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					messages = append(messages, body)
					if body.MsgType == "file" && mode == "partial send" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					_, _ = io.WriteString(w, `{"code":0,"data":{"message_id":"om_result"}}`)
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			request := markdownDeliveryRequest()
			request.Attachments = []channelruntime.DeliveryAttachment{{ID: "result", Filename: "result.txt", Size: 6,
				Open: func(context.Context) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("result")), nil }}}
			adapter, err := NewDirectChannelAdapter(newTestAdapter(t, server), &channelConnectionResolverStub{connection: directChannelConnection(request.ConnectionID)})
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := adapter.Deliver(t.Context(), request)
			server.Close()
			if uploads != 1 {
				t.Fatalf("uploads=%d", uploads)
			}
			switch mode {
			case "success":
				if err != nil || receipt == nil || len(messages) != 2 || messages[1].MsgType != "file" || messages[1].Content != `{"file_key":"file_test_key"}` || messages[0].UUID == messages[1].UUID {
					t.Fatalf("receipt=%v error=%v messages=%+v", receipt, err, messages)
				}
			case "upload rejected":
				if err == nil || errors.Is(err, channelruntime.ErrDeliveryOutcomeUnknown) || len(messages) != 0 {
					t.Fatalf("upload failure sent messages: %v %+v", err, messages)
				}
			case "partial send":
				if !errors.Is(err, channelruntime.ErrDeliveryOutcomeUnknown) || len(messages) != 2 {
					t.Fatalf("partial delivery retried or downgraded: %v %+v", err, messages)
				}
			}
		})
	}
}

func TestNativeChannelFileLimitRejectsBeforeReading(t *testing.T) {
	adapter := &ChannelAdapter{}
	_, err := adapter.deliverWithAttachments(t.Context(), "", "", channelruntime.DeliveryRequest{ExternalConversationID: "oc_test", Attachments: []channelruntime.DeliveryAttachment{{ID: "file", Filename: "large.pdf", Size: maxChannelFileBytes + 1, Open: func(context.Context) (io.ReadCloser, error) { t.Fatal("oversized file read"); return nil, nil }}}})
	if err == nil {
		t.Fatal("oversized file accepted")
	}
}
