package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/models"
)

func TestReplyActionIntentBranchFieldsValidateAndMatch(t *testing.T) {
	for _, tc := range []struct {
		field string
		class string
	}{
		{field: "reply_question", class: "question"},
		{field: "reply_wrong_person", class: "wrong_person"},
		{field: "reply_bad_timing", class: "bad_timing"},
		{field: "reply_referral", class: "referral"},
		{field: "reply_unsubscribe", class: "unsubscribe"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			bc := &models.BranchConditions{Branches: []models.Branch{{
				BranchID:         tc.field,
				TargetSequenceID: ptrUUID(uuid.New()),
				Conditions: []models.BranchCondition{{
					Field:    tc.field,
					Operator: "ever",
				}},
			}}}
			if err := validateBranchConditions(bc); err != nil {
				t.Fatalf("validateBranchConditions(%s) = %v", tc.field, err)
			}
			state, _ := evaluateBranchState(&bc.Branches[0], &CampaignContactProgress{ReplyClass: tc.class}, time.Time{}, time.Time{})
			if state != BranchMatch {
				t.Fatalf("evaluateBranchState(%s/%s) = %v, want BranchMatch", tc.field, tc.class, state)
			}
			if !fieldBelongsToEvent(tc.field, "reply") {
				t.Fatalf("fieldBelongsToEvent(%s, reply) = false, want true", tc.field)
			}
		})
	}
}

func TestReplyActionIntentBranchesArePositiveReplyBranches(t *testing.T) {
	for _, field := range []string{"reply_question", "reply_wrong_person", "reply_bad_timing", "reply_referral", "reply_unsubscribe", "reply_negative", "reply_automated"} {
		b := &models.Branch{Conditions: []models.BranchCondition{{Field: field, Operator: "ever"}}}
		if !branchHasPositiveReplyCondition(b) {
			t.Fatalf("branchHasPositiveReplyCondition(%s) = false, want true", field)
		}
	}
}

func ptrUUID(v uuid.UUID) *uuid.UUID { return &v }
