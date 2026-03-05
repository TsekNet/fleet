package main

import (
	"testing"

	"github.com/fleetdm/fleet/v4/orbit/pkg/notification"
	"github.com/stretchr/testify/assert"
)

func TestHeadlessResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *notification.Config
		want string
	}{
		{
			name: "primary button",
			cfg: &notification.Config{
				Buttons: []notification.Button{
					{Label: "Defer", Style: "secondary", Value: "defer_1h"},
					{Label: "Restart", Style: "primary", Value: "restart"},
				},
			},
			want: "restart",
		},
		{
			name: "timeout value",
			cfg: &notification.Config{
				TimeoutValue: "timeout_restart",
			},
			want: "timeout_restart",
		},
		{
			name: "first button fallback",
			cfg: &notification.Config{
				Buttons: []notification.Button{
					{Label: "OK", Style: "secondary", Value: "ok"},
				},
			},
			want: "ok",
		},
		{
			name: "no buttons no timeout",
			cfg:  &notification.Config{},
			want: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, headlessResult(tt.cfg))
		})
	}
}
