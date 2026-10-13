package zgimarket

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// Release ends the market binding of a subject that no longer exists: the market deletes the
// binding and revokes its credential, as it does when the last market item is removed. A subject
// that still exists, or one without market items, is left alone.
//
// The delete of the subject has already happened and is not undone. When the market cannot be
// reached the binding stays there with an active credential, and the row here is kept so the
// binding can still be found and ended: it names the market binding, and the organization's
// market credentials page lists the credential for revocation.
func (t *Tools) Release(ctx context.Context, who caller, subjectType string, subjectID uuid.UUID) error {
	if t == nil || t.proxy == nil {
		return nil
	}
	organizationID, err := uuid.Parse(who.organizationID)
	if err != nil {
		return nil
	}
	existing, err := t.store.find(ctx, organizationID, subjectType, subjectID)
	if err != nil || existing == nil {
		return err
	}
	exists, err := t.store.subjectExists(ctx, organizationID, subjectType, subjectID)
	if err != nil || exists {
		return err
	}
	path := "bindings/" + strconv.FormatUint(existing.MarketBindingID, 10)
	if err := t.marketData(ctx, who, http.MethodDelete, path, nil, nil); err != nil && !isNotFound(err) {
		return err
	}
	if err := t.store.delete(ctx, existing.ID); err != nil {
		return err
	}
	t.forget(existing.ID)
	return nil
}

// releasedSubjects are the routes that delete a subject market items may be bound to.
var releasedSubjects = []struct{ suffix, param, subjectType string }{
	{"/agents/:agent_id", "agent_id", SubjectAgent},
	{"/digital-humans/:id", "id", SubjectDigitalHuman},
}

// ReleaseOnDelete ends the market binding of an agent or a digital human once the request that
// deletes it has gone through. It looks at what is left rather than at the answer: the binding is
// released only when the subject is gone. A failure is logged and never changes the answer.
func ReleaseOnDelete(t *Tools) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if t == nil || c.Request.Method != http.MethodDelete {
			return
		}
		route := c.FullPath()
		for _, subject := range releasedSubjects {
			if !strings.HasSuffix(route, subject.suffix) {
				continue
			}
			subjectID, err := uuid.Parse(strings.TrimSpace(c.Param(subject.param)))
			if err != nil {
				return
			}
			who := caller{organizationID: util.GetOrganizationID(c), accountID: c.GetString("account_id"), admin: true}
			// The request may be over for the client; the release still has to finish.
			ctx := context.WithoutCancel(c.Request.Context())
			if err := t.Release(ctx, who, subject.subjectType, subjectID); err != nil {
				logger.WarnContext(ctx, "the ZGI Market binding of a deleted subject was not released; its credential stays active at the market until it is revoked there",
					"subject_type", subject.subjectType, "subject_id", subjectID.String(), "error", err.Error())
			}
			return
		}
	}
}
