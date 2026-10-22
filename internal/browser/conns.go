package browser

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// tcpEstablished is the state code of an established socket in /proc/net/tcp.
const tcpEstablished = "01"

// Connections counts established TCP connections whose local port is port, from
// /proc/net/tcp and tcp6. The sandbox shares the host network, so a CDP client's
// connection to the browser shows up here; serve uses it to detect idleness.
func Connections(port int) (int, error) {
	n := 0
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		c, err := countEstablished(f, port)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, err
		}
		n += c
	}
	return n, nil
}

func countEstablished(path string, port int) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return parseEstablished(bufio.NewScanner(f), port), nil
}

// parseEstablished reads /proc/net/tcp lines: "sl local_address rem_address st ...",
// addresses as HEXIP:HEXPORT.
func parseEstablished(sc *bufio.Scanner, port int) int {
	want := strings.ToUpper(strconv.FormatInt(int64(port), 16))
	for len(want) < 4 {
		want = "0" + want
	}
	n := 0
	sc.Scan() // header
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 || fields[3] != tcpEstablished {
			continue
		}
		local := fields[1]
		if i := strings.LastIndexByte(local, ':'); i >= 0 && local[i+1:] == want {
			n++
		}
	}
	return n
}
