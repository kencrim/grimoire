package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kencrim/grimoire/libs/relay"
)

// testSocketPath returns a short socket path under /tmp to avoid
// macOS's 104-byte Unix socket path limit.
func testSocketPath(t *testing.T, suffix string) string {
	t.Helper()
	path := fmt.Sprintf("/tmp/ws-test-%s-%d.sock", suffix, os.Getpid())
	t.Cleanup(func() { os.Remove(path) })
	return path
}

// mockServer creates a Unix socket server that accepts connections and responds to envelopes.
func mockServer(t *testing.T, socketPath string, handler func(env relay.Envelope) any) net.Listener {
	t.Helper()
	os.Remove(socketPath)
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return // listener closed
			}
			go func(c net.Conn) {
				defer c.Close()
				dec := json.NewDecoder(c)
				enc := json.NewEncoder(c)
				for {
					var env relay.Envelope
					if err := dec.Decode(&env); err != nil {
						return
					}
					resp := handler(env)
					enc.Encode(resp)
				}
			}(conn)
		}
	}()
	return l
}

func TestRelayClient_SendSuccess(t *testing.T) {
	socketPath := testSocketPath(t, "send")

	l := mockServer(t, socketPath, func(env relay.Envelope) any {
		return map[string]string{"status": "delivered"}
	})
	defer l.Close()

	client, err := newRelayClient(socketPath, "test-agent")
	if err != nil {
		t.Fatalf("newRelayClient: %v", err)
	}
	defer client.close()

	err = client.send("parent", "hello", relay.MsgStatus)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
}

func TestRelayClient_ReconnectOnBrokenPipe(t *testing.T) {
	socketPath := testSocketPath(t, "reconn")

	// Start server 1
	l1 := mockServer(t, socketPath, func(env relay.Envelope) any {
		return map[string]string{"status": "delivered"}
	})

	client, err := newRelayClient(socketPath, "test-agent")
	if err != nil {
		t.Fatalf("newRelayClient: %v", err)
	}
	defer client.close()

	// First send succeeds
	if err := client.send("parent", "msg1", relay.MsgStatus); err != nil {
		t.Fatalf("first send: %v", err)
	}

	// Stop server 1
	l1.Close()
	time.Sleep(50 * time.Millisecond)

	// Start server 2 on the same socket path
	l2 := mockServer(t, socketPath, func(env relay.Envelope) any {
		return map[string]string{"status": "delivered"}
	})
	defer l2.Close()

	// Second send should reconnect and succeed
	if err := client.send("parent", "msg2", relay.MsgStatus); err != nil {
		t.Fatalf("second send after reconnect: %v", err)
	}
}

func TestRelayClient_MaxRetriesExhausted(t *testing.T) {
	socketPath := testSocketPath(t, "retry")

	// Start a server so we can create a client
	l := mockServer(t, socketPath, func(env relay.Envelope) any {
		return map[string]string{"status": "delivered"}
	})

	client, err := newRelayClient(socketPath, "test-agent")
	if err != nil {
		t.Fatalf("newRelayClient: %v", err)
	}
	defer client.close()

	// Stop the server permanently and close the client's existing connection
	// to force a broken-pipe scenario on the next send attempt.
	l.Close()
	client.conn.Close()
	// Remove the socket file so reconnect attempts also fail
	os.Remove(socketPath)

	// Send should fail after retries
	err = client.send("parent", "doomed", relay.MsgStatus)
	if err == nil {
		t.Fatal("expected error after retries, got nil")
	}
	if !strings.Contains(err.Error(), "after 3 retries") {
		t.Fatalf("expected 'after 3 retries' in error, got: %v", err)
	}
}
