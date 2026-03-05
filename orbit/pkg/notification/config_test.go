package notification

import (
	"encoding/json"
	"strings"
	"testing"

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

func TestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "missing heading", cfg: Config{Message: "M"}, wantErr: `"heading" is required`},
		{name: "missing message", cfg: Config{Heading: "H"}, wantErr: `"message" is required`},
		{name: "valid minimal", cfg: Config{Heading: "Restart Required", Message: "Please restart."}},
		{name: "invalid dnd", cfg: Config{Heading: "H", Message: "M", DND: "invalid"}, wantErr: `"dnd" must be`},
		{name: "too many images", cfg: Config{Heading: "H", Message: "M", Images: make([]string, 21)}, wantErr: "images: 21 exceeds maximum of 20"},
		{name: "http image rejected", cfg: Config{Heading: "H", Message: "M", Images: []string{"http://example.com/img.png"}}, wantErr: "must be an https URL"},
		{name: "svg data uri rejected", cfg: Config{Heading: "H", Message: "M", Images: []string{"data:image/svg+xml;base64,abc"}}, wantErr: "SVG data URIs are not allowed"},
		{name: "raster data uri accepted", cfg: Config{Heading: "H", Message: "M", Images: []string{"data:image/png;base64,abc"}}},
		{name: "path traversal rejected", cfg: Config{Heading: "H", Message: "M", WatchPaths: []string{"/etc/../shadow"}}, wantErr: "path traversal"},
		{name: "dotdot in filename allowed", cfg: Config{Heading: "H", Message: "M", WatchPaths: []string{"/tmp/foo..bar"}}},
		{name: "too many watch paths", cfg: Config{Heading: "H", Message: "M", WatchPaths: make([]string, 11)}, wantErr: "watch_paths: 11 exceeds maximum of 10"},
		{name: "button newline rejected", cfg: Config{Heading: "H", Message: "M", Buttons: []Button{{Label: "OK", Value: "val\nue"}}}, wantErr: "button values must not contain newlines"},
		{name: "dropdown newline rejected", cfg: Config{Heading: "H", Message: "M", Buttons: []Button{{Label: "D", Dropdown: []DropdownOption{{Label: "x", Value: "v\n"}}}}}, wantErr: "dropdown values must not contain newlines"},
		{name: "html escaped", cfg: Config{Heading: "<script>alert(1)</script>", Message: "M"}},
		{name: "javascript helpurl rejected", cfg: Config{Heading: "H", Message: "M", HelpURL: "javascript:alert(1)"}, wantErr: `"help_url" must be an http or https URL`},
		{name: "https helpurl accepted", cfg: Config{Heading: "H", Message: "M", HelpURL: "https://example.com/help"}},
		{name: "invalid accent color", cfg: Config{Heading: "H", Message: "M", AccentColor: "red"}, wantErr: `"accent_color" must be a hex color`},
		{name: "valid accent color", cfg: Config{Heading: "H", Message: "M", AccentColor: "#76B900"}},
		{name: "priority out of range", cfg: Config{Heading: "H", Message: "M", Priority: 11}, wantErr: `"priority" must be 0-10`},
		{name: "valid priority", cfg: Config{Heading: "H", Message: "M", Priority: 10}},
		{name: "escalation after_defers < 1", cfg: Config{Heading: "H", Message: "M", Escalation: []EscalationStep{{AfterDefers: 0}}}, wantErr: "after_defers must be >= 1"},
		{name: "self-referencing depends_on", cfg: Config{Heading: "H", Message: "M", ID: "a", DependsOn: "a"}, wantErr: "must not reference the notification's own ID"},
		{name: "bad result_actions prefix", cfg: Config{Heading: "H", Message: "M", ResultActions: map[string]string{"x": "ftp://evil"}}, wantErr: "result_actions"},
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
		Priority:       8,
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
		Escalation: []EscalationStep{
			{AfterDefers: 3, Timeout: 120, AccentColor: "#FF0000", MessageSuffix: "\n\nThis is urgent."},
		},
		QuietHours: &QuietHours{Start: "22:00", End: "07:00", Timezone: "America/Los_Angeles"},
	}

	data, err := json.Marshal(cfg)
	require.NoError(t, err)

	parsed, err := LoadJSON(data)
	require.NoError(t, err)
	assert.Equal(t, cfg.Heading, parsed.Heading)
	assert.Equal(t, cfg.TimeoutSeconds, parsed.TimeoutSeconds)
	assert.Equal(t, cfg.Priority, parsed.Priority)
	assert.Len(t, parsed.Buttons, 2)
	assert.Len(t, parsed.Buttons[1].Dropdown, 2)
	assert.Len(t, parsed.Escalation, 1)
	assert.NotNil(t, parsed.QuietHours)

	assert.Equal(t, "timeout_value", jsonTag(t, data, "defer_1h"))
}

func jsonTag(t *testing.T, data []byte, value string) string {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &m))
	for k, v := range m {
		if string(v) == `"`+value+`"` {
			return k
		}
	}
	return ""
}
