package browser

import (
	"bufio"
	"strings"
	"testing"
)

func TestParseEstablished(t *testing.T) {
	// Port 9222 is 0x2406; 0x0240 (port 576) is a near miss.
	in := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:2406 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 1 1
   1: 0100007F:2406 0100007F:A1B2 01 00000000:00000000 00:00000000 00000000  1000        0 2 1
   2: 0100007F:A1B2 0100007F:2406 01 00000000:00000000 00:00000000 00000000  1000        0 3 1
   3: 0100007F:0240 0100007F:A1B3 01 00000000:00000000 00:00000000 00000000  1000        0 4 1
   4: 0100007F:2406 0100007F:A1B4 06 00000000:00000000 00:00000000 00000000  1000        0 5 1
   5: 0100007F:2406 0100007F:A1B5 01 00000000:00000000 00:00000000 00000000  1000        0 6 1
`
	got := parseEstablished(bufio.NewScanner(strings.NewReader(in)), 9222)
	// Lines 1 and 5: established with local port 9222. Not 0 (listen), 2 (client side),
	// 3 (other port), 4 (TIME_WAIT).
	if got != 2 {
		t.Errorf("got %d, want 2", got)
	}
}
