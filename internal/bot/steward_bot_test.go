// Package bot provides Telegram bot implementations
package bot

import (
	"context"
	"testing"
	"time"

	"ParanoidX/internal/ai"
)


// TestNewBot handles the TestNewBot HTTP request.
func TestNewBot(t *testing.T) {
	c := ai.NewClient("", "")
	s := ai.NewSteward(c)
	b := New("test:token", s)
	if b.Token != "test:token" {
		t.Fatalf("expected test:token, got %s", b.Token)
	}
	if b.Steward != s {
		t.Fatal("expected steward reference")
	}
	if b.BaseURL != "https://api.telegram.org/bottest:token" {
		t.Fatalf("unexpected base url: %s", b.BaseURL)
	}
}


// TestBotContextCancel handles the TestBotContextCancel HTTP request.
func TestBotContextCancel(t *testing.T) {
	c := ai.NewClient("", "")
	s := ai.NewSteward(c)
	b := New("test:token", s)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := b.Run(ctx)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}


// TestSendMessageFailsWithoutServer handles the TestSendMessageFailsWithoutServer HTTP request.
func TestSendMessageFailsWithoutServer(t *testing.T) {
	c := ai.NewClient("", "")
	s := ai.NewSteward(c)
	b := New("test:token", s)
	b.BaseURL = "http://127.0.0.1:1/bottest:token"

	go b.Run(context.Background())

	err := b.sendMessage(12345, "hello", nil)
	if err == nil {
		t.Fatal("expected error from bogus server")
	}
	t.Logf("expected error: %v", err)
}


// TestGetUpdatesFailsWithoutServer handles the TestGetUpdatesFailsWithoutServer HTTP request.
func TestGetUpdatesFailsWithoutServer(t *testing.T) {
	c := ai.NewClient("", "")
	s := ai.NewSteward(c)
	b := New("test:token", s)
	b.BaseURL = "http://127.0.0.1:1/bottest:token"

	_, err := b.getUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error from bogus server")
	}
	t.Logf("expected error: %v", err)
}


// TestBotLoopExitsOnCancel handles the TestBotLoopExitsOnCancel HTTP request.
func TestBotLoopExitsOnCancel(t *testing.T) {
	c := ai.NewClient("", "")
	s := ai.NewSteward(c)
	b := New("test:token", s)
	b.BaseURL = "http://127.0.0.1:1/bottest:token"
	b.Client.Timeout = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := b.Run(ctx)
	if err == nil {
		t.Fatal("expected error from bogus server")
	}
	t.Logf("loop error: %v", err)
}
