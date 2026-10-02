// Package sendpayload defines the canonical content checked before a provider send.
package sendpayload

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/emsg"
)

// Content is the plaintext intended send, including all envelope recipients and
// material headers. Empty recipient and attachment lists normalize because
// Avro and the binary body format may decode absent slices as empty slices.
type Content struct {
	From, MessageID                                              string
	To, CC, BCC                                                  []string
	Subject, Plain, HTML, InReplyTo, WarmupToken, UnsubscribeURL string
	IsWarmup                                                     bool
	Attachments                                                  []emsg.Attachment
	Parent                                                       *models.EmailParent
}

func (c Content) Fingerprint() string {
	if len(c.To) == 0 {
		c.To = nil
	}
	if len(c.CC) == 0 {
		c.CC = nil
	}
	if len(c.BCC) == 0 {
		c.BCC = nil
	}
	if len(c.Attachments) == 0 {
		c.Attachments = nil
	}
	data, err := json.Marshal(c)
	if err != nil { // This fixed, JSON-compatible struct cannot fail to marshal.
		panic(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
