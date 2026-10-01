package tasks

import (
	"testing"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/emsg"
	"github.com/warmbly/warmbly/internal/pkg/sendpayload"
)

func TestSendFingerprintMatchesDecodedProviderFields(t *testing.T) {
	for _, warmup := range []bool{false, true} {
		msg := EmailMessage{
			From: "sender@example.org", To: []string{"to@example.org"},
			CC: []string{}, BCC: nil, Subject: "subject", BodyPlain: "plain", BodyHTML: "<p>html</p>",
			MessageID: "msg", InReplyTo: "parent", IsWarmup: warmup,
			WarmupToken: "token", UnsubscribeURL: "url",
			Attachments: []models.AttachmentRef{{S3Key: "key", Filename: "name", MimeType: "text/plain"}},
		}
		html := msg.BodyHTML
		if warmup {
			html = ""
		}
		got := sendFingerprint(msg)
		want := (sendpayload.Content{
			From: msg.From, To: msg.To, CC: nil, BCC: []string{}, Subject: msg.Subject,
			Plain: msg.BodyPlain, HTML: html, MessageID: msg.MessageID, InReplyTo: msg.InReplyTo,
			IsWarmup: msg.IsWarmup, WarmupToken: msg.WarmupToken, UnsubscribeURL: msg.UnsubscribeURL,
			Attachments: []emsg.Attachment{{S3Key: "key", Filename: "name", MimeType: "text/plain"}},
		}).Fingerprint()
		if got != want {
			t.Fatalf("warmup=%v producer %s != worker %s", warmup, got, want)
		}
		msg.To = []string{"tampered@example.org"}
		if sendFingerprint(msg) == want {
			t.Fatal("changed recipient retained hash")
		}
	}
}
