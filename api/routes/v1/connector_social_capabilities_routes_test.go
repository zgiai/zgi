package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/socialevents"
	"github.com/zgiai/zgi/api/internal/util"
)

type socialEventsRouteStub struct {
	org, owner, conn uuid.UUID
	after            int64
	limit            int
	acked            int64
	signature        string
	payload          string
}

func (s *socialEventsRouteStub) VerifyChallenge(mode, token, challenge string) (string, error) {
	return challenge, nil
}
func (s *socialEventsRouteStub) ReceiveFacebook(_ context.Context, signature string, raw []byte) error {
	s.signature = signature
	s.payload = string(raw)
	return nil
}
func (s *socialEventsRouteStub) List(_ context.Context, o, a, c uuid.UUID, after int64, limit int) (socialevents.Page, error) {
	s.org = o
	s.owner = a
	s.conn = c
	s.after = after
	s.limit = limit
	return socialevents.Page{}, nil
}
func (s *socialEventsRouteStub) Acknowledge(_ context.Context, o, a, c uuid.UUID, through int64) error {
	s.org = o
	s.owner = a
	s.conn = c
	s.acked = through
	return nil
}
func socialCapabilitiesRouter(stub *socialEventsRouteStub, org, owner uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &connectorHandler{socialEvents: stub}
	h.registerSocialWebhook(r.Group("/console/api"))
	g := r.Group("/console/api")
	g.Use(func(c *gin.Context) { util.SetOrganizationID(c, org.String()); c.Set("account_id", owner.String()) })
	h.registerSocialCapabilities(g)
	return r
}
func TestSocialEventRoutesKeepOwnerAndCursor(t *testing.T) {
	org, owner, conn := uuid.New(), uuid.New(), uuid.New()
	stub := &socialEventsRouteStub{}
	r := socialCapabilitiesRouter(stub, org, owner)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/console/api/connector-connections/"+conn.String()+"/social-events?after_id=12&limit=7", nil))
	if rec.Code != 200 || stub.org != org || stub.owner != owner || stub.conn != conn || stub.after != 12 || stub.limit != 7 {
		t.Fatal("event list lost scope/cursor", rec.Code, rec.Body.String())
	}
	for _, query := range []string{"after_id=-1", "limit=101", "limit=0", "after_id=invalid"} {
		rec = httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", "/console/api/connector-connections/"+conn.String()+"/social-events?"+query, nil))
		if rec.Code < 400 {
			t.Fatal("unbounded/invalid list accepted", query)
		}
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/console/api/connector-connections/"+conn.String()+"/social-events/ack", strings.NewReader(`{"through_id":42}`)))
	if rec.Code != 200 || stub.acked != 42 || stub.owner != owner {
		t.Fatal("ack lost actor")
	}
}
func TestSocialWebhookRoutesPublicBoundedAndPlainChallenge(t *testing.T) {
	stub := &socialEventsRouteStub{}
	r := socialCapabilitiesRouter(stub, uuid.Nil, uuid.Nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/console/api/connectors/social-webhooks/facebook?hub.mode=subscribe&hub.verify_token=secret&hub.challenge=123456", nil))
	if rec.Code != 200 || rec.Body.String() != "123456" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("challenge response", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest("POST", "/console/api/connectors/social-webhooks/facebook", strings.NewReader(`{"object":"page","entry":[]}`))
	req.Header.Set("X-Hub-Signature-256", "sha256=fixture")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 || stub.signature != "sha256=fixture" || stub.payload == "" {
		t.Fatal("signed body not dispatched")
	}
	stub.payload = ""
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/console/api/connectors/social-webhooks/facebook", strings.NewReader(strings.Repeat("x", (1<<20)+1))))
	if rec.Code < 400 || stub.payload != "" {
		t.Fatal("oversized webhook dispatched")
	}
}
func TestSocialImagePreparationRequiresActorAndRealImage(t *testing.T) {
	r := socialCapabilitiesRouter(&socialEventsRouteStub{}, uuid.Nil, uuid.Nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/console/api/connectors/social-media/prepare-image", strings.NewReader("invalid")))
	if rec.Code < 400 {
		t.Fatal("anonymous image processing accepted")
	}
	r = socialCapabilitiesRouter(&socialEventsRouteStub{}, uuid.New(), uuid.New())
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("POST", "/console/api/connectors/social-media/prepare-image", strings.NewReader("invalid")))
	if rec.Code < 400 {
		t.Fatal("corrupt image accepted")
	}
}

func TestSocialImagePreparationReturnsBoundedNativeInput(t *testing.T) {
	r := socialCapabilitiesRouter(&socialEventsRouteStub{}, uuid.New(), uuid.New())
	img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	img.SetNRGBA(5, 6, color.NRGBA{R: 255, A: 255})
	var body bytes.Buffer
	if err := png.Encode(&body, img); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/console/api/connectors/social-media/prepare-image", &body)
	req.Header.Set("Content-Type", "application/octet-stream")
	r.ServeHTTP(rec, req)
	var result struct {
		Data struct {
			ImageData string `json:"image_data"`
			MIMEType  string `json:"mime_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			Bytes     int    `json:"bytes"`
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || result.Data.ImageData == "" || len(result.Data.ImageData) > 196608 || result.Data.MIMEType != "image/png" || result.Data.Width != 32 || result.Data.Height != 16 || result.Data.Bytes > 147456 {
		t.Fatal("invalid prepared image response", rec.Code)
	}
}
