package notification

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "empty", input: "", wantErr: "empty config"},
		{name: "whitespace only", input: "   \n\t  ", wantErr: "empty config"},
		{name: "invalid json", input: "{bad", wantErr: "parse config JSON"},
		{name: "minimal valid", input: `{"heading": "Restart Required"}`},
		{name: "too large", input: `{"heading":"` + strings.Repeat("x", MaxConfigSize) + `"}`, wantErr: "config too large"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := LoadJSON([]byte(tt.input))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, cfg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
			}
		})
	}
}

func TestApplyDefaults(t *testing.T) {
	t.Parallel()

	cfg := &Config{Heading: "Test"}
	cfg.ApplyDefaults()

	assert.Equal(t, defaultTimeoutSeconds, cfg.TimeoutSeconds)
	assert.Equal(t, defaultTitle, cfg.Title)
	assert.Equal(t, defaultAccentColor, cfg.AccentColor)
	assert.Equal(t, DNDRespect, cfg.DND)
}

func TestApplyDefaultsPreservesExisting(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Heading:        "Test",
		TimeoutSeconds: 60,
		Title:          "Custom Title",
		AccentColor:    "#76B900",
		DND:            DNDIgnore,
	}
	cfg.ApplyDefaults()

	assert.Equal(t, 60, cfg.TimeoutSeconds)
	assert.Equal(t, "Custom Title", cfg.Title)
	assert.Equal(t, "#76B900", cfg.AccentColor)
	assert.Equal(t, DNDIgnore, cfg.DND)
}

func TestApplyDefaultsEscValueFallback(t *testing.T) {
	t.Parallel()

	cfg := &Config{Heading: "Test", TimeoutValue: "defer_1h"}
	cfg.ApplyDefaults()
	assert.Equal(t, "defer_1h", cfg.EscValue)

	cfg2 := &Config{Heading: "Test", TimeoutValue: "defer_1h", EscValue: "dismiss"}
	cfg2.ApplyDefaults()
	assert.Equal(t, "dismiss", cfg2.EscValue)
}

func TestApplyDefaultsButtonStyle(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Heading: "Test",
		Buttons: []Button{
			{Label: "OK"},
			{Label: "Cancel", Style: "danger"},
			{Label: "Defer", Dropdown: []DropdownOption{{Label: "1h", Value: "defer_1h"}}},
		},
	}
	cfg.ApplyDefaults()

	assert.Equal(t, "secondary", cfg.Buttons[0].Style)
	assert.Equal(t, "ok", cfg.Buttons[0].Value)
	assert.Equal(t, "danger", cfg.Buttons[1].Style)
	assert.Equal(t, "cancel", cfg.Buttons[1].Value)
	assert.Equal(t, "secondary", cfg.Buttons[2].Style)
	assert.Empty(t, cfg.Buttons[2].Value)
}

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "missing heading", cfg: Config{}, wantErr: `"heading" is required`},
		{name: "valid minimal", cfg: Config{Heading: "Restart Required"}},
		{name: "invalid dnd", cfg: Config{Heading: "Test", DND: "invalid"}, wantErr: `"dnd" must be`},
		{name: "too many images", cfg: Config{Heading: "Test", Images: make([]string, 21)}, wantErr: "images: 21 exceeds maximum of 20"},
		{name: "http image rejected", cfg: Config{Heading: "Test", Images: []string{"http://example.com/img.png"}}, wantErr: "must be an https URL"},
		{name: "svg data uri rejected", cfg: Config{Heading: "Test", Images: []string{"data:image/svg+xml;base64,abc"}}, wantErr: "SVG data URIs are not allowed"},
		{name: "raster data uri accepted", cfg: Config{Heading: "Test", Images: []string{"data:image/png;base64,abc"}}},
		{name: "path traversal rejected", cfg: Config{Heading: "Test", WatchPaths: []string{"/etc/../shadow"}}, wantErr: "path traversal"},
		{name: "dotdot in filename allowed", cfg: Config{Heading: "Test", WatchPaths: []string{"/tmp/foo..bar"}}},
		{name: "too many watch paths", cfg: Config{Heading: "Test", WatchPaths: make([]string, 11)}, wantErr: "watch_paths: 11 exceeds maximum of 10"},
		{name: "button newline rejected", cfg: Config{Heading: "Test", Buttons: []Button{{Label: "OK", Value: "val\nue"}}}, wantErr: "button values must not contain newlines"},
		{name: "dropdown newline rejected", cfg: Config{Heading: "Test", Buttons: []Button{{Label: "D", Dropdown: []DropdownOption{{Label: "x", Value: "v\n"}}}}}, wantErr: "dropdown values must not contain newlines"},
		{name: "html escaped", cfg: Config{Heading: "<script>alert(1)</script>"}},
		{name: "javascript helpurl rejected", cfg: Config{Heading: "Test", HelpURL: "javascript:alert(1)"}, wantErr: `"help_url" must be an http or https URL`},
		{name: "https helpurl accepted", cfg: Config{Heading: "Test", HelpURL: "https://example.com/help"}},
		{name: "invalid accent color", cfg: Config{Heading: "Test", AccentColor: "red"}, wantErr: `"accent_color" must be a hex color`},
		{name: "valid accent color", cfg: Config{Heading: "Test", AccentColor: "#76B900"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateHTMLEscapingIdempotent(t *testing.T) {
	t.Parallel()

	cfg := Config{Heading: "<b>bold</b>", Message: "<script>xss</script>"}
	require.NoError(t, cfg.Validate())
	assert.Equal(t, "&lt;b&gt;bold&lt;/b&gt;", cfg.Heading)
	assert.Equal(t, "&lt;script&gt;xss&lt;/script&gt;", cfg.Message)

	require.NoError(t, cfg.Validate())
	assert.Equal(t, "&lt;b&gt;bold&lt;/b&gt;", cfg.Heading, "double-escape must not occur")
}

func TestValidateID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id      string
		wantErr bool
	}{
		{"notif-123", false},
		{"restart_required.v2", false},
		{"", true},
		{"../etc/passwd", true},
		{"foo/bar", true},
		{"foo\\bar", true},
		{"notif 1", true},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			err := ValidateID(tt.id)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParseDeferValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  time.Duration
	}{
		{"defer_4h", 4 * time.Hour},
		{"defer_1d", 24 * time.Hour},
		{"defer_30m", 30 * time.Minute},
		{"defer_30s", 30 * time.Second},
		{"defer", 0},
		{"restart", 0},
		{"", 0},
		{"defer_0h", 0},
		{"defer_100001d", 0},
		{"defer_2500001h", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ParseDeferValue(tt.input))
		})
	}
}

func TestParseDeadline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  time.Duration
	}{
		{"24h", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"30m", 30 * time.Minute},
		{"", 0},
		{"invalid", 0},
		{"100001d", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ParseDeadline(tt.input))
		})
	}
}

func TestConfigJSONRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Heading:        "Restart Required",
		Message:        "Please restart your computer.",
		TimeoutSeconds: 600,
		TimeoutValue:   "defer_1h",
		DeferDeadline:  "24h",
		MaxDefers:      3,
		AccentColor:    "#76B900",
		Buttons: []Button{
			{Label: "Restart Now", Value: "url:ms-settings:windowsupdate", Style: "primary"},
			{
				Label: "Defer", Style: "secondary",
				Dropdown: []DropdownOption{
					{Label: "1 Hour", Value: "defer_1h"},
					{Label: "4 Hours", Value: "defer_4h"},
				},
			},
		},
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	parsed, err := LoadJSON(data)
	require.NoError(t, err)
	assert.Equal(t, cfg.Heading, parsed.Heading)
	assert.Equal(t, cfg.TimeoutSeconds, parsed.TimeoutSeconds)
	assert.Len(t, parsed.Buttons, 2)
	assert.Len(t, parsed.Buttons[1].Dropdown, 2)
}
