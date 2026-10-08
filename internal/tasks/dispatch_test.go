package tasks

import "testing"

func TestDispatchKindForTaskType(t *testing.T) {
	tests := []struct {
		name     string
		taskType string
		want     dispatchKind
	}{
		{name: "campaign", taskType: "campaign", want: dispatchKindCampaign},
		{name: "warmup", taskType: "warmup", want: dispatchKindWarmup},
		{name: "legacy user email", taskType: "user_email", want: dispatchKindUserEmail},
		{name: "scheduled email", taskType: "email", want: dispatchKindUserEmail},
		{name: "unknown", taskType: "nope", want: dispatchKindUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dispatchKindForTaskType(tt.taskType); got != tt.want {
				t.Fatalf("dispatchKindForTaskType(%q) = %q, want %q", tt.taskType, got, tt.want)
			}
		})
	}
}
