# headbox

A headless browser in a box: one Go binary that installs a pinned
[chrome-headless-shell](https://developer.chrome.com/blog/chrome-headless-shell),
runs it inside [bubblewrap](https://github.com/containers/bubblewrap) and leaves nothing
behind, even after `kill -9`. Tests and agents talk to it over the Chrome DevTools Protocol.

Status: planning. Nothing to install yet.

```
headbox install                 download + sha256-check the pinned browser
headbox exec [flags] -- CMD     run CMD with CDP_URL set, stop the browser after CMD
headbox serve [--idle 15m]      long-lived browser for agents, stops when idle
headbox shot URL -o FILE        screenshot (--width, --theme light|dark, --full-page)
headbox version                 launcher and browser versions
```

Linux only for now (needs `bwrap`). Install later with
`go install ubunatic.com/headbox@latest`.

Licence: AGPL-3.0-or-later.
