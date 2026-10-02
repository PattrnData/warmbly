package sendpayload

import (
	"testing"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/emsg"
)

func TestFingerprintBindsEveryMaterialField(t *testing.T) {
	base := Content{
		From: "sender@example.org", MessageID: "msg-1", To: []string{"to@example.org"},
		CC: []string{"cc@example.org"}, BCC: []string{"bcc@example.org"},
		Subject: "subject", Plain: "plain", HTML: "<p>html</p>", InReplyTo: "prior",
		WarmupToken: "token", UnsubscribeURL: "https://example.org/unsubscribe",
		Attachments: []emsg.Attachment{{S3Key: "object", Filename: "file", MimeType: "text/plain"}},
	}
	original := base.Fingerprint()
	if original != base.Fingerprint() || len(original) != 64 {
		t.Fatal("fingerprint is not stable SHA-256")
	}
	for name, change := range map[string]func(*Content){
		"from":        func(c *Content) { c.From = "other@example.org" },
		"message-id":  func(c *Content) { c.MessageID = "msg-2" },
		"to":          func(c *Content) { c.To = []string{"other@example.org"} },
		"cc":          func(c *Content) { c.CC = nil },
		"bcc":         func(c *Content) { c.BCC = nil },
		"subject":     func(c *Content) { c.Subject = "other" },
		"plain":       func(c *Content) { c.Plain = "other" },
		"html":        func(c *Content) { c.HTML = "other" },
		"reply":       func(c *Content) { c.InReplyTo = "other" },
		"warmup":      func(c *Content) { c.IsWarmup = true },
		"token":       func(c *Content) { c.WarmupToken = "other" },
		"unsubscribe": func(c *Content) { c.UnsubscribeURL = "other" },
		"attachment-key": func(c *Content) {
			c.Attachments = []emsg.Attachment{{S3Key: "other", Filename: "file", MimeType: "text/plain"}}
		},
		"attachment-name": func(c *Content) {
			c.Attachments = []emsg.Attachment{{S3Key: "object", Filename: "other", MimeType: "text/plain"}}
		},
		"attachment-type": func(c *Content) {
			c.Attachments = []emsg.Attachment{{S3Key: "object", Filename: "file", MimeType: "other"}}
		},
		"attachment-bytes": func(c *Content) {
			c.Attachments = []emsg.Attachment{{S3Key: "object", Filename: "file", MimeType: "text/plain", SHA256: "different"}}
		},
		"parent": func(c *Content) {
			c.Parent = &models.EmailParent{MessageID: "forged", ThreadID: "other"}
		},
	} {
		copy := base
		change(&copy)
		if copy.Fingerprint() == original {
			t.Errorf("changed %s was not bound", name)
		}
	}
}
