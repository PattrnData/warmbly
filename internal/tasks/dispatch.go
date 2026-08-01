package tasks

import (
	"context"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/tasks/proto"
)

// HandleTask routes a Cloud Tasks callback to the handler matching the task
// row's task_type. All enqueues share the single CLOUD_TASKS_WEBHOOK_URL, so
// without this dispatch a campaign task landing on the email webhook would be
// run through the warmup handler.
func (s *tasksService) HandleTask(task *proto.ProcessTask) *errx.Error {
	taskID, err := uuid.Parse(task.TaskId)
	if err != nil {
		sentry.CaptureException(err)
		return errx.New(errx.BadRequest, "invalid task ID")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	rec, err := s.taskRepo.GetTask(ctx, taskID)
	cancel()
	if err != nil {
		sentry.CaptureException(err)
		return errx.InternalError()
	}
	if rec == nil {
		return errx.ErrNotFound
	}

	switch dispatchKindForTaskType(rec.TaskType) {
	case dispatchKindCampaign:
		return s.HandleCampaignTask(task)
	case dispatchKindWarmup:
		return s.HandleEmailTask(task)
	case dispatchKindUserEmail:
		return s.HandleUserEmailTask(task)
	default:
		log.Warn().Str("task_id", taskID.String()).Str("task_type", rec.TaskType).Msg("task dispatch: unknown task type")
		return errx.New(errx.BadRequest, "unknown task type")
	}
}

type dispatchKind string

const (
	dispatchKindUnknown   dispatchKind = ""
	dispatchKindCampaign  dispatchKind = "campaign"
	dispatchKindWarmup    dispatchKind = "warmup"
	dispatchKindUserEmail dispatchKind = "user_email"
)

func dispatchKindForTaskType(taskType string) dispatchKind {
	switch taskType {
	case "campaign":
		return dispatchKindCampaign
	case "warmup":
		return dispatchKindWarmup
	case "user_email", "email":
		return dispatchKindUserEmail
	default:
		return dispatchKindUnknown
	}
}
