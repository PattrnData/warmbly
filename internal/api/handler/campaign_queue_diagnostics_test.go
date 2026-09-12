package handler

import (
	"os"
	"strings"
	"testing"
)

func TestCampaignQueueDiagnosticsEndpointIsReadOnlyAndRouted(t *testing.T) {
	handlerSrc, err := os.ReadFile("campaign.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(handlerSrc), "GetCampaignQueueDiagnostics") {
		t.Fatalf("campaign handler must expose GetCampaignQueueDiagnostics")
	}
	routesSrc, err := os.ReadFile("../routes.go")
	if err != nil {
		t.Fatal(err)
	}
	routes := string(routesSrc)
	if !strings.Contains(routes, "GET(\"/:id/queue-diagnostics\"") || !strings.Contains(routes, "APIPermReadCampaigns") {
		t.Fatalf("queue diagnostics must be a read-only campaign route")
	}
}
