package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	shortlinkcap "github.com/zgiai/zgi/api/internal/capabilities/shortlink"
)

type fakeShortLinkService struct {
	resolveErr error
}

func (s fakeShortLinkService) CreateOrGet(ctx context.Context, req shortlinkcap.CreateOrGetRequest) (*shortlinkcap.ShortLink, error) {
	_ = ctx
	_ = req
	return nil, nil
}

func (s fakeShortLinkService) Resolve(ctx context.Context, token string) (*shortlinkcap.ShortLink, error) {
	_ = ctx
	_ = token
	if s.resolveErr != nil {
		return nil, s.resolveErr
	}
	return &shortlinkcap.ShortLink{TargetPath: "/a/form-token"}, nil
}

func (s fakeShortLinkService) SyncKnownTargetExpiresAt(ctx context.Context, now time.Time, limit int) (int64, error) {
	_ = ctx
	_ = now
	_ = limit
	return 0, nil
}

func (s fakeShortLinkService) CleanupExpired(ctx context.Context, before time.Time, limit int) (int64, error) {
	_ = ctx
	_ = before
	_ = limit
	return 0, nil
}

func (s fakeShortLinkService) BuildPublicURL(shortToken string) (string, error) {
	_ = shortToken
	return "", nil
}

func TestShortLinkResolveRouteMapsExpiredToNotFound(t *testing.T) {
	previousConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Platform: config.PlatformConfig{Edition: "CLOUD"},
	}
	t.Cleanup(func() {
		config.GlobalConfig = previousConfig
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterShortLinkRoutes(router.Group(""), ShortLinkRouteDeps{
		ShortLinkService: fakeShortLinkService{resolveErr: shortlinkcap.ErrExpired},
	})

	req := httptest.NewRequest(http.MethodGet, "/short-link-resolutions/abc234ef", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
