package tool_file

import (
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/response"
)

// ServeExecutionFile is called only after the entry has authenticated the user
// and checked current application/conversation permissions. Access is server
// generated, never decoded from request JSON or execution identity headers.
func (h *HTTPHandler) ServeExecutionFile(c *gin.Context, access ExecutionFileAccess) {
	if h.manager == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	data, file, err := h.manager.GetExecutionFileBinary(c.Request.Context(), access, c.Param("file_id"))
	if err != nil {
		response.Fail(c, response.ErrFileNotFound)
		return
	}
	WriteExecutionFile(c, file, data)
}

// WriteExecutionFile writes bytes already authorized by GetExecutionFileBinary.
// DigitalHuman uses this after projecting errors through its own HTTP contract.
func WriteExecutionFile(c *gin.Context, file *ToolFile, data []byte) {
	// Never redirect to a public storage URL. Reject active-document execution
	// on the application origin, and do not allow shared/browser caches to retain
	// a response after logout or permission revocation.
	c.Header("Cache-Control", "private, no-store")
	c.Header("Vary", "Authorization, Cookie")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
	c.Header("Content-Type", toolFileResponseContentType(file.MimeType))
	if shouldDownloadToolFile(c.Query("download")) {
		c.Header("Content-Disposition", toolFileContentDisposition(file.Name))
	}
	http.ServeContent(c.Writer, c.Request, file.Name, file.CreatedAt, bytes.NewReader(data))
}
