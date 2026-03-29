package relay

import (
	"strings"
	"testing"
	"time"
)

func TestFormatForAmp_Basic(t *testing.T) {
	msg := Message{
		From:    "auth",
		To:      "parent",
		Type:    MsgResult,
		Content: "task complete",
		Time:    time.Now(),
	}

	result := FormatForAmp(msg)

	if result.Type != "user" {
		t.Errorf("expected Type %q, got %q", "user", result.Type)
	}
	if result.Message.Role != "user" {
		t.Errorf("expected Role %q, got %q", "user", result.Message.Role)
	}
	if len(result.Message.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Message.Content))
	}

	block := result.Message.Content[0]
	if block.Type != "text" {
		t.Errorf("expected content Type %q, got %q", "text", block.Type)
	}
	if !strings.Contains(block.Text, "[relay from auth]") {
		t.Errorf("expected text to contain %q, got %q", "[relay from auth]", block.Text)
	}
	if !strings.Contains(block.Text, "(result)") {
		t.Errorf("expected text to contain %q, got %q", "(result)", block.Text)
	}
	if !strings.Contains(block.Text, "task complete") {
		t.Errorf("expected text to contain %q, got %q", "task complete", block.Text)
	}
}

func TestFormatForAmp_AllMessageTypes(t *testing.T) {
	types := []MessageType{MsgTask, MsgResult, MsgStatus, MsgQuestion, MsgError}

	for _, mt := range types {
		t.Run(string(mt), func(t *testing.T) {
			msg := Message{
				From:    "agent",
				To:      "parent",
				Type:    mt,
				Content: "payload",
				Time:    time.Now(),
			}

			result := FormatForAmp(msg)

			if len(result.Message.Content) == 0 {
				t.Fatal("expected at least 1 content block")
			}

			text := result.Message.Content[0].Text
			if !strings.Contains(text, "("+string(mt)+")") {
				t.Errorf("expected text to contain %q, got %q", "("+string(mt)+")", text)
			}
		})
	}
}

func TestFormatForAmp_EmptyContent(t *testing.T) {
	msg := Message{
		From:    "worker",
		To:      "parent",
		Type:    MsgStatus,
		Content: "",
		Time:    time.Now(),
	}

	result := FormatForAmp(msg)

	if len(result.Message.Content) == 0 {
		t.Fatal("expected at least 1 content block")
	}

	text := result.Message.Content[0].Text
	if !strings.Contains(text, "[relay from worker]") {
		t.Errorf("expected prefix in text, got %q", text)
	}
}
