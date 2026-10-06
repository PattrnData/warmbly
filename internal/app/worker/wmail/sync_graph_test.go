package wmail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/warmbly/warmbly/internal/client/msgraph"
	"github.com/warmbly/warmbly/internal/models"
	"golang.org/x/oauth2"
)

func TestGraphSyncEmitsMailboxSuccessOnlyAfterBothFolders(t *testing.T) {
	for _, tc := range []struct {
		name       string
		junkStatus int
		junkBody   string
		wantEvents int
	}{
		{"both folders succeed", http.StatusOK, `{"value":[],"@odata.deltaLink":"cursor"}`, 1},
		{"junk rejects token after inbox success", http.StatusUnauthorized, "", 0},
		{"junk fails after inbox success", http.StatusServiceUnavailable, "", 0},
		{"junk returns no cursor", http.StatusOK, `{}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test" {
					http.Error(w, "missing test bearer", http.StatusUnauthorized)
					return
				}
				if r.URL.Path == "/junk" {
					w.WriteHeader(tc.junkStatus)
					if tc.junkStatus != http.StatusOK {
						return
					}
					_, _ = fmt.Fprint(w, tc.junkBody)
					return
				}
				_, _ = fmt.Fprint(w, `{"value":[],"@odata.deltaLink":"cursor"}`)
			}))
			defer server.Close()

			client := &msgraph.Client{DeltaLinks: map[string]string{
				msgraph.FolderInbox: server.URL + "/inbox",
				msgraph.FolderJunk:  server.URL + "/junk",
			}, OnTokenRefresh: func(context.Context, *oauth2.Token) error { return nil }}
			if err := client.Init(context.Background(), &oauth2.Token{AccessToken: "test", Expiry: time.Now().Add(time.Hour)}, oauth2.Config{}); err != nil {
				t.Fatal(err)
			}
			accountID := uuid.New()
			events := 0
			mail := &WMail{ID: accountID, UserID: uuid.New(), GraphData: &GraphData{Client: client}, onEvent: func(typ models.JobEventType, body any) error {
				if typ != models.JobEventTypeMailboxProviderSync {
					t.Fatalf("event type %s", typ)
				}
				if body.(*models.JobEventMailboxProviderSync).EmailID != accountID {
					t.Fatal("success for wrong mailbox")
				}
				if body.(*models.JobEventMailboxProviderSync).StartedAt.IsZero() {
					t.Fatal("success must carry the start of the completed sync pass")
				}
				events++
				return nil
			}}
			_ = mail.SyncGraph(context.Background())
			if events != tc.wantEvents {
				t.Fatalf("success events = %d, want %d", events, tc.wantEvents)
			}
		})
	}
}
