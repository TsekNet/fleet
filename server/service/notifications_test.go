package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/mock"
	"github.com/fleetdm/fleet/v4/server/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOrbitNotificationConfig(t *testing.T) {
	ds := new(mock.DataStore)
	svc, ctx := newTestService(t, ds, nil, nil)

	host := &fleet.Host{ID: 1, TeamID: nil}
	ctx = test.HostContext(ctx, host)

	expectedConfig := json.RawMessage(`{"heading":"Test","message":"hello"}`)

	ds.GetNotificationByNotificationIDFunc = func(ctx context.Context, notificationID string, teamID *uint) (*fleet.Notification, error) {
		return &fleet.Notification{
			ID:                    1,
			NotificationID:        notificationID,
			NotificationContentID: 42,
		}, nil
	}
	ds.GetNotificationContentsFunc = func(ctx context.Context, notificationContentID uint) ([]byte, error) {
		return expectedConfig, nil
	}

	config, err := svc.GetOrbitNotificationConfig(ctx, "restart-required")
	require.NoError(t, err)
	assert.JSONEq(t, string(expectedConfig), string(config))
	assert.True(t, ds.GetNotificationByNotificationIDFuncInvoked)
	assert.True(t, ds.GetNotificationContentsFuncInvoked)
}

func TestSaveOrbitNotificationResult(t *testing.T) {
	ds := new(mock.DataStore)
	svc, ctx := newTestService(t, ds, nil, nil)

	host := &fleet.Host{ID: 1}
	ctx = test.HostContext(ctx, host)

	ds.SetHostNotificationResultFunc = func(ctx context.Context, result *fleet.HostNotificationResultPayload) error {
		assert.Equal(t, uint(1), result.HostID)
		assert.Equal(t, "restart-required", result.NotificationID)
		assert.Equal(t, "accept", result.Result)
		assert.Equal(t, 0, result.ExitCode)
		return nil
	}

	err := svc.SaveOrbitNotificationResult(ctx, "restart-required", "accept", 0)
	require.NoError(t, err)
	assert.True(t, ds.SetHostNotificationResultFuncInvoked)
}
