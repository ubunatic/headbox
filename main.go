// headbox installs a pinned chrome-headless-shell and runs it in a bwrap sandbox that
// leaves no process or file behind, for tests and agents.
package main

import (
	"errors"
	"fmt"
	"os"

	"ubunatic.com/headbox/internal/cli"
)

func main() {
	err := cli.New(Version).Execute()
	var ee *cli.ExitError
	if errors.As(err, &ee) {
		os.Exit(ee.Code)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "headbox:", err)
		os.Exit(1)
	}
}
