// Package cdp is a small Chrome DevTools Protocol client over a stdlib-only WebSocket,
// enough for headbox's own commands (shot). Tests and agents bring their own clients.
package cdp

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// wsGUID is the fixed key suffix from RFC 6455, section 1.3.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// maxMessage caps one incoming message; a full-page PNG in base64 stays well below.
const maxMessage = 256 << 20

const (
	opCont  = 0x0
	opText  = 0x1
	opBin   = 0x2
	opClose = 0x8
	opPing  = 0x9
	opPong  = 0xA
)

// wsConn is a client WebSocket connection: text messages out, whole messages in.
type wsConn struct {
	conn net.Conn
	r    *bufio.Reader
}

// acceptKey is the Sec-WebSocket-Accept value the server must send for key.
func acceptKey(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// dialWS opens a WebSocket to a ws:// URL. No Origin header is sent: Chrome only
// accepts origin-less clients unless --remote-allow-origins is set.
func dialWS(ctx context.Context, wsURL string) (*wsConn, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "ws" {
		return nil, fmt.Errorf("unsupported WebSocket scheme %q (want ws)", u.Scheme)
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	key := base64.StdEncoding.EncodeToString(nonce)
	req := "GET " + u.RequestURI() + " HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, err
	}
	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, &http.Request{Method: http.MethodGet})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("websocket handshake: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("websocket handshake: %s", resp.Status)
	}
	if resp.Header.Get("Sec-WebSocket-Accept") != acceptKey(key) {
		conn.Close()
		return nil, errors.New("websocket handshake: bad Sec-WebSocket-Accept")
	}
	return &wsConn{conn: conn, r: r}, nil
}

func (c *wsConn) Close() error { return c.conn.Close() }

// SetDeadline bounds all further reads and writes.
func (c *wsConn) SetDeadline(t time.Time) error { return c.conn.SetDeadline(t) }

// WriteText sends one masked text frame, as RFC 6455 requires of clients.
func (c *wsConn) WriteText(p []byte) error {
	return writeFrame(c.conn, opText, p, true)
}

// ReadMessage returns the next text or binary message, answering pings on the way.
func (c *wsConn) ReadMessage() ([]byte, error) {
	var msg []byte
	for {
		fin, op, p, err := readFrame(c.r)
		if err != nil {
			return nil, err
		}
		switch op {
		case opPing:
			if err := writeFrame(c.conn, opPong, p, true); err != nil {
				return nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			_ = writeFrame(c.conn, opClose, nil, true)
			return nil, io.EOF
		case opText, opBin, opCont:
		default:
			return nil, fmt.Errorf("websocket: unknown opcode %#x", op)
		}
		if len(msg)+len(p) > maxMessage {
			return nil, errors.New("websocket: message too large")
		}
		msg = append(msg, p...)
		if fin {
			return msg, nil
		}
	}
}

// writeFrame writes a single final frame; mask is set for client frames.
func writeFrame(w io.Writer, op byte, p []byte, mask bool) error {
	hdr := make([]byte, 0, 14)
	hdr = append(hdr, 0x80|op)
	var mbit byte
	if mask {
		mbit = 0x80
	}
	switch n := len(p); {
	case n < 126:
		hdr = append(hdr, mbit|byte(n))
	case n <= 0xFFFF:
		hdr = append(hdr, mbit|126)
		hdr = binary.BigEndian.AppendUint16(hdr, uint16(n))
	default:
		hdr = append(hdr, mbit|127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	body := p
	if mask {
		var key [4]byte
		_, _ = rand.Read(key[:])
		hdr = append(hdr, key[:]...)
		body = make([]byte, len(p))
		for i, b := range p {
			body[i] = b ^ key[i%4]
		}
	}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

// readFrame reads one frame and unmasks its payload if it is masked.
func readFrame(r io.Reader) (fin bool, op byte, p []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(r, h[:]); err != nil {
		return
	}
	fin, op = h[0]&0x80 != 0, h[0]&0x0F
	masked := h[1]&0x80 != 0
	n := uint64(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > maxMessage {
		err = errors.New("websocket: frame too large")
		return
	}
	var key [4]byte
	if masked {
		if _, err = io.ReadFull(r, key[:]); err != nil {
			return
		}
	}
	p = make([]byte, n)
	if _, err = io.ReadFull(r, p); err != nil {
		return
	}
	if masked {
		for i := range p {
			p[i] ^= key[i%4]
		}
	}
	return
}
