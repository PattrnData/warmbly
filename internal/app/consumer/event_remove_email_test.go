package jobs

import (
	"testing"
	"time"
)

func TestShouldRecordWarmupDeletionTampering(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		receivedAt time.Time
		want       bool
	}{
		{
			name:       "recent removal is treated as provider folder move noise",
			receivedAt: now.Add(-30 * time.Minute),
			want:       false,
		},
		{
			name:       "old deletion still records tampering",
			receivedAt: now.Add(-(warmupDeletionTamperingGrace + time.Minute)),
			want:       true,
		},
		{
			name:       "zero received time is conservative",
			receivedAt: time.Time{},
			want:       true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRecordWarmupDeletionTampering(tc.receivedAt, now)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
