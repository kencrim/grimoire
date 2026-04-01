package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PushService manages Expo push tokens and sends push notifications
// when agent status changes. Notifications are delivered through Expo's
// push API, which routes through APNs/FCM to reach the device even
// when the app is closed.
type PushService struct {
	mu        sync.RWMutex
	tokens    []string
	storePath string
}

// expoPushMessage is the payload sent to Expo's push API.
type expoPushMessage struct {
	To    string            `json:"to"`
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
	Sound string            `json:"sound,omitempty"`
}

const expoPushURL = "https://exp.host/--/api/v2/push/send"

// NewPushService creates a push service that persists tokens to disk.
func NewPushService() *PushService {
	home, _ := os.UserHomeDir()
	storePath := filepath.Join(home, ".config", "ws", "push-tokens.json")
	ps := &PushService{storePath: storePath}
	ps.load()
	return ps
}

// RegisterToken adds an Expo push token. Duplicates are ignored.
func (ps *PushService) RegisterToken(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	for _, t := range ps.tokens {
		if t == token {
			return // already registered
		}
	}

	ps.tokens = append(ps.tokens, token)
	ps.save()
	log.Printf("[push] registered token: %s...%s", token[:20], token[len(token)-4:])
}

// RemoveToken removes an Expo push token.
func (ps *PushService) RemoveToken(token string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	for i, t := range ps.tokens {
		if t == token {
			ps.tokens = append(ps.tokens[:i], ps.tokens[i+1:]...)
			ps.save()
			return
		}
	}
}

// NotifyIdle sends a push notification to all registered devices
// that an agent has gone idle.
func (ps *PushService) NotifyIdle(agent AgentStatus) {
	ps.mu.RLock()
	tokens := make([]string, len(ps.tokens))
	copy(tokens, ps.tokens)
	ps.mu.RUnlock()

	if len(tokens) == 0 {
		return
	}

	name := agent.ID
	if idx := strings.LastIndex(agent.ID, "/"); idx >= 0 {
		name = agent.ID[idx+1:]
	}
	agentLabel := ""
	if agent.Agent != "" {
		agentLabel = " (" + agent.Agent + ")"
	}

	for _, token := range tokens {
		msg := expoPushMessage{
			To:    token,
			Title: name + " is ready",
			Body:  agent.ID + agentLabel + " is waiting for input",
			Data:  map[string]string{"agentId": agent.ID},
			Sound: "default",
		}
		go ps.send(msg)
	}
}

func (ps *PushService) send(msg expoPushMessage) {
	body, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[push] marshal error: %v", err)
		return
	}

	req, err := http.NewRequest("POST", expoPushURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("[push] request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[push] send error: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[push] expo returned %d for agent %s", resp.StatusCode, msg.Data["agentId"])
	}
}

func (ps *PushService) load() {
	data, err := os.ReadFile(ps.storePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &ps.tokens)
	if len(ps.tokens) > 0 {
		log.Printf("[push] loaded %d token(s)", len(ps.tokens))
	}
}

func (ps *PushService) save() {
	data, err := json.Marshal(ps.tokens)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(ps.storePath), 0o755)
	if err := os.WriteFile(ps.storePath, data, 0o600); err != nil {
		log.Printf("[push] save error: %v", err)
	}
}

// HandlePushToken returns an HTTP handler for POST /api/push-token.
// Expects JSON body: {"token": "ExponentPushToken[...]"}
func (ps *PushService) HandlePushToken(authCheck func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	return authCheck(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, fmt.Sprintf("invalid body: %v", err), http.StatusBadRequest)
			return
		}

		if !strings.HasPrefix(payload.Token, "ExponentPushToken[") {
			http.Error(w, "invalid expo push token format", http.StatusBadRequest)
			return
		}

		ps.RegisterToken(payload.Token)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
	})
}
