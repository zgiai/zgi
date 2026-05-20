package workflow

import (
	"time"

	"github.com/zgiai/zgi/api/internal/dto"

	"github.com/gin-gonic/gin"
)

// WorkflowStatisticHandler handles workflow statistics requests
type WorkflowStatisticHandler struct {
	workflowService WorkflowService
}

// NewWorkflowStatisticHandler creates a new workflow statistic handler
func NewWorkflowStatisticHandler(workflowService WorkflowService) *WorkflowStatisticHandler {
	return &WorkflowStatisticHandler{
		workflowService: workflowService,
	}
}

// GetWorkflowDailyRuns gets workflow daily runs statistics
func (sh *WorkflowStatisticHandler) GetWorkflowDailyRuns(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	appID := c.Param("agent_id")

	var req dto.WorkflowStatisticRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Get timezone from user account
	timezone := "UTC" // Default timezone, should get from user account
	if userTimezone := c.GetString("timezone"); userTimezone != "" {
		timezone = userTimezone
	}

	// Convert timezone format if needed
	if req.Start != nil {
		req.Start = convertToUTCWithTimezone(*req.Start, timezone)
	}
	if req.End != nil {
		req.End = convertToUTCWithTimezone(*req.End, timezone)
	}

	result, err := sh.workflowService.GetWorkflowDailyRuns(c.Request.Context(), tenantID, appID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, result)
}

// GetWorkflowDailyTerminals gets workflow daily terminals statistics
func (sh *WorkflowStatisticHandler) GetWorkflowDailyTerminals(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	appID := c.Param("agent_id")

	var req dto.WorkflowStatisticRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Get timezone from user account
	timezone := "UTC" // Default timezone, should get from user account
	if userTimezone := c.GetString("timezone"); userTimezone != "" {
		timezone = userTimezone
	}

	// Convert timezone format if needed
	if req.Start != nil {
		req.Start = convertToUTCWithTimezone(*req.Start, timezone)
	}
	if req.End != nil {
		req.End = convertToUTCWithTimezone(*req.End, timezone)
	}

	result, err := sh.workflowService.GetWorkflowDailyTerminals(c.Request.Context(), tenantID, appID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, result)
}

// GetWorkflowDailyTokenCost gets workflow daily token cost statistics
func (sh *WorkflowStatisticHandler) GetWorkflowDailyTokenCost(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	appID := c.Param("agent_id")

	var req dto.WorkflowStatisticRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Get timezone from user account
	timezone := "UTC" // Default timezone, should get from user account
	if userTimezone := c.GetString("timezone"); userTimezone != "" {
		timezone = userTimezone
	}

	// Convert timezone format if needed
	if req.Start != nil {
		req.Start = convertToUTCWithTimezone(*req.Start, timezone)
	}
	if req.End != nil {
		req.End = convertToUTCWithTimezone(*req.End, timezone)
	}

	result, err := sh.workflowService.GetWorkflowDailyTokenCost(c.Request.Context(), tenantID, appID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, result)
}

// GetWorkflowAverageAppInteraction gets workflow average app interaction statistics
func (sh *WorkflowStatisticHandler) GetWorkflowAverageAppInteraction(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	appID := c.Param("agent_id")

	var req dto.WorkflowStatisticRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Get timezone from user account
	timezone := "UTC" // Default timezone, should get from user account
	if userTimezone := c.GetString("timezone"); userTimezone != "" {
		timezone = userTimezone
	}

	// Convert timezone format if needed
	if req.Start != nil {
		req.Start = convertToUTCWithTimezone(*req.Start, timezone)
	}
	if req.End != nil {
		req.End = convertToUTCWithTimezone(*req.End, timezone)
	}

	result, err := sh.workflowService.GetWorkflowAverageAppInteraction(c.Request.Context(), tenantID, appID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, result)
}

// convertToUTCWithTimezone converts time with timezone consideration
// This is a simplified version, in production you'd want proper timezone handling
func convertToUTCWithTimezone(t time.Time, timezone string) *time.Time {
	// Load the timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		// Default to UTC if timezone loading fails
		loc = time.UTC
	}

	// Convert the time to the specified timezone, then to UTC
	localTime := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
	utcTime := localTime.UTC()

	return &utcTime
}
