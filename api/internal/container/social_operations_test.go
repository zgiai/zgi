package container

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	socialtools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/socialoperations"
	"gorm.io/gorm"
)

func TestSocialOperationsLocalServiceIsSharedAndNotPublicOpenDALStorage(t *testing.T) {
	base := filepath.Join(t.TempDir(), "opendal")
	c := &ServiceContainer{db: &gorm.DB{}, config: &config.Config{Storage: config.StorageConfig{OpenDALBasePath: base}}}
	var wg sync.WaitGroup
	services := make(chan *socialops.Service, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := c.GetSocialOperationsService()
			if err != nil {
				t.Error(err)
				return
			}
			services <- s
		}()
	}
	wg.Wait()
	close(services)
	var first *socialops.Service
	for service := range services {
		if first == nil {
			first = service
		} else if first != service {
			t.Fatal("separate journal services")
		}
	}
	if first == nil {
		t.Fatal("missing service")
	}
	defer first.Close()
	if _, err := os.Stat(filepath.Join(filepath.Dir(base), "social-operations")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatal("journal created public OpenDAL root", err)
	}
}
func TestSocialOperationsRuntimeRejectsUnverifiedCallers(t *testing.T) {
	resolver := &socialOperationsRuntime{container: &ServiceContainer{}}
	for _, identity := range []socialtools.InvocationIdentity{{}, {Runtime: &tools.ToolRuntime{TenantID: "fake", InvokeFrom: tools.ToolInvokeFromDigitalHuman}}, {Runtime: &tools.ToolRuntime{InvokeFrom: tools.ToolInvokeFromAIChat}}} {
		if _, _, err := resolver.ResolveSocialOperations(context.Background(), identity); err == nil {
			t.Fatal("unverified caller accepted")
		}
	}
}
