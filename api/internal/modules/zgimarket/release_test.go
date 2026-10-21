package zgimarket

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deleteEngine serves the two delete routes behind the release middleware. The handlers stand in
// for the platform's: they soft-delete the subject, or refuse when asked to.
func deleteEngine(fixture *toolsFixture, refuse bool) *gin.Engine {
	engine := gin.New()
	engine.Use(ReleaseOnDelete(fixture.tools))
	engine.Use(func(c *gin.Context) {
		c.Set("organization_id", fixture.org.String())
		c.Set("account_id", "5b0c2f0e-3c1d-4a51-9a52-0f4f4a6f6f01")
	})
	remove := func(table, param string) gin.HandlerFunc {
		return func(c *gin.Context) {
			if refuse {
				c.JSON(http.StatusConflict, gin.H{"code": "blocked"})
				return
			}
			fixture.db.Exec("UPDATE "+table+" SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?", c.Param(param))
			c.JSON(http.StatusOK, gin.H{"code": "0"})
		}
	}
	engine.DELETE("/console/api/agents/:agent_id", remove("agents", "agent_id"))
	engine.DELETE("/console/api/digital-humans/:id", remove("digital_humans", "id"))
	engine.DELETE("/console/api/agents/:agent_id/api-keys/:key_id", remove("agents", "agent_id"))
	return engine
}

func deleteSubject(engine *gin.Engine, path string) int {
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodDelete, path, nil))
	return recorder.Code
}

func bindings(t *testing.T, fixture *toolsFixture) int64 {
	t.Helper()
	var count int64
	require.NoError(t, fixture.db.Model(&ToolBinding{}).Count(&count).Error)
	return count
}

func TestDeletingASubjectReleasesItsMarketBinding(t *testing.T) {
	fixture := newToolsFixture(t, true)
	status, answer := fixture.send(t, http.MethodPut, "tools/subjects/agent/"+fixture.agent.String(), `{"items":[{"listing_id":7,"connection_id":5}]}`)
	require.Equal(t, http.StatusOK, status, answer)
	status, answer = fixture.send(t, http.MethodPut, "tools/subjects/digital_human/"+fixture.human.String(), `{"items":[{"listing_id":7,"connection_id":5}]}`)
	require.Equal(t, http.StatusOK, status, answer)
	require.EqualValues(t, 2, bindings(t, fixture))

	// A delete the platform refused, and a delete of something under the agent, release nothing.
	assert.Equal(t, http.StatusConflict, deleteSubject(deleteEngine(fixture, true), "/console/api/agents/"+fixture.agent.String()))
	assert.Equal(t, http.StatusConflict, deleteSubject(deleteEngine(fixture, true), "/console/api/agents/"+fixture.agent.String()+"/api-keys/k1"))
	assert.Empty(t, fixture.market.deleted)
	assert.EqualValues(t, 2, bindings(t, fixture))

	engine := deleteEngine(fixture, false)
	assert.Equal(t, http.StatusOK, deleteSubject(engine, "/console/api/agents/"+fixture.agent.String()))
	assert.Equal(t, []string{"bindings/1"}, fixture.market.deleted, "the market ends the binding and with it the credential")
	assert.EqualValues(t, 1, bindings(t, fixture))
	assert.Equal(t, http.StatusOK, deleteSubject(engine, "/console/api/digital-humans/"+fixture.human.String()))
	assert.Equal(t, []string{"bindings/1", "bindings/2"}, fixture.market.deleted)
	assert.EqualValues(t, 0, bindings(t, fixture))

	// A subject without market items, and one that was never there, cost no market call.
	assert.Equal(t, http.StatusOK, deleteSubject(engine, "/console/api/agents/"+fixture.agent.String()))
	assert.Equal(t, http.StatusOK, deleteSubject(engine, "/console/api/agents/not-a-uuid"))
	assert.Len(t, fixture.market.deleted, 2)
}

func TestAMarketFailureDoesNotBlockTheDeleteAndKeepsTheBindingToEnd(t *testing.T) {
	fixture := newToolsFixture(t, true)
	status, answer := fixture.send(t, http.MethodPut, "tools/subjects/agent/"+fixture.agent.String(), `{"items":[{"listing_id":7,"connection_id":5}]}`)
	require.Equal(t, http.StatusOK, status, answer)

	fixture.market.failDelete = true
	engine := deleteEngine(fixture, false)
	assert.Equal(t, http.StatusOK, deleteSubject(engine, "/console/api/agents/"+fixture.agent.String()), "the agent is deleted all the same")
	assert.Empty(t, fixture.market.deleted)
	assert.EqualValues(t, 1, bindings(t, fixture), "the row still names the market binding that has to be ended")

	// Once the market answers again the same release ends it.
	fixture.market.failDelete = false
	require.NoError(t, fixture.tools.Release(t.Context(), caller{organizationID: fixture.org.String(), admin: true}, SubjectAgent, fixture.agent))
	assert.Equal(t, []string{"bindings/1"}, fixture.market.deleted)
	assert.EqualValues(t, 0, bindings(t, fixture))
}
