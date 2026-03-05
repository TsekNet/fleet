package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationConfigReceiver_NilConfig(t *testing.T) {
	t.Parallel()
	r := ApplyNotificationConfigReceiverMiddleware(NotificationReceiverOptions{})
	require.NoError(t, r.Run(nil))
}

func TestNotificationConfigReceiver_NoNotifications(t *testing.T) {
	t.Parallel()
	r := ApplyNotificationConfigReceiverMiddleware(NotificationReceiverOptions{})
	cfg := &fleet.OrbitConfig{}
	require.NoError(t, r.Run(cfg))
}

func TestNotificationConfigReceiver_NilUpdateRunner(t *testing.T) {
	t.Parallel()
	r := ApplyNotificationConfigReceiverMiddleware(NotificationReceiverOptions{
		UpdateRunner: nil,
	})
	cfg := &fleet.OrbitConfig{
		Notifications: fleet.OrbitConfigNotifications{
			PendingNotificationIDs: []string{"notif-1"},
		},
	}
	require.NoError(t, r.Run(cfg))
}

func TestNotificationConfigReceiver_SkipsDuringSetupExperience(t *testing.T) {
	t.Parallel()
	r := ApplyNotificationConfigReceiverMiddleware(NotificationReceiverOptions{})
	cfg := &fleet.OrbitConfig{
		Notifications: fleet.OrbitConfigNotifications{
			RunSetupExperience:     true,
			PendingNotificationIDs: []string{"notif-1"},
		},
	}
	require.NoError(t, r.Run(cfg))
}

func TestNotificationConfigReceiver_Configure(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	receiver := &NotificationConfigReceiver{
		opt: NotificationReceiverOptions{
			RootDir: tmpDir,
		},
	}

	testPayload := json.RawMessage(`{"heading":"Restart Required","timeout":300}`)
	err := receiver.configure("test-123", testPayload)
	require.NoError(t, err)

	expectedPath := filepath.Join(tmpDir, "notification-test-123.json")
	data, err := os.ReadFile(expectedPath)
	require.NoError(t, err)
	assert.JSONEq(t, `{"heading":"Restart Required","timeout":300}`, string(data))

	info, err := os.Stat(expectedPath)
	require.NoError(t, err)
	assert.Equal(t, notificationConfigFileMode, info.Mode())
}

func TestNotificationConfigReceiver_ConfigureIdempotent(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	receiver := &NotificationConfigReceiver{
		opt: NotificationReceiverOptions{
			RootDir: tmpDir,
		},
	}

	payload := json.RawMessage(`{"heading":"Test"}`)

	err := receiver.configure("idem", payload)
	require.NoError(t, err)

	path := filepath.Join(tmpDir, "notification-idem.json")
	info1, err := os.Stat(path)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	err = receiver.configure("idem", payload)
	require.NoError(t, err)

	info2, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, info1.ModTime(), info2.ModTime())
}

func TestNotificationConfigReceiver_ConfigureUpdatesOnChange(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	receiver := &NotificationConfigReceiver{
		opt: NotificationReceiverOptions{
			RootDir: tmpDir,
		},
	}

	err := receiver.configure("change", json.RawMessage(`{"heading":"V1"}`))
	require.NoError(t, err)

	err = receiver.configure("change", json.RawMessage(`{"heading":"V2"}`))
	require.NoError(t, err)

	path := filepath.Join(tmpDir, "notification-change.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"heading":"V2"}`, string(data))
}

func TestNotificationConfigReceiver_IdempotentActiveNotification(t *testing.T) {
	t.Parallel()

	receiver := &NotificationConfigReceiver{
		opt: NotificationReceiverOptions{},
	}

	receiver.cmdMu.Lock()
	receiver.activeNotificationID = "notif-1"
	receiver.cmdMu.Unlock()

	cfg := &fleet.OrbitConfig{
		Notifications: fleet.OrbitConfigNotifications{
			PendingNotificationIDs: []string{"notif-1"},
		},
	}

	err := receiver.Run(cfg)
	require.NoError(t, err)
}

func TestNotificationConfigReceiver_LaunchRateLimit(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	receiver := &NotificationConfigReceiver{
		opt: NotificationReceiverOptions{
			RootDir:  tmpDir,
			Interval: 1 * time.Hour,
		},
	}

	receiver.cmdMu.Lock()
	receiver.lastRun = time.Now()
	receiver.cmdMu.Unlock()

	err := receiver.launch("test")
	require.NoError(t, err)
}

func TestHermesTargetForPlatform(t *testing.T) {
	t.Parallel()

	target := hermesTargetForPlatform()
	switch runtime.GOOS {
	case "windows":
		assert.Equal(t, hermesWindowsTarget, target)
	case "darwin":
		assert.Equal(t, hermesMacOSTarget, target)
	default:
		assert.Equal(t, hermesLinuxTarget, target)
	}
}

func TestHermesTargetDefinitions(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "windows", hermesWindowsTarget.Platform)
	assert.Equal(t, "hermes.exe", hermesWindowsTarget.TargetFile)

	assert.Equal(t, "linux", hermesLinuxTarget.Platform)
	assert.Equal(t, "hermes", hermesLinuxTarget.TargetFile)

	assert.Equal(t, "macos", hermesMacOSTarget.Platform)
	assert.Equal(t, "hermes.app.tar.gz", hermesMacOSTarget.TargetFile)
	assert.Equal(t, []string{"hermes.app", "Contents", "MacOS", "hermes"}, hermesMacOSTarget.ExtractedExecSubPath)
}
