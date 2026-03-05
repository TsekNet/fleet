package mysql

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotifications(t *testing.T) {
	ds := CreateMySQLDS(t)

	cases := []struct {
		name string
		fn   func(t *testing.T, ds *Datastore)
	}{
		{"BatchSetNotifications", testBatchSetNotifications},
		{"GetNotificationByNotificationID", testGetNotificationByNotificationID},
		{"GetNotificationContents", testGetNotificationContents},
		{"ListPendingNotificationsForHost", testListPendingNotificationsForHost},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer TruncateTables(t, ds)
			c.fn(t, ds)
		})
	}
}

func testBatchSetNotifications(t *testing.T, ds *Datastore) {
	ctx := context.Background()

	config1 := json.RawMessage(`{"heading":"Restart Required","message":"Please restart"}`)
	config2 := json.RawMessage(`{"heading":"Update Available","message":"New update ready"}`)

	results, err := ds.BatchSetNotifications(ctx, nil, []*fleet.NotificationPayload{
		{NotificationID: "restart-required", Config: config1},
		{NotificationID: "update-available", Config: config2},
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "restart-required", results[0].NotificationID)
	assert.Equal(t, "update-available", results[1].NotificationID)

	results2, err := ds.BatchSetNotifications(ctx, nil, []*fleet.NotificationPayload{
		{NotificationID: "restart-required", Config: config1},
	})
	require.NoError(t, err)
	require.Len(t, results2, 1)
	assert.Equal(t, "restart-required", results2[0].NotificationID)

	results3, err := ds.BatchSetNotifications(ctx, nil, []*fleet.NotificationPayload{})
	require.NoError(t, err)
	require.Len(t, results3, 0)
}

func testGetNotificationByNotificationID(t *testing.T, ds *Datastore) {
	ctx := context.Background()

	config := json.RawMessage(`{"heading":"Test","message":"test message"}`)
	_, err := ds.BatchSetNotifications(ctx, nil, []*fleet.NotificationPayload{
		{NotificationID: "test-notif", Config: config},
	})
	require.NoError(t, err)

	n, err := ds.GetNotificationByNotificationID(ctx, "test-notif", nil)
	require.NoError(t, err)
	assert.Equal(t, "test-notif", n.NotificationID)
	assert.NotZero(t, n.NotificationContentID)

	_, err = ds.GetNotificationByNotificationID(ctx, "nonexistent", nil)
	require.Error(t, err)
	assert.True(t, fleet.IsNotFound(err))
}

func testGetNotificationContents(t *testing.T, ds *Datastore) {
	ctx := context.Background()

	config := json.RawMessage(`{"heading":"Content Test","message":"content body"}`)
	_, err := ds.BatchSetNotifications(ctx, nil, []*fleet.NotificationPayload{
		{NotificationID: "content-test", Config: config},
	})
	require.NoError(t, err)

	n, err := ds.GetNotificationByNotificationID(ctx, "content-test", nil)
	require.NoError(t, err)

	contents, err := ds.GetNotificationContents(ctx, n.NotificationContentID)
	require.NoError(t, err)
	assert.JSONEq(t, string(config), string(contents))

	_, err = ds.GetNotificationContents(ctx, 99999)
	require.Error(t, err)
	assert.True(t, fleet.IsNotFound(err))
}

func testListPendingNotificationsForHost(t *testing.T, ds *Datastore) {
	ctx := context.Background()

	ids, err := ds.ListPendingNotificationsForHost(ctx, 1)
	require.NoError(t, err)
	assert.Empty(t, ids)
}
