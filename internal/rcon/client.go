// Package rcon issues RCON commands to a running Minecraft server over TCP.
package rcon

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// playerListRe matches the vanilla/Paper `list` output:
var playerListRe = regexp.MustCompile(`There are (\d+) of a max of (\d+) players`)

// ParsePlayerList extracts the online and max player counts from `list` output.
func ParsePlayerList(out string) (players, max int, ok bool) {
	m := playerListRe.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, false
	}
	players, _ = strconv.Atoi(m[1])
	max, _ = strconv.Atoi(m[2])
	return players, max, true
}

// Client targets a single server's RCON port over TCP.
type Client struct {
	addr     string
	password string
}

// New binds a client to a local port address.
func New(addr, password string) *Client {
	return &Client{addr: addr, password: password}
}

// Run executes an arbitrary RCON command and returns its output.
func (c *Client) Run(ctx context.Context, args ...string) (string, error) {
	cmd := strings.Join(args, " ")
	
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return "", fmt.Errorf("rcon connect: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}

	// 1. Authenticate
	if err := writePacket(conn, 1, 3, c.password); err != nil {
		return "", fmt.Errorf("rcon write auth: %w", err)
	}
	respID, respType, _, err := readPacket(conn)
	if err != nil {
		return "", fmt.Errorf("rcon read auth: %w", err)
	}
	
	// Some servers send an empty SERVERDATA_RESPONSE_VALUE packet before SERVERDATA_AUTH_RESPONSE.
	if respType == 2 && respID == 0 {
		respID, respType, _, err = readPacket(conn)
		if err != nil {
			return "", fmt.Errorf("rcon read auth retry: %w", err)
		}
	}

	if respID == -1 {
		return "", fmt.Errorf("rcon authentication failed")
	}

	// 2. Send command
	if err := writePacket(conn, 2, 2, cmd); err != nil {
		return "", fmt.Errorf("rcon write cmd: %w", err)
	}

	_, _, respBody, err := readPacket(conn)
	if err != nil {
		return "", fmt.Errorf("rcon read cmd response: %w", err)
	}

	return respBody, nil
}

func writePacket(conn net.Conn, id, typ int32, payload string) error {
	payloadBytes := []byte(payload)
	size := int32(4 + 4 + len(payloadBytes) + 2) // id(4) + typ(4) + payload + null(1) + pad(1)
	
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, size)
	_ = binary.Write(buf, binary.LittleEndian, id)
	_ = binary.Write(buf, binary.LittleEndian, typ)
	buf.Write(payloadBytes)
	buf.WriteByte(0x00) // null terminator
	buf.WriteByte(0x00) // pad
	
	_, err := conn.Write(buf.Bytes())
	return err
}

func readPacket(conn net.Conn) (int32, int32, string, error) {
	var size int32
	if err := binary.Read(conn, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", err
	}
	if size < 10 || size > 65536 {
		return 0, 0, "", fmt.Errorf("invalid packet size: %d", size)
	}
	
	data := make([]byte, size)
	if _, err := io.ReadFull(conn, data); err != nil {
		return 0, 0, "", err
	}
	
	id := int32(binary.LittleEndian.Uint32(data[0:4]))
	typ := int32(binary.LittleEndian.Uint32(data[4:8]))
	
	payloadBytes := data[8:]
	if idx := bytes.IndexByte(payloadBytes, 0x00); idx >= 0 {
		payloadBytes = payloadBytes[:idx]
	}
	
	return id, typ, string(payloadBytes), nil
}

// SaveOff disables automatic world saving.
func (c *Client) SaveOff(ctx context.Context) error {
	_, err := c.Run(ctx, "save-off")
	return err
}

// SaveAll flushes all dirty chunks to disk.
func (c *Client) SaveAll(ctx context.Context) error {
	_, err := c.Run(ctx, "save-all")
	return err
}

// SaveOn re-enables automatic world saving.
func (c *Client) SaveOn(ctx context.Context) error {
	_, err := c.Run(ctx, "save-on")
	return err
}

// ListPlayers returns the raw output of the `list` command.
func (c *Client) ListPlayers(ctx context.Context) (string, error) {
	return c.Run(ctx, "list")
}
