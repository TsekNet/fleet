// fleet-notification is a standalone binary that displays a notification
// dialog to the end user and reports the result on stdout. It is managed
// as a TUF target and launched by orbit via execuser in the user's session.
//
// In production this binary uses Wails + WebView2 to render a rich webview
// UI. This prototype uses a headless mode (--headless) for testing on
// systems without a display server (e.g. WSL).
//
// Usage:
//
//	fleet-notification --config /path/to/notification.json [--headless]
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/fleetdm/fleet/v4/orbit/pkg/notification"
)

func main() {
	configPath := flag.String("config", "", "path to notification config JSON file")
	headless := flag.Bool("headless", false, "headless mode: validate config and print first button value without UI")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config is required")
		os.Exit(2)
	}

	info, err := os.Stat(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading config: %v\n", err)
		os.Exit(2)
	}
	if info.Size() > notification.MaxConfigSize {
		fmt.Fprintf(os.Stderr, "error: config file too large: %d bytes (max %d)\n", info.Size(), notification.MaxConfigSize)
		os.Exit(2)
	}

	data, err := os.ReadFile(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading config: %v\n", err)
		os.Exit(2)
	}

	cfg, err := notification.LoadJSON(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing config: %v\n", err)
		os.Exit(2)
	}

	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "error validating config: %v\n", err)
		os.Exit(2)
	}

	if *headless {
		fmt.Print(headlessResult(cfg))
		return
	}

	fmt.Fprintf(os.Stderr, "webview mode not available in this build, use --headless\n")
	fmt.Print(headlessResult(cfg))
}

// headlessResult determines the result value without user interaction.
// It returns the first primary button's value, or the timeout value, or "ok".
func headlessResult(cfg *notification.Config) string {
	for _, btn := range cfg.Buttons {
		if btn.Style == "primary" {
			return btn.Value
		}
	}
	if cfg.TimeoutValue != "" {
		return cfg.TimeoutValue
	}
	if len(cfg.Buttons) > 0 {
		return cfg.Buttons[0].Value
	}
	return "ok"
}
