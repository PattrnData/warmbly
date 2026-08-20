package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/warmbly/warmbly/internal/models"
)

// TaskResultRepository is the narrow task-write seam needed to acknowledge
// worker result events from jobs.worker-events. Keeping this small avoids
// coupling the worker-event consumer to the full task repository surface.
type TaskResultRepository interface {
	UpdateTaskStatus(ctx context.Context, taskID uuid.UUID, status string) error
	UpdateTaskMessageID(ctx context.Context, taskID uuid.UUID, messageID string) error
	RecordTaskFailure(ctx context.Context, taskID uuid.UUID, title, message string) error
}

func RegisterEmailError(w *JobsService, eventType models.JobEventType, handler EventHandler[models.EmailErrorEvent]) {
	w.eventHandlers[eventType] = func(ctx context.Context, body any) error {
		if looksLikeSendEmailResult(body) {
			result, err := normalizeSendEmailResult(body, eventType)
			if err != nil {
				return err
			}
			return w.HandleEmailFailed(ctx, result)
		}
		return registerAndCall(ctx, eventType, body, handler)
	}
}

func registerAndCall[T any](ctx context.Context, eventType models.JobEventType, body any, handler EventHandler[T]) error {
	if data, ok := body.(T); ok {
		return handler(ctx, data)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("invalid event body for type %v: %w", eventType, err)
	}
	var data T
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("invalid event body for type %v: %w", eventType, err)
	}
	return handler(ctx, data)
}

func looksLikeSendEmailResult(body any) bool {
	if _, ok := body.(models.SendEmailResult); ok {
		return true
	}
	if _, ok := body.(*models.SendEmailResult); ok {
		return true
	}
	m, ok := body.(map[string]any)
	if !ok {
		return false
	}
	if _, ok := m["success"]; ok {
		return true
	}
	if _, ok := m["error"]; ok {
		return true
	}
	if _, ok := m["legacy_error"]; ok {
		return true
	}
	return false
}

func normalizeSendEmailResult(body any, eventType models.JobEventType) (models.SendEmailResult, error) {
	if data, ok := body.(models.SendEmailResult); ok {
		return data, nil
	}
	if data, ok := body.(*models.SendEmailResult); ok && data != nil {
		return *data, nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return models.SendEmailResult{}, fmt.Errorf("invalid event body for type %v: %w", eventType, err)
	}
	var data models.SendEmailResult
	if err := json.Unmarshal(raw, &data); err != nil {
		return models.SendEmailResult{}, fmt.Errorf("invalid event body for type %v: %w", eventType, err)
	}
	return data, nil
}

func (s *JobsService) HandleEmailSent(ctx context.Context, result models.SendEmailResult) error {
	if s.TaskResultRepository == nil {
		log.Warn().Str("task_id", result.TaskID.String()).Msg("email sent result ignored: task repository is not configured")
		return nil
	}
	if result.TaskID == uuid.Nil {
		log.Warn().Msg("email sent result ignored: missing task_id")
		return nil
	}
	if strings.TrimSpace(result.MessageID) != "" {
		if err := s.TaskResultRepository.UpdateTaskMessageID(ctx, result.TaskID, result.MessageID); err != nil {
			return err
		}
	}
	return s.TaskResultRepository.UpdateTaskStatus(ctx, result.TaskID, "completed")
}

func (s *JobsService) HandleEmailFailed(ctx context.Context, result models.SendEmailResult) error {
	if s.TaskResultRepository == nil {
		log.Warn().Str("task_id", result.TaskID.String()).Msg("email failed result ignored: task repository is not configured")
		return nil
	}
	if result.TaskID == uuid.Nil {
		log.Warn().Msg("email failed result ignored: missing task_id")
		return nil
	}
	title := "Email send failed"
	message := strings.TrimSpace(result.LegacyErrorMsg)
	if result.Error != nil {
		if strings.TrimSpace(result.Error.Message) != "" {
			message = strings.TrimSpace(result.Error.Message)
		}
		if strings.TrimSpace(result.Error.Code) != "" {
			title = "Email send failed: " + strings.TrimSpace(result.Error.Code)
		}
	}
	if message == "" {
		message = "worker reported EMAIL_FAILED without an error message"
	}
	return s.TaskResultRepository.RecordTaskFailure(ctx, result.TaskID, title, message)
}
