package socks5

import (
	"fmt"
	"io"
	"net"
)

func ReadBindReply(conn net.Conn) error {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	if header[0] != 0x05 || header[1] != 0x00 {
		return fmt.Errorf("reply failed: %d", header[1])
	}

	var addrLen int
	switch header[3] {
	case 0x01:
		addrLen = 4
	case 0x04:
		addrLen = 16
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return fmt.Errorf("read domain len: %w", err)
		}
		addrLen = int(lenBuf[0])
	default:
		return fmt.Errorf("invalid addr type: %d", header[3])
	}

	if _, err := io.ReadFull(conn, make([]byte, addrLen)); err != nil {
		return fmt.Errorf("read address: %w", err)
	}
	if _, err := io.ReadFull(conn, make([]byte, 2)); err != nil {
		return fmt.Errorf("read port: %w", err)
	}
	return nil
}
