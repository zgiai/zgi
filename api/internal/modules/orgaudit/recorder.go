package orgaudit

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const pendingContextKey = "orgaudit.pending"

// writeTimeout bounds the audit insert that runs after the response is written.
const writeTimeout = 3 * time.Second

// Writer persists audit records.
type Writer interface {
	Create(ctx context.Context, logs []Log) error
}

// Target is one account or object a change applied to. WorkspaceID and
// Result override the request's own for a batch whose items land in different
// workspaces or succeed and fail separately.
type Target struct {
	AccountID   string
	Type        string
	ID          string
	WorkspaceID string
	Result      Result
}

type pending struct {
	action         Action
	organizationID string
	workspaceID    string
	targets        []Target
	before         map[string]any
	after          map[string]any
	skip           bool
}

// Recorder writes exactly one record per audited change. Routes are wrapped
// with Wrap; the handler annotates the change it made and the wrapper writes
// the record once the response is known, so a request records success,
// denied or failed exactly once.
type Recorder struct {
	writer Writer
	now    func() time.Time
}

// NewRecorder builds a recorder backed by writer.
func NewRecorder(writer Writer) *Recorder {
	if writer == nil {
		panic("org audit recorder requires a writer")
	}
	return &Recorder{writer: writer, now: time.Now}
}

// Wrap audits next as action. A nil recorder leaves next unwrapped.
func (r *Recorder) Wrap(action Action, next gin.HandlerFunc) gin.HandlerFunc {
	if r == nil {
		return next
	}
	return func(c *gin.Context) {
		entry := &pending{action: action}
		c.Set(pendingContextKey, entry)
		next(c)
		r.finish(c, entry)
	}
}

// SetOrganization names the organization when the route does not carry it.
func SetOrganization(c *gin.Context, organizationID string) {
	if entry := pendingFrom(c); entry != nil {
		entry.organizationID = organizationID
	}
}

// SetWorkspace names the workspace the change applied in.
func SetWorkspace(c *gin.Context, workspaceID string) {
	if entry := pendingFrom(c); entry != nil {
		entry.workspaceID = workspaceID
	}
}

// SetTarget replaces the targets with one target.
func SetTarget(c *gin.Context, target Target) {
	if entry := pendingFrom(c); entry != nil {
		entry.targets = []Target{target}
	}
}

// AddTarget adds a target. A change with several targets writes one record
// per target.
func AddTarget(c *gin.Context, target Target) {
	if entry := pendingFrom(c); entry != nil {
		entry.targets = append(entry.targets, target)
	}
}

// ClearTargets drops targets set so far, for example before a batch handler
// adds only the targets that actually changed.
func ClearTargets(c *gin.Context) {
	if entry := pendingFrom(c); entry != nil {
		entry.targets = nil
	}
}

// SetBefore records the state before the change.
func SetBefore(c *gin.Context, fields map[string]any) {
	if entry := pendingFrom(c); entry != nil {
		entry.before = fields
	}
}

// SetAfter records the requested or resulting state.
func SetAfter(c *gin.Context, fields map[string]any) {
	if entry := pendingFrom(c); entry != nil {
		entry.after = fields
	}
}

// Skip writes no record, for a request that changed nothing.
func Skip(c *gin.Context) {
	if entry := pendingFrom(c); entry != nil {
		entry.skip = true
	}
}

func pendingFrom(c *gin.Context) *pending {
	if c == nil {
		return nil
	}
	value, ok := c.Get(pendingContextKey)
	if !ok {
		return nil
	}
	entry, _ := value.(*pending)
	return entry
}

func resultForStatus(status int) (Result, bool) {
	switch {
	case status < http.StatusBadRequest:
		return ResultSuccess, true
	case status == http.StatusUnauthorized:
		// No authenticated actor: nothing traceable to record.
		return "", false
	case status == http.StatusForbidden:
		return ResultDenied, true
	default:
		return ResultFailed, true
	}
}

func (r *Recorder) finish(c *gin.Context, entry *pending) {
	if entry.skip {
		return
	}
	result, ok := resultForStatus(c.Writer.Status())
	if !ok {
		return
	}
	if _, batch := c.Get(batchContextKey); batch && result == ResultSuccess && len(entry.targets) == 0 {
		// A batch that changed nothing has nothing to record.
		return
	}
	logs, ok := r.build(c, entry, result)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), writeTimeout)
	defer cancel()
	if err := r.writer.Create(ctx, logs); err != nil {
		logger.ErrorContext(ctx, "write org audit log failed", "action", string(entry.action), "organization_id", logs[0].OrganizationID.String(), "error", err)
	}
}

const batchContextKey = "orgaudit.batch"

// MarkBatch says the handler adds one target per changed item, so a success
// with no targets changed nothing and writes no record.
func MarkBatch(c *gin.Context) {
	if pendingFrom(c) != nil {
		c.Set(batchContextKey, true)
	}
}

func (r *Recorder) build(c *gin.Context, entry *pending, result Result) ([]Log, bool) {
	routeOrganizationID := c.Param("organization_id")
	if routeOrganizationID == "current" {
		// "current" routes act on the organization in the session context.
		routeOrganizationID = ""
	}
	organizationID, ok := parseUUID(firstNonEmpty(entry.organizationID, routeOrganizationID, util.GetOrganizationID(c)))
	if !ok {
		logger.WarnContext(c.Request.Context(), "org audit record has no organization", "action", string(entry.action))
		return nil, false
	}
	workspaceID := optionalUUID(firstNonEmpty(entry.workspaceID, c.Param("workspace_id")))
	actorID := optionalUUID(util.GetAccountID(c))
	before := encodeFields(entry.before)
	after := encodeFields(entry.after)
	targets := entry.targets
	if len(targets) == 0 {
		targets = []Target{defaultTarget(c)}
	}
	createdAt := r.now().UTC()
	requestID := truncate(c.GetString("request_id"), 128)
	ip := truncate(c.ClientIP(), 64)

	logs := make([]Log, 0, len(targets))
	for _, target := range targets {
		targetWorkspaceID := workspaceID
		if target.WorkspaceID != "" {
			targetWorkspaceID = optionalUUID(target.WorkspaceID)
		}
		targetResult := result
		if result == ResultSuccess && target.Result != "" {
			targetResult = target.Result
		}
		logs = append(logs, Log{
			ID:              uuid.New(),
			OrganizationID:  organizationID,
			WorkspaceID:     targetWorkspaceID,
			ActorAccountID:  actorID,
			TargetAccountID: optionalUUID(target.AccountID),
			TargetType:      truncate(target.Type, 32),
			TargetID:        truncate(target.ID, 64),
			Action:          entry.action,
			Result:          targetResult,
			Before:          before,
			After:           after,
			RequestID:       requestID,
			IP:              ip,
			CreatedAt:       createdAt,
		})
	}
	return logs, true
}

// defaultTarget reads the target from the route when the handler stopped
// before naming it, as a denied or failed request usually does.
func defaultTarget(c *gin.Context) Target {
	if accountID := c.Param("account_id"); accountID != "" {
		return Target{AccountID: accountID, Type: TargetMember, ID: accountID}
	}
	if memberID := c.Param("member_id"); memberID != "" {
		// Organization and workspace member routes name the member by account.
		return Target{AccountID: memberID, Type: TargetMember, ID: memberID}
	}
	if roleID := c.Param("role_id"); roleID != "" {
		return Target{Type: TargetRole, ID: roleID}
	}
	if departmentID := c.Param("dept_id"); departmentID != "" {
		return Target{Type: TargetDepartment, ID: departmentID}
	}
	return Target{}
}

// sensitiveKeyParts names fields that never reach an audit record, whatever a
// handler passes in.
var sensitiveKeyParts = []string{"password", "secret", "token", "credential", "api_key", "apikey", "private_key", "verification_code", "otp"}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(key)
	for _, part := range sensitiveKeyParts {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func sanitize(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		clean := make(map[string]any, len(typed))
		for key, item := range typed {
			if isSensitiveKey(key) {
				continue
			}
			clean[key] = sanitize(item)
		}
		return clean
	case []any:
		clean := make([]any, len(typed))
		for i, item := range typed {
			clean[i] = sanitize(item)
		}
		return clean
	default:
		return value
	}
}

func encodeFields(fields map[string]any) []byte {
	if len(fields) == 0 {
		return []byte("{}")
	}
	// Round-trip through JSON so typed values (structs, string slices) are
	// sanitized as plain maps and lists.
	raw, err := json.Marshal(fields)
	if err != nil {
		return []byte("{}")
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return []byte("{}")
	}
	encoded, err := json.Marshal(sanitize(generic))
	if err != nil {
		return []byte("{}")
	}
	return encoded
}

func parseUUID(value string) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || parsed == uuid.Nil {
		return uuid.Nil, false
	}
	return parsed, true
}

func optionalUUID(value string) *uuid.UUID {
	parsed, ok := parseUUID(value)
	if !ok {
		return nil
	}
	return &parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
