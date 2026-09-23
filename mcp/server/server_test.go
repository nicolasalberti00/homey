package server

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestClientInitializesInProcess is the smoke test of the server itself: a
// client connects and sees who it is talking to. The HTTP transport has its
// own test where the endpoint is mounted.
func TestClientInitializesInProcess(t *testing.T) {
	ctx := t.Context()
	server := New(nil)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connecting the server: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "homey-test", Version: "0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connecting the client: %v", err)
	}
	defer session.Close()

	result := session.InitializeResult()
	if result == nil {
		t.Fatal("initialization returned no result")
	}
	if result.ServerInfo.Name != Name {
		t.Fatalf("server name = %q, want %q", result.ServerInfo.Name, Name)
	}
	if result.ServerInfo.Version == "" {
		t.Fatal("server version is empty")
	}
	if result.Instructions == "" {
		t.Fatal("server sent no instructions")
	}
}

func TestVersionIsNeverEmpty(t *testing.T) {
	if got := version(); got == "" {
		t.Fatal("version is empty")
	}
}
