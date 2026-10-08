package tasks

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/errx"
	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/repository"
	"github.com/warmbly/warmbly/internal/tasks/proto"
)

type deniedEmailRepo struct {
	repository.EmailRepository
	account *models.Email
}

func (r deniedEmailRepo) GetByID(context.Context, uuid.UUID) (*models.Email, *errx.Error) {
	return r.account, nil
}

type deniedTaskRepo struct {
	repository.TaskRepository
	task      *repository.Task
	mu        sync.Mutex
	cancelled int
	created   int
}

func (r *deniedTaskRepo) GetTask(context.Context, uuid.UUID) (*repository.Task, error) {
	return r.task, nil
}
func (r *deniedTaskRepo) UpdateTaskStatus(_ context.Context, _ uuid.UUID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if status == "cancelled" {
		r.cancelled++
	}
	return nil
}
func (r *deniedTaskRepo) CreateWarmupTaskWithLock(context.Context, *repository.Task, *repository.WarmupTask) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created++
	return true, nil
}

func TestDeniedWarmupTaskDoesNotSendOrSeed(t *testing.T) {
	id, taskID := uuid.New(), uuid.New()
	account := &models.Email{ID: id, Status: "active", WarmupDenied: true}
	repo := &deniedTaskRepo{task: &repository.Task{ID: taskID, EmailAccountID: id, Status: "pending"}}
	service := &tasksService{emailRepo: deniedEmailRepo{account: account}, taskRepo: repo}
	if err := service.HandleEmailTask(&proto.ProcessTask{TaskId: taskID.String()}); err != nil {
		t.Fatal(err)
	}
	if err := service.createWarmupTask(context.Background(), id, time.Now()); err != nil {
		t.Fatal(err)
	}
	if repo.cancelled != 1 || repo.created != 0 {
		t.Fatalf("cancelled=%d created=%d", repo.cancelled, repo.created)
	}
}

func TestDeniedWarmupConcurrentEnqueues(t *testing.T) {
	id := uuid.New()
	repo := &deniedTaskRepo{}
	service := &tasksService{emailRepo: deniedEmailRepo{account: &models.Email{ID: id, Status: "active", WarmupDenied: true}}, taskRepo: repo}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := service.createWarmupTask(context.Background(), id, time.Now()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if repo.created != 0 {
		t.Fatalf("created %d tasks", repo.created)
	}
}
