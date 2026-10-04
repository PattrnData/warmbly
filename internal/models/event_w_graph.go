package models

import "github.com/google/uuid"

// JobEventGraphDeltaUpdate carries the opaque per-folder Microsoft Graph delta
// cursor back to the control plane for persistence. Unlike the Gmail history id
// (a small monotonic int stored on the email row), a deltaLink is a long opaque
// URL, so it is stored per (email, folder) in its own table.
type JobEventGraphDeltaUpdate struct {
	UserID    uuid.UUID `json:"user_id"`
	EmailID   uuid.UUID `json:"email_id"`
	Folder    string    `json:"folder"`
	DeltaLink string    `json:"delta_link"`
}

// JobEventMailboxProviderSync identifies a mailbox whose provider sync completed.
type JobEventMailboxProviderSync struct {
	EmailID uuid.UUID `json:"email_id"`
}
