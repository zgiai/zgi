package digitalhuman

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

type approvalNoticeStore struct {
	RunStore
	notice *TaskApprovalNotice
}

func (s approvalNoticeStore) PendingTaskApproval(context.Context, *Run) (*TaskApprovalNotice, error) {
	if s.notice == nil {
		return nil, nil
	}
	copy := *s.notice
	return &copy, nil
}

type recordingApprovalNotifier struct{ notices []TaskApprovalNotice }

func (n *recordingApprovalNotifier) NotifyTaskApproval(_ context.Context, notice TaskApprovalNotice) error {
	n.notices = append(n.notices, notice)
	return nil
}

func TestTaskApprovalNoticeReachesOnlyAPIEndUsers(t *testing.T) {
	subject := uuid.New()
	waiting := &TaskApprovalNotice{TaskName: "Weekly digest", TaskAction: "update", MessageID: uuid.New(), ClientReference: "crm-42"}
	run := func(surface, kind string) *Run {
		return &Run{ID: uuid.New(), Surface: surface, UserSubjectKind: kind, UserSubjectID: &subject}
	}
	cases := []struct {
		name   string
		run    *Run
		notice *TaskApprovalNotice
		want   int
	}{
		{"api end user", run(SurfaceAPI, string(usersubject.APIEndUser)), waiting, 1},
		{"console member answers where they chat", run(SurfaceConsole, string(usersubject.Account)), waiting, 0},
		{"webapp user answers where they chat", run(SurfaceWebApp, string(usersubject.Account)), waiting, 0},
		{"waiting on something other than a task", run(SurfaceAPI, string(usersubject.APIEndUser)), nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			notifier := &recordingApprovalNotifier{}
			service := &RuntimeService{store: approvalNoticeStore{notice: tc.notice}, approvalNotifier: notifier}
			service.notifyTaskApproval(t.Context(), tc.run)
			if len(notifier.notices) != tc.want {
				t.Fatalf("notices = %d, want %d", len(notifier.notices), tc.want)
			}
			if tc.want == 1 {
				got := notifier.notices[0]
				if got.RecipientID != subject || got.RecipientKind != string(usersubject.APIEndUser) || got.ClientReference != "crm-42" {
					t.Fatalf("notice = %#v", got)
				}
			}
		})
	}
}
