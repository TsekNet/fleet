package notification

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Config mirrors the hermes NotificationConfig schema. JSON tags use
// snake_case to match hermes's native format — orbit writes this JSON
// directly and hermes reads it without translation.
type Config struct {
	Heading        string            `json:"heading"`
	Message        string            `json:"message,omitempty"`
	Buttons        []Button          `json:"buttons,omitempty"`
	TimeoutSeconds int               `json:"timeout,omitempty"`
	TimeoutValue   string            `json:"timeout_value,omitempty"`
	EscValue       string            `json:"esc_value,omitempty"`
	Title          string            `json:"title,omitempty"`
	AccentColor    string            `json:"accent_color,omitempty"`
	HelpURL        string            `json:"help_url,omitempty"`
	Platform       string            `json:"platform,omitempty"`
	ID             string            `json:"id,omitempty"`
	DeferDeadline  string            `json:"defer_deadline,omitempty"`
	MaxDefers      int               `json:"max_defers,omitempty"`
	Images         []string          `json:"images,omitempty"`
	WatchPaths     []string          `json:"watch_paths,omitempty"`
	DND            string            `json:"dnd,omitempty"`
	Priority       int               `json:"priority,omitempty"`
	Escalation     []EscalationStep  `json:"escalation,omitempty"`
	ResultActions  map[string]string `json:"result_actions,omitempty"`
	QuietHours     *QuietHours       `json:"quiet_hours,omitempty"`

	HeadingLocalized map[string]string `json:"heading_localized,omitempty"`
	MessageLocalized map[string]string `json:"message_localized,omitempty"`

	DependsOn string `json:"depends_on,omitempty"`
}

// EscalationStep defines a mutation applied after repeated deferrals.
type EscalationStep struct {
	AfterDefers   int    `json:"after_defers"`
	Timeout       int    `json:"timeout,omitempty"`
	AccentColor   string `json:"accent_color,omitempty"`
	MessageSuffix string `json:"message_suffix,omitempty"`
}

// QuietHours defines a daily window during which notifications are delayed.
type QuietHours struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone,omitempty"`
}

// Button represents a clickable action in the notification.
type Button struct {
	Label    string           `json:"label"`
	Value    string           `json:"value,omitempty"`
	Style    string           `json:"style"`
	Dropdown []DropdownOption `json:"dropdown,omitempty"`
}

// DropdownOption is one item in a button's dropdown menu.
type DropdownOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

const (
	DNDRespect = "respect"
	DNDIgnore  = "ignore"
	DNDSkip    = "skip"

	MaxConfigSize = 64 * 1024
)

var (
	accentColorRe = regexp.MustCompile(`^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`)
	safeIDRe      = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	validDND      = map[string]bool{DNDRespect: true, DNDIgnore: true, DNDSkip: true}
)

// validateValue checks a value string for newlines. Returns an error
// message or empty string. Does NOT escape — values must match
// result_actions keys verbatim.
func validateValue(s *string, prefix string) string {
	if strings.ContainsAny(*s, "\n\r") {
		return prefix + " values must not contain newlines"
	}
	return ""
}

// LoadJSON parses raw JSON bytes into a Config.
func LoadJSON(data []byte) (*Config, error) {
	data = trimLeadingWhitespace(data)
	if len(data) == 0 {
		return nil, errors.New("empty config")
	}
	if len(data) > MaxConfigSize {
		return nil, fmt.Errorf("config too large: %d bytes (max %d)", len(data), MaxConfigSize)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config JSON: %w", err)
	}
	return &cfg, nil
}

// escapeOnce applies html.EscapeString idempotently by unescaping first.
func escapeOnce(s string) string {
	return html.EscapeString(html.UnescapeString(s))
}

// Validate checks that required fields are present and values are safe.
// Safe to call multiple times (HTML escaping is idempotent).
func (c *Config) Validate() error {
	c.Heading = escapeOnce(c.Heading)
	c.Message = escapeOnce(c.Message)
	c.Title = escapeOnce(c.Title)

	var errs []string
	if strings.TrimSpace(c.Heading) == "" {
		errs = append(errs, `"heading" is required`)
	}
	if strings.TrimSpace(c.Message) == "" {
		errs = append(errs, `"message" is required`)
	}
	if c.HelpURL != "" {
		u, err := url.Parse(c.HelpURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			errs = append(errs, `"help_url" must be an http or https URL`)
		}
	}
	if c.AccentColor != "" && !accentColorRe.MatchString(c.AccentColor) {
		errs = append(errs, `"accent_color" must be a hex color (e.g. #D4A843)`)
	}
	for i := range c.Buttons {
		c.Buttons[i].Label = escapeOnce(c.Buttons[i].Label)
		if err := validateValue(&c.Buttons[i].Value, "button"); err != "" {
			errs = append(errs, err)
		}
		for j := range c.Buttons[i].Dropdown {
			c.Buttons[i].Dropdown[j].Label = escapeOnce(c.Buttons[i].Dropdown[j].Label)
			if err := validateValue(&c.Buttons[i].Dropdown[j].Value, "dropdown"); err != "" {
				errs = append(errs, err)
			}
		}
	}
	if c.DND != "" && !validDND[c.DND] {
		errs = append(errs, fmt.Sprintf(`"dnd" must be %q, %q, or %q`, DNDRespect, DNDIgnore, DNDSkip))
	}
	if c.Priority < 0 || c.Priority > 10 {
		errs = append(errs, fmt.Sprintf(`"priority" must be 0-10, got %d`, c.Priority))
	}
	if len(c.Images) > 20 {
		errs = append(errs, fmt.Sprintf("images: %d exceeds maximum of 20", len(c.Images)))
	}
	for i, img := range c.Images {
		lower := strings.ToLower(img)
		switch {
		case strings.HasPrefix(lower, "data:image/svg"):
			errs = append(errs, fmt.Sprintf("images[%d]: SVG data URIs are not allowed", i))
		case strings.HasPrefix(lower, "data:image/"):
			// valid raster data URI
		default:
			u, err := url.Parse(img)
			if err != nil || u.Scheme != "https" {
				errs = append(errs, fmt.Sprintf("images[%d]: must be an https URL or data:image/ URI", i))
			}
		}
	}
	if len(c.WatchPaths) > 10 {
		errs = append(errs, fmt.Sprintf("watch_paths: %d exceeds maximum of 10", len(c.WatchPaths)))
	}
	for i, p := range c.WatchPaths {
		for _, part := range strings.Split(filepath.ToSlash(p), "/") {
			if part == ".." {
				errs = append(errs, fmt.Sprintf("watch_paths[%d]: path traversal (..) is not allowed", i))
				break
			}
		}
	}
	for i, step := range c.Escalation {
		if step.AfterDefers < 1 {
			errs = append(errs, fmt.Sprintf("escalation[%d]: after_defers must be >= 1", i))
		}
	}
	for k, v := range c.ResultActions {
		if strings.ContainsAny(k, "\n\r") {
			errs = append(errs, fmt.Sprintf("result_actions key %q: must not contain newlines", k))
		}
		lower := strings.ToLower(v)
		if !strings.HasPrefix(lower, "cmd:") && !strings.HasPrefix(lower, "url:") &&
			!strings.HasPrefix(lower, "https:") && !strings.HasPrefix(lower, "http:") {
			errs = append(errs, fmt.Sprintf("result_actions[%q]: value must start with cmd:, url:, https:, or http:", k))
		}
	}
	if c.DependsOn != "" && c.DependsOn == c.ID {
		errs = append(errs, `"depends_on" must not reference the notification's own ID`)
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// ValidateID checks that a notification ID is safe for use in file paths.
func ValidateID(id string) error {
	if id == "" {
		return errors.New("notification ID is empty")
	}
	if !safeIDRe.MatchString(id) {
		return fmt.Errorf("notification ID %q contains invalid characters", id)
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("notification ID %q contains path traversal", id)
	}
	return nil
}

func trimLeadingWhitespace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\n' || b[0] == '\r') {
		b = b[1:]
	}
	return b
}
