package socks5_test

import (
	"net"
	"strings"
	"testing"
	"time"

	"ParanoidX/internal/socks5"
)

func TestReadBindReplyIPv4(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() { server.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x7f, 0x00, 0x00, 0x01, 0x00, 0x50}) }()

	if err := socks5.ReadBindReply(client); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadBindReplyIPv6(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() { server.Write([]byte{
		0x05, 0x00, 0x00, 0x04,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x50,
	}) }()

	if err := socks5.ReadBindReply(client); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadBindReplyDomain(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() { server.Write([]byte{0x05, 0x00, 0x00, 0x03, 0x03, 0x61, 0x62, 0x63, 0x00, 0x50}) }()

	if err := socks5.ReadBindReply(client); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadBindReplyFailedAuth(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() { server.Write([]byte{0x05, 0x01, 0x00, 0x01, 0x7f, 0x00, 0x00, 0x01, 0x00, 0x50}) }()

	err := socks5.ReadBindReply(client)
	if err == nil {
		t.Fatal("expected error for failed auth, got nil")
	}
	if !strings.Contains(err.Error(), "reply failed") {
		t.Fatalf("expected 'reply failed' error, got: %v", err)
	}
}

func TestReadBindReplyTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	client.SetDeadline(time.Now().Add(100 * time.Millisecond))

	err := socks5.ReadBindReply(client)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestReadBindReplyTruncatedHeader(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	go func() { server.Write([]byte{0x05, 0x00}); server.Close() }()

	err := socks5.ReadBindReply(client)
	if err == nil {
		t.Fatal("expected error for truncated header, got nil")
	}
}
