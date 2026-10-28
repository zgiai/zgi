package v1

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"github.com/zgiai/zgi/api/internal/util"
)

type connectorRuntimeConfirmationContinuerSpy struct {
	results []connectorconfirmation.Result
}

func (spy *connectorRuntimeConfirmationContinuerSpy) ResumeAfterConnectorConfirmation(
	_ context.Context,
	_ connectorconfirmation.Command,
	result connectorconfirmation.Result,
) error {
	spy.results = append(spy.results, result)
	return nil
}

func newConnectorRuntimeConfirmationContinuationTestRouter(
	service *connectorRuntimeConfirmationServiceSpy,
	continuer ConnectorRuntimeConfirmationContinuer,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")
	organizationID, ownerID := uuid.New(), uuid.New()
	group.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, organizationID.String())
		c.Set("account_id", ownerID.String())
		c.Next()
	})
	(&connectorRuntimeConfirmationHandler{service: service, recorder: service, digitalHuman: continuer}).register(group)
	return router
}

func TestConnectorRuntimeConfirmationRouteContinuesDigitalHumanTurnWithResult(t *testing.T) {
	result := connectorconfirmation.Result{
		State: connectorconfirmation.StateCompleted, DecisionID: uuid.New(),
		Origin: connectorexecution.ConfirmationOrigin{Surface: connectorcore.TargetTypeDigitalHuman},
		ExecutionResult: &connectorexecution.ExecutionResult{
			State: connectorexecution.ExecutionStateCompleted,
			Value: map[string]any{"active_state": "active"},
		},
	}
	service := &connectorRuntimeConfirmationServiceSpy{result: result}
	continuer := &connectorRuntimeConfirmationContinuerSpy{}
	router := newConnectorRuntimeConfirmationContinuationTestRouter(service, continuer)

	response := performConnectorRuntimeConfirmationRequest(
		t, router, `{"route_receipt":"`+uuid.NewString()+`","decision":"confirmed"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(service.recordCalls) != 0 {
		t.Fatalf("a DigitalHuman decision was only recorded: %#v", service.recordCalls)
	}
	if len(continuer.results) != 1 || continuer.results[0].ExecutionResult == nil {
		t.Fatalf("continuation did not receive the operation result: %#v", continuer.results)
	}
}

func TestConnectorRuntimeConfirmationRouteRecordsOtherHostsWithoutResult(t *testing.T) {
	result := connectorconfirmation.Result{
		State: connectorconfirmation.StateCompleted, DecisionID: uuid.New(),
		Origin: connectorexecution.ConfirmationOrigin{Surface: connectorcore.TargetTypeAgent},
		ExecutionResult: &connectorexecution.ExecutionResult{
			State: connectorexecution.ExecutionStateCompleted,
			Value: map[string]any{"provider_value": connectorRuntimeConfirmationRouteSecret},
		},
	}
	service := &connectorRuntimeConfirmationServiceSpy{result: result}
	continuer := &connectorRuntimeConfirmationContinuerSpy{}
	router := newConnectorRuntimeConfirmationContinuationTestRouter(service, continuer)

	response := performConnectorRuntimeConfirmationRequest(
		t, router, `{"route_receipt":"`+uuid.NewString()+`","decision":"confirmed"}`,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(continuer.results) != 0 {
		t.Fatal("an Agent decision reached the DigitalHuman continuation")
	}
	if len(service.recordCalls) != 1 || service.recordCalls[0].result.ExecutionResult != nil {
		t.Fatalf("resolution recorder calls = %#v", service.recordCalls)
	}
}
