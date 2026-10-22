package cdp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
)

// ShotOptions for Screenshot.
type ShotOptions struct {
	URL    string
	Width  int
	Height int
	Dark   bool // emulate prefers-color-scheme: dark; light otherwise
}

// MediaFeature is one Emulation.setEmulatedMedia feature.
type MediaFeature struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// EmulatedMedia are the Emulation.setEmulatedMedia params.
type EmulatedMedia struct {
	Features []MediaFeature `json:"features"`
}

// ColorScheme returns the setEmulatedMedia params for the color scheme. Dark mode
// is emulated as a media feature, so pages see it exactly like a user preference.
func ColorScheme(dark bool) EmulatedMedia {
	v := "light"
	if dark {
		v = "dark"
	}
	return EmulatedMedia{Features: []MediaFeature{{Name: "prefers-color-scheme", Value: v}}}
}

// DeviceMetrics are the Emulation.setDeviceMetricsOverride params.
type DeviceMetrics struct {
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	DeviceScaleFactor float64 `json:"deviceScaleFactor"`
	Mobile            bool    `json:"mobile"`
}

// Viewport returns a desktop viewport of w x h CSS pixels at scale 1, so the PNG has
// exactly w x h pixels.
func Viewport(w, h int) DeviceMetrics {
	return DeviceMetrics{Width: w, Height: h, DeviceScaleFactor: 1}
}

// Screenshot opens a new page in the browser at wsURL, sets viewport and color
// scheme, loads opt.URL, waits for its load event and returns the PNG.
func Screenshot(ctx context.Context, wsURL string, opt ShotOptions) ([]byte, error) {
	if opt.Width <= 0 || opt.Height <= 0 {
		return nil, fmt.Errorf("bad viewport %dx%d", opt.Width, opt.Height)
	}
	c, err := Dial(ctx, wsURL)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	png, err := screenshot(ctx, c, opt)
	if err != nil && ctx.Err() != nil {
		return nil, fmt.Errorf("%w (%w)", ctx.Err(), err)
	}
	return png, err
}

func screenshot(ctx context.Context, c *Client, opt ShotOptions) ([]byte, error) {
	var target struct {
		TargetID string `json:"targetId"`
	}
	if err := c.Call(ctx, "", "Target.createTarget", map[string]any{"url": "about:blank"}, &target); err != nil {
		return nil, err
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := c.Call(ctx, "", "Target.attachToTarget",
		map[string]any{"targetId": target.TargetID, "flatten": true}, &attached); err != nil {
		return nil, err
	}
	s := attached.SessionID
	steps := []struct {
		method string
		params any
	}{
		{"Page.enable", nil},
		{"Emulation.setDeviceMetricsOverride", Viewport(opt.Width, opt.Height)},
		{"Emulation.setEmulatedMedia", ColorScheme(opt.Dark)},
	}
	for _, st := range steps {
		if err := c.Call(ctx, s, st.method, st.params, nil); err != nil {
			return nil, err
		}
	}
	c.DropEvents()
	var nav struct {
		ErrorText string `json:"errorText"`
	}
	if err := c.Call(ctx, s, "Page.navigate", map[string]any{"url": opt.URL}, &nav); err != nil {
		return nil, err
	}
	if nav.ErrorText != "" {
		return nil, fmt.Errorf("load %s: %s", opt.URL, nav.ErrorText)
	}
	if _, err := c.WaitEvent(ctx, s, "Page.loadEventFired"); err != nil {
		return nil, err
	}
	var shot struct {
		Data string `json:"data"`
	}
	if err := c.Call(ctx, s, "Page.captureScreenshot", map[string]any{"format": "png"}, &shot); err != nil {
		return nil, err
	}
	if shot.Data == "" {
		return nil, errors.New("Page.captureScreenshot returned no data")
	}
	return base64.StdEncoding.DecodeString(shot.Data)
}
