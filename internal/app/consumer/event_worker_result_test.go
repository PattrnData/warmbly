package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

type fakeTaskResultRepo struct {
	statusTaskID  uuid.UUID
	status        string
	messageTaskID uuid.UUID
	messageID     string
	failureTaskID uuid.UUID
	failureTitle  string
	failureMsg    string
}

func (f *fakeTaskResultRepo) UpdateTaskStatus(_ context.Context, taskID uuid.UUID, status string) error {
	f.statusTaskID = taskID
	f.status = status
	return nil
}

func (f *fakeTaskResultRepo) UpdateTaskMessageID(_ context.Context, taskID uuid.UUID, messageID string) error {
	f.messageTaskID = taskID
	f.messageID = messageID
	return nil
}

func (f *fakeTaskResultRepo) RecordTaskFailure(_ context.Context, taskID uuid.UUID, title, message string) error {
	f.failureTaskID = taskID
	f.failureTitle = title
	f.failureMsg = message
	return nil
}

func TestEmailSentResultCompletesTaskAndPersistsMessageID(t *testing.T) {
	taskID := uuid.New()
	repo := &fakeTaskResultRepo{}
	svc := &JobsService{TaskResultRepository: repo}
	svc.InitEvents()

	err := svc.HandleEvent(context.Background(), &models.JobEvent{
		Type: models.JobEventTypeEmailSent,
		Body: map[string]any{
			"task_id":    taskID.String(),
			"success":    true,
			"message_id": "<sent-message@example.com>",
		},
	})
	if err != nil {
		t.Fatalf("HandleEvent EMAIL_SENT returned error: %v", err)
	}
	if repo.messageTaskID != taskID || repo.messageID != "<sent-message@example.com>" {
		t.Fatalf("message id update = (%s, %q), want (%s, %q)", repo.messageTaskID, repo.messageID, taskID, "<sent-message@example.com>")
	}
	if repo.statusTaskID != taskID || repo.status != "completed" {
		t.Fatalf("status update = (%s, %q), want (%s, completed)", repo.statusTaskID, repo.status, taskID)
	}
}

func TestEmailServerErrorTaskResultRecordsTaskFailure(t *testing.T) {
	taskID := uuid.New()
	repo := &fakeTaskResultRepo{}
	svc := &JobsService{TaskResultRepository: repo}
	svc.InitEvents()

	err := svc.HandleEvent(context.Background(), &models.JobEvent{
		Type: models.JobEventTypeEmailServerError,
		Body: map[string]any{
			"task_id": taskID.String(),
			"success": false,
			"error": map[string]any{
				"code":    "server_unreachable",
				"message": "server unavailable",
			},
		},
	})
	if err != nil {
		t.Fatalf("HandleEvent EMAIL_SERVER_ERROR send result returned error: %v", err)
	}
	if repo.failureTaskID != taskID || repo.failureMsg != "server unavailable" {
		t.Fatalf("failure record = (%s, %q), want (%s, %q)", repo.failureTaskID, repo.failureMsg, taskID, "server unavailable")
	}
}

func TestEmailServerErrorAccountEventStillUsesAccountHandler(t *testing.T) {
	taskID := uuid.New()
	repo := &fakeTaskResultRepo{}
	svc := &JobsService{TaskResultRepository: repo}
	svc.InitEvents()

	err := svc.HandleEvent(context.Background(), &models.JobEvent{
		Type: models.JobEventTypeEmailServerError,
		Body: map[string]any{
			"task_id":          taskID.String(),
			"email_account_id": uuid.New().String(),
			"user_id":          uuid.New().String(),
			"error_code":       "server_unreachable",
			"message":          "server unavailable",
		},
	})
	if err != nil {
		t.Fatalf("HandleEvent EMAIL_SERVER_ERROR account event returned error: %v", err)
	}
	if repo.failureTaskID != uuid.Nil {
		t.Fatalf("account-shaped error event recorded task failure for %s", repo.failureTaskID)
	}
}

func TestEmailFailedResultRecordsTaskFailure(t *testing.T) {
	taskID := uuid.New()
	repo := &fakeTaskResultRepo{}
	svc := &JobsService{TaskResultRepository: repo}
	svc.InitEvents()

	err := svc.HandleEvent(context.Background(), &models.JobEvent{
		Type: models.JobEventTypeEmailFailed,
		Body: map[string]any{
			"task_id": taskID.String(),
			"success": false,
			"error": map[string]any{
				"code":    "smtp_timeout",
				"message": "SMTP timed out",
			},
		},
	})
	if err != nil {
		t.Fatalf("HandleEvent EMAIL_FAILED returned error: %v", err)
	}
	if repo.failureTaskID != taskID {
		t.Fatalf("failure task id = %s, want %s", repo.failureTaskID, taskID)
	}
	if repo.failureTitle != "Email send failed: smtp_timeout" {
		t.Fatalf("failure title = %q", repo.failureTitle)
	}
	if repo.failureMsg != "SMTP timed out" {
		t.Fatalf("failure message = %q", repo.failureMsg)
	}
}
