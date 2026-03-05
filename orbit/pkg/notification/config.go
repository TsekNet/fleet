package notification

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Config holds all parameters for a notification dialog shown to the end user.
// Only Heading is required; all other fields have sensible defaults applied by
// ApplyDefaults.
type Config struct {
	Heading        string   `json:"heading"`
	Message        string   `json:"message,omitempty"`
	Buttons        []Button `json:"buttons,omitempty"`
	TimeoutSeconds int      `json:"timeout,omitempty"`
	TimeoutValue   string   `json:"timeout_value,omitempty"`
	EscValue       string   `json:"esc_value,omitempty"`
	Title          string   `json:"title,omitempty"`
	AccentColor    string   `json:"accent_color,omitempty"`
	HelpURL        string   `json:"help_url,omitempty"`
	ID             string   `json:"id,omitempty"`
	DeferDeadline  string   `json:"defer_deadline,omitempty"`
	MaxDefers      int      `json:"max_defers,omitempty"`
	Images         []string `json:"images,omitempty"`
	WatchPaths     []string `json:"watch_paths,omitempty"`
	DND            string   `json:"dnd,omitempty"`
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

	defaultTimeoutSeconds = 300
	defaultTitle          = "IT Department"
	defaultAccentColor    = "#D4A843"
)

var (
	deferRe         = regexp.MustCompile(`^defer_(\d+)([hdms])$`)
	deadlineDaysRe  = regexp.MustCompile(`^(\d+)d$`)
	accentColorRe   = regexp.MustCompile(`^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{6})$`)
	safeIDRe        = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

	deferMaxSafe = map[string]int{
		"s": 9_000_000_000,
		"m": 150_000_000,
		"h": 2_500_000,
		"d": 100_000,
	}

	unitToDuration = map[string]time.Duration{
		"s": time.Second,
		"m": time.Minute,
		"h": time.Hour,
		"d": 24 * time.Hour,
	}
)

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

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = defaultTimeoutSeconds
	}
	if c.Title == "" {
		c.Title = defaultTitle
	}
	if c.AccentColor == "" {
		c.AccentColor = defaultAccentColor
	}
	if c.EscValue == "" && c.TimeoutValue != "" {
		c.EscValue = c.TimeoutValue
	}
	if c.DND == "" {
		c.DND = DNDRespect
	}
	for i := range c.Buttons {
		if c.Buttons[i].Style == "" {
			c.Buttons[i].Style = "secondary"
		}
		if c.Buttons[i].Value == "" && len(c.Buttons[i].Dropdown) == 0 {
			c.Buttons[i].Value = strings.ToLower(strings.ReplaceAll(c.Buttons[i].Label, " ", "_"))
		}
	}
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
		c.Buttons[i].Value = escapeOnce(c.Buttons[i].Value)
		if strings.ContainsAny(c.Buttons[i].Value, "\n\r") {
			errs = append(errs, "button values must not contain newlines")
		}
		for j := range c.Buttons[i].Dropdown {
			c.Buttons[i].Dropdown[j].Label = escapeOnce(c.Buttons[i].Dropdown[j].Label)
			c.Buttons[i].Dropdown[j].Value = escapeOnce(c.Buttons[i].Dropdown[j].Value)
			if strings.ContainsAny(c.Buttons[i].Dropdown[j].Value, "\n\r") {
				errs = append(errs, "dropdown values must not contain newlines")
			}
		}
	}
	if c.DND != "" && c.DND != DNDRespect && c.DND != DNDIgnore && c.DND != DNDSkip {
		errs = append(errs, fmt.Sprintf(`"dnd" must be %q, %q, or %q`, DNDRespect, DNDIgnore, DNDSkip))
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

// ParseDeferValue extracts the duration from a defer response value like
// "defer_4h", "defer_1d", "defer_30m", "defer_30s".
func ParseDeferValue(value string) time.Duration {
	m := deferRe.FindStringSubmatch(value)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	if n > deferMaxSafe[m[2]] {
		return 0
	}
	return time.Duration(n) * unitToDuration[m[2]]
}

// ParseDeadline parses a DeferDeadline string like "24h", "7d" into a
// time.Duration.
func ParseDeadline(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	m := deadlineDaysRe.FindStringSubmatch(s)
	if m != nil {
		n, _ := strconv.Atoi(m[1])
		if n > 100000 {
			return 0
		}
		return time.Duration(n) * 24 * time.Hour
	}
	return 0
}

func trimLeadingWhitespace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\n' || b[0] == '\r') {
		b = b[1:]
	}
	return b
}
