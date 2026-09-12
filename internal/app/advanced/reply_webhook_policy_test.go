package advanced

import (
	"os"
	"strings"
	"testing"
)

func TestProcessIncomingReplyWebhookUsesDeepReplyClassForAutomation(t *testing.T) {
	srcBytes, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	for _, want := range []string{
		`replyClass = replyResult.Class`,
		`replySource = replyResult.Source`,
		`replyConfidence = replyResult.Confidence`,
		`isAutomatedReply = replyclassify.IsAutomated(replyResult.Class)`,
		`"intent":           replyClass`,
		`"legacy_intent":    string(intent)`,
		`"reply_class":      replyClass`,
		`"reply_source":     replySource`,
		`"reply_confidence": replyConfidence`,
		`"is_automated":     isAutomatedReply`,
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("ProcessIncomingReply webhook payload missing deep classifier automation field %s", want)
		}
	}
	if strings.Contains(src, `"intent":        string(intent)`) || strings.Contains(src, `"intent":           string(intent)`) {
		t.Fatalf("campaign.reply_received webhook intent must not use the shallow legacy classifier")
	}
}

func TestProcessIncomingReplySuppressesUnsubscribeFromDeepClassifier(t *testing.T) {
	srcBytes, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	if !strings.Contains(src, `replyClass == replyclassify.ClassUnsubscribe`) {
		t.Fatalf("unsubscribe suppression must trigger from the deep reply classifier, not keyword text only")
	}
}
