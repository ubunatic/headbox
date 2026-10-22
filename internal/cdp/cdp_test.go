package cdp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestColorScheme(t *testing.T) {
	for dark, want := range map[bool]string{
		true:  `{"features":[{"name":"prefers-color-scheme","value":"dark"}]}`,
		false: `{"features":[{"name":"prefers-color-scheme","value":"light"}]}`,
	} {
		b, err := json.Marshal(ColorScheme(dark))
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != want {
			t.Errorf("dark=%v: got %s, want %s", dark, b, want)
		}
	}
}

func TestViewport(t *testing.T) {
	b, _ := json.Marshal(Viewport(320, 200))
	if want := `{"width":320,"height":200,"deviceScaleFactor":1,"mobile":false}`; string(b) != want {
		t.Errorf("got %s, want %s", b, want)
	}
}

func TestEncode(t *testing.T) {
	c := &Client{}
	id, b, err := c.encode("S1", "Emulation.setEmulatedMedia", ColorScheme(true))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":1,"method":"Emulation.setEmulatedMedia","params":{"features":[{"name":"prefers-color-scheme","value":"dark"}]},"sessionId":"S1"}`
	if id != 1 || string(b) != want {
		t.Errorf("got %d %s,\nwant 1 %s", id, b, want)
	}
	id, b, _ = c.encode("", "Page.enable", nil)
	if want := `{"id":2,"method":"Page.enable"}`; id != 2 || string(b) != want {
		t.Errorf("got %d %s, want 2 %s", id, b, want)
	}
}

func TestFrameRoundTrip(t *testing.T) {
	for _, n := range []int{0, 5, 125, 126, 0xFFFF, 0x10000} {
		p := bytes.Repeat([]byte{'x'}, n)
		for _, mask := range []bool{true, false} {
			var buf bytes.Buffer
			if err := writeFrame(&buf, opText, p, mask); err != nil {
				t.Fatal(err)
			}
			if got := buf.Bytes()[1]&0x80 != 0; got != mask {
				t.Errorf("n=%d: mask bit %v, want %v", n, got, mask)
			}
			fin, op, got, err := readFrame(&buf)
			if err != nil || !fin || op != opText || !bytes.Equal(got, p) {
				t.Errorf("n=%d mask=%v: fin=%v op=%d len=%d err=%v", n, mask, fin, op, len(got), err)
			}
		}
	}
}

func TestAcceptKey(t *testing.T) {
	// Example from RFC 6455, section 1.3.
	if got := acceptKey("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Errorf("got %s", got)
	}
}

// fakeBrowser is a CDP server that answers the screenshot sequence and records it.
type fakeBrowser struct {
	t       *testing.T
	calls   []Request
	png     []byte
	navErr  string
	fragged bool // send responses in two fragments, with a ping in between
}

func (fb *fakeBrowser) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	conn, rw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		fb.t.Error(err)
		return
	}
	defer conn.Close()
	rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey(r.Header.Get("Sec-WebSocket-Key")) + "\r\n\r\n")
	rw.Flush()
	send := func(v any) {
		b, _ := json.Marshal(v)
		if fb.fragged && len(b) > 2 {
			conn.Write([]byte{opText, 1, b[0]}) // not final
			writeFrame(conn, opPing, []byte("p"), false)
			writeFrame(conn, opCont, b[1:], false)
			return
		}
		writeFrame(conn, opText, b, false)
	}
	br := bufio.NewReader(rw)
	for {
		fin, op, p, err := readFrame(br)
		if err != nil {
			return
		}
		if op == opPong {
			continue
		}
		if !fin || op != opText {
			fb.t.Errorf("unexpected frame fin=%v op=%d", fin, op)
			return
		}
		var req Request
		if err := json.Unmarshal(p, &req); err != nil {
			fb.t.Error(err)
			return
		}
		fb.calls = append(fb.calls, req)
		result := map[string]any{}
		switch req.Method {
		case "Target.createTarget":
			result["targetId"] = "T1"
		case "Target.attachToTarget":
			result["sessionId"] = "S1"
		case "Page.navigate":
			if fb.navErr != "" {
				result["errorText"] = fb.navErr
			} else {
				// An event before the response, and one for another session.
				send(map[string]any{"method": "Page.loadEventFired", "sessionId": "other", "params": map[string]any{}})
				send(map[string]any{"method": "Page.loadEventFired", "sessionId": "S1", "params": map[string]any{"timestamp": 1}})
			}
		case "Page.captureScreenshot":
			result["data"] = base64.StdEncoding.EncodeToString(fb.png)
		}
		send(map[string]any{"id": req.ID, "result": result})
	}
}

func shoot(t *testing.T, fb *fakeBrowser, opt ShotOptions) ([]byte, error) {
	t.Helper()
	srv := httptest.NewServer(fb)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return Screenshot(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/devtools/browser/x", opt)
}

func TestScreenshotSequence(t *testing.T) {
	for _, fragged := range []bool{false, true} {
		fb := &fakeBrowser{t: t, png: []byte("\x89PNG fake"), fragged: fragged}
		png, err := shoot(t, fb, ShotOptions{URL: "file:///site/index.html", Width: 320, Height: 200, Dark: true})
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(png, fb.png) {
			t.Errorf("png %q", png)
		}
		var got []string
		for _, c := range fb.calls {
			p, _ := json.Marshal(c.Params)
			got = append(got, c.SessionID+" "+c.Method+" "+string(p))
		}
		want := []string{
			` Target.createTarget {"url":"about:blank"}`,
			` Target.attachToTarget {"flatten":true,"targetId":"T1"}`,
			`S1 Page.enable null`,
			`S1 Emulation.setDeviceMetricsOverride {"deviceScaleFactor":1,"height":200,"mobile":false,"width":320}`,
			`S1 Emulation.setEmulatedMedia {"features":[{"name":"prefers-color-scheme","value":"dark"}]}`,
			`S1 Page.navigate {"url":"file:///site/index.html"}`,
			`S1 Page.captureScreenshot {"format":"png"}`,
		}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("fragged=%v calls:\n%s\nwant:\n%s", fragged, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

func TestScreenshotNavigateError(t *testing.T) {
	fb := &fakeBrowser{t: t, navErr: "net::ERR_FILE_NOT_FOUND"}
	_, err := shoot(t, fb, ShotOptions{URL: "file:///nope.html", Width: 10, Height: 10})
	if err == nil || !strings.Contains(err.Error(), "ERR_FILE_NOT_FOUND") {
		t.Fatalf("got %v", err)
	}
}

func TestScreenshotBadViewport(t *testing.T) {
	if _, err := Screenshot(context.Background(), "ws://127.0.0.1:1/x", ShotOptions{Width: 0, Height: 10}); err == nil {
		t.Fatal("want an error for a zero width")
	}
}

func TestScreenshotCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		// Accept and never answer the handshake.
		c, err := ln.Accept()
		if err == nil {
			defer c.Close()
			<-done
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = Screenshot(ctx, "ws://"+ln.Addr().String()+"/x", ShotOptions{Width: 1, Height: 1})
	if err == nil {
		t.Fatal("want a timeout")
	}
}
