package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

type routeSelectionRepo struct {
	channelrepo.TenantRouteRepository
	routes []*channelmodel.LLMRoute
}

func (f *routeSelectionRepo) GetEnabledRoutes(context.Context, uuid.UUID) ([]*channelmodel.LLMRoute, error) {
	return f.routes, nil
}

func TestNameOnlyRouteSelectionRejectsAmbiguousOfficialProviders(t *testing.T) {
	organizationID := uuid.New()
	modelName := "same-name"
	svc := &channelService{tenantRouteRepo: &routeSelectionRepo{routes: []*channelmodel.LLMRoute{{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		Type:           shared.RouteTypeZGICloud,
		Models:         []string{modelName},
		OfficialProviderModels: []channelmodel.ProviderModel{
			{Provider: "openai", Model: modelName},
			{Provider: "anthropic", Model: modelName},
		},
		IsOfficial: true,
	}}}}

	t.Run("GetRoutesForModel", func(t *testing.T) {
		routes, err := svc.GetRoutesForModel(context.Background(), organizationID, modelName)
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("GetRoutesForModel() = (%#v, %v), want ambiguity error", routes, err)
		}
		if routes != nil {
			t.Fatalf("routes = %#v, want nil", routes)
		}
	})

	t.Run("SelectRoute", func(t *testing.T) {
		route, err := svc.SelectRoute(context.Background(), organizationID, modelName)
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("SelectRoute() = (%#v, %v), want ambiguity error", route, err)
		}
		if route != nil {
			t.Fatalf("route = %#v, want nil", route)
		}
	})
}

func TestGetRoutesForModelUsesUniqueOfficialProviderPair(t *testing.T) {
	organizationID := uuid.New()
	modelName := "same-name"
	routeID := uuid.New()
	svc := &channelService{tenantRouteRepo: &routeSelectionRepo{routes: []*channelmodel.LLMRoute{{
		ID:                     routeID,
		OrganizationID:         organizationID,
		Type:                   shared.RouteTypeZGICloud,
		Models:                 []string{"stale-flat-name"},
		OfficialProviderModels: []channelmodel.ProviderModel{{Provider: "openai", Model: modelName}},
		IsOfficial:             true,
	}}}}

	routes, err := svc.GetRoutesForModel(context.Background(), organizationID, modelName)
	if err != nil {
		t.Fatalf("GetRoutesForModel() error = %v", err)
	}
	if len(routes) != 1 || routes[0].RouteID != routeID {
		t.Fatalf("routes = %#v, want only exact provider/model route %s", routes, routeID)
	}
}

func TestGetRoutesForModelPreservesPrivateNameMatching(t *testing.T) {
	organizationID := uuid.New()
	modelName := "same-name"
	routeID := uuid.New()
	svc := &channelService{tenantRouteRepo: &routeSelectionRepo{routes: []*channelmodel.LLMRoute{{
		ID:             routeID,
		OrganizationID: organizationID,
		Type:           shared.RouteTypePrivate,
		Models:         []string{modelName},
	}}}}

	routes, err := svc.GetRoutesForModel(context.Background(), organizationID, modelName)
	if err != nil {
		t.Fatalf("GetRoutesForModel() error = %v", err)
	}
	if len(routes) != 1 || routes[0].RouteID != routeID {
		t.Fatalf("routes = %#v, want private route %s", routes, routeID)
	}
}
