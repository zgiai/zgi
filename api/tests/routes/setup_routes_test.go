package routes_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	v1 "github.com/zgiai/zgi/api/routes/v1"
)

func TestRegisterSetupPathsSelfHostedExposesSetup(t *testing.T) {
	setSetupRouteTestEdition(t, "SELF_HOSTED")
	gin.SetMode(gin.TestMode)

	router := gin.New()
	group := router.Group("")
	v1.RegisterSetupPaths(
		group,
		func(c *gin.Context) { c.Status(http.StatusOK) },
		func(c *gin.Context) { c.Status(http.StatusCreated) },
		func(c *gin.Context) { c.Status(http.StatusAccepted) },
	)

	getSetup := httptest.NewRecorder()
	router.ServeHTTP(getSetup, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if getSetup.Code != http.StatusOK {
		t.Fatalf("GET /setup status = %d, want %d", getSetup.Code, http.StatusOK)
	}

	postSetup := httptest.NewRecorder()
	router.ServeHTTP(postSetup, httptest.NewRequest(http.MethodPost, "/setup", nil))
	if postSetup.Code != http.StatusCreated {
		t.Fatalf("POST /setup status = %d, want %d", postSetup.Code, http.StatusCreated)
	}

	systemFeatures := httptest.NewRecorder()
	router.ServeHTTP(systemFeatures, httptest.NewRequest(http.MethodGet, "/system-features", nil))
	if systemFeatures.Code != http.StatusAccepted {
		t.Fatalf("GET /system-features status = %d, want %d", systemFeatures.Code, http.StatusAccepted)
	}
}

func TestRegisterSetupPathsCloudHidesSetup(t *testing.T) {
	setSetupRouteTestEdition(t, "CLOUD")
	gin.SetMode(gin.TestMode)

	router := gin.New()
	group := router.Group("")
	v1.RegisterSetupPaths(
		group,
		func(c *gin.Context) { c.Status(http.StatusOK) },
		func(c *gin.Context) { c.Status(http.StatusCreated) },
		func(c *gin.Context) { c.Status(http.StatusAccepted) },
	)

	getSetup := httptest.NewRecorder()
	router.ServeHTTP(getSetup, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if getSetup.Code != http.StatusNotFound {
		t.Fatalf("GET /setup status = %d, want %d", getSetup.Code, http.StatusNotFound)
	}

	postSetup := httptest.NewRecorder()
	router.ServeHTTP(postSetup, httptest.NewRequest(http.MethodPost, "/setup", nil))
	if postSetup.Code != http.StatusNotFound {
		t.Fatalf("POST /setup status = %d, want %d", postSetup.Code, http.StatusNotFound)
	}

	systemFeatures := httptest.NewRecorder()
	router.ServeHTTP(systemFeatures, httptest.NewRequest(http.MethodGet, "/system-features", nil))
	if systemFeatures.Code != http.StatusAccepted {
		t.Fatalf("GET /system-features status = %d, want %d", systemFeatures.Code, http.StatusAccepted)
	}
}

func setSetupRouteTestEdition(t *testing.T, edition string) {
	t.Helper()

	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Platform: config.PlatformConfig{
			Edition: edition,
		},
	}

	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
}
