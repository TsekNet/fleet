package update

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/fleetdm/fleet/v4/orbit/pkg/constant"
	"github.com/fleetdm/fleet/v4/orbit/pkg/execuser"
	"github.com/fleetdm/fleet/v4/orbit/pkg/notification"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/rs/zerolog/log"
)

const (
	notificationConfigFilePrefix = "notification-"
	notificationConfigFileMode   = os.FileMode(constant.DefaultWorldReadableFileMode)
)

var (
	notificationWindowsTarget = TargetInfo{
		Platform:   "windows",
		Channel:    "stable",
		TargetFile: "fleet-notification.exe",
	}

	notificationLinuxTarget = TargetInfo{
		Platform:   "linux",
		Channel:    "stable",
		TargetFile: "fleet-notification",
	}

	notificationMacOSTarget = TargetInfo{
		Platform:             "macos",
		Channel:              "stable",
		TargetFile:           "fleet-notification.app.tar.gz",
		ExtractedExecSubPath: []string{"fleet-notification.app", "Contents", "MacOS", "fleet-notification"},
	}
)

// notificationTargetForPlatform returns the TUF TargetInfo for the current OS.
func notificationTargetForPlatform() TargetInfo {
	switch runtime.GOOS {
	case "windows":
		return notificationWindowsTarget
	case "darwin":
		return notificationMacOSTarget
	default:
		return notificationLinuxTarget
	}
}

const notificationTargetName = "fleet-notification"

// NotificationConfigReceiver is an OrbitConfigReceiver that reacts to
// PendingNotificationIDs in the orbit config. When notification IDs are
// present, it ensures the notification UI binary is available via TUF,
// writes the notification config to a temp file, and launches the binary
// in the user's session via execuser.
type NotificationConfigReceiver struct {
	opt NotificationReceiverOptions

	// cmdMu ensures only one notification launches at a time and protects
	// access to lastRun and activeNotificationID.
	cmdMu   sync.Mutex
	lastRun time.Time

	// activeNotificationID tracks the currently-displayed notification to
	// prevent re-launching while it is still visible.
	activeNotificationID string

	launchErr *notificationLaunchErr
}

// NotificationReceiverOptions configures the NotificationConfigReceiver.
type NotificationReceiverOptions struct {
	// UpdateRunner manages TUF targets for the notification binary.
	UpdateRunner *Runner
	// RootDir is where notification config files are written.
	RootDir string
	// Interval is the minimum time between notification launches.
	Interval time.Duration
	// runNotificationFn can be set in tests to mock the notification binary.
	runNotificationFn func(execPath, configPath string) error
	// fetchConfigFn fetches the full notification config from the server by ID.
	// In production this calls the orbit API; in tests it can be mocked.
	fetchConfigFn func(notificationID string) (json.RawMessage, error)
}

// ApplyNotificationConfigReceiverMiddleware creates a new
// NotificationConfigReceiver with the given options.
func ApplyNotificationConfigReceiverMiddleware(opt NotificationReceiverOptions) fleet.OrbitConfigReceiver {
	return &NotificationConfigReceiver{opt: opt}
}

// Run implements fleet.OrbitConfigReceiver. It checks for pending notification
// IDs and launches the notification UI binary for the first pending one.
func (n *NotificationConfigReceiver) Run(cfg *fleet.OrbitConfig) error {
	log.Debug().Msg("running notification config receiver")

	if cfg == nil {
		log.Debug().Msg("NotificationConfigReceiver received nil config")
		return nil
	}

	if n.opt.UpdateRunner == nil {
		log.Debug().Msg("NotificationConfigReceiver received nil UpdateRunner, this probably indicates that updates are turned off. Skipping any actions related to notifications")
		return nil
	}

	if cfg.Notifications.RunSetupExperience {
		log.Debug().Msg("setup experience active, skipping notifications")
		return nil
	}

	ids := cfg.Notifications.PendingNotificationIDs
	if len(ids) == 0 {
		log.Debug().Msg("no pending notifications")
		n.cmdMu.Lock()
		n.activeNotificationID = ""
		n.cmdMu.Unlock()
		return nil
	}

	notificationID := ids[0]

	if err := notification.ValidateID(notificationID); err != nil {
		log.Debug().Err(err).Msg("invalid notification ID, skipping")
		return nil
	}

	n.cmdMu.Lock()
	if n.activeNotificationID == notificationID {
		n.cmdMu.Unlock()
		log.Debug().Str("notification_id", notificationID).Msg("notification already active, skipping")
		return nil
	}
	n.cmdMu.Unlock()

	updaterHasTarget := n.opt.UpdateRunner.HasRunnerOptTarget(notificationTargetName)
	runnerHasLocalHash := n.opt.UpdateRunner.HasLocalHash(notificationTargetName)
	if !updaterHasTarget || !runnerHasLocalHash {
		log.Info().Msg("refreshing the update runner config with notification binary targets and hashes")
		log.Debug().Msgf("updater has target: %t, runner has local hash: %t", updaterHasTarget, runnerHasLocalHash)
		return n.setTargetsAndHashes()
	}

	configData, err := n.fetchNotificationConfig(notificationID)
	if err != nil {
		return fmt.Errorf("fetching notification config %s: %w", notificationID, err)
	}

	if err := n.configure(notificationID, configData); err != nil {
		log.Info().Err(err).Str("notification_id", notificationID).Msg("notification configuration")
		return err
	}

	if err := n.launch(notificationID); err != nil {
		log.Info().Err(err).Str("notification_id", notificationID).Msg("notification launch")
		return err
	}

	return nil
}

func (n *NotificationConfigReceiver) setTargetsAndHashes() error {
	n.opt.UpdateRunner.AddRunnerOptTarget(notificationTargetName)
	n.opt.UpdateRunner.updater.SetTargetInfo(notificationTargetName, notificationTargetForPlatform())
	if err := n.opt.UpdateRunner.StoreLocalHash(notificationTargetName); err != nil {
		log.Debug().Msgf("removing %s from target options, error updating local hashes: %s", notificationTargetName, err)
		n.opt.UpdateRunner.RemoveRunnerOptTarget(notificationTargetName)
		n.opt.UpdateRunner.updater.RemoveTargetInfo(notificationTargetName)
		return err
	}
	return nil
}

func (n *NotificationConfigReceiver) fetchNotificationConfig(id string) (json.RawMessage, error) {
	if n.opt.fetchConfigFn != nil {
		return n.opt.fetchConfigFn(id)
	}
	return nil, errors.New("fetchConfigFn not configured")
}

func (n *NotificationConfigReceiver) configure(id string, data json.RawMessage) error {
	cfgFile := filepath.Join(n.opt.RootDir, notificationConfigFilePrefix+id+".json")
	writeConfig := func() error {
		return os.WriteFile(cfgFile, data, constant.DefaultWorldReadableFileMode)
	}

	fileInfo, err := os.Stat(cfgFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return writeConfig()
		}
		return err
	}

	if fileInfo.Mode() != notificationConfigFileMode {
		log.Info().Msgf("%s config file had wrong permissions (%v) setting permissions to %v", cfgFile, fileInfo.Mode(), notificationConfigFileMode)
		if err := os.Chmod(cfgFile, notificationConfigFileMode); err != nil {
			return fmt.Errorf("ensuring permissions of config file, chmod %q: %w", cfgFile, err)
		}
	}

	if fileInfo.Size() != int64(len(data)) {
		log.Debug().Msg("configuring notification: local file has different size than remote, writing remote config")
		return writeConfig()
	}

	fileBytes, err := os.ReadFile(cfgFile)
	if err != nil {
		return err
	}

	if !bytes.Equal(fileBytes, data) {
		log.Debug().Msg("configuring notification: local file is different than remote, writing remote config")
		return writeConfig()
	}

	return nil
}

func (n *NotificationConfigReceiver) launch(id string) error {
	cfgFile := filepath.Join(n.opt.RootDir, notificationConfigFilePrefix+id+".json")

	if n.cmdMu.TryLock() {
		defer n.cmdMu.Unlock()

		if time.Since(n.lastRun) > n.opt.Interval {
			target, err := n.opt.UpdateRunner.updater.localTarget(notificationTargetName)
			if err != nil {
				return err
			}

			meta, err := n.opt.UpdateRunner.updater.Lookup(notificationTargetName)
			if err != nil {
				return err
			}
			if err := checkFileHash(meta, target.Path); err != nil {
				n.launchErr = nil
				return n.setTargetsAndHashes()
			}

			if n.launchErr != nil {
				log.Info().Msgf("notification binary disabled since %s due to launch error: %v", n.launchErr.timestamp.Format("2006-01-02"), n.launchErr)
				n.lastRun = time.Now()
				return nil
			}

			n.activeNotificationID = id

			fn := n.opt.runNotificationFn
			if fn == nil {
				fn = func(appPath, configPath string) error {
					log.Info().Str("notification_id", id).Msg("launching notification UI via execuser")
					_, err := execuser.Run(
						appPath,
						execuser.WithArg("--config", configPath),
					)
					return err
				}
			}

			if err := fn(target.ExecPath, cfgFile); err != nil {
				n.activeNotificationID = ""
				if exitErr, ok := err.(*exec.ExitError); ok {
					n.launchErr = &notificationLaunchErr{
						err:       err,
						exitCode:  exitErr.ExitCode(),
						detail:    string(exitErr.Stderr),
						cfgFile:   cfgFile,
						timestamp: time.Now(),
					}
					return fmt.Errorf("launching notification binary with config %q: %w", cfgFile, n.launchErr)
				}
				return fmt.Errorf("launching notification binary with config %q: %w", cfgFile, err)
			}

			n.lastRun = time.Now()
			n.activeNotificationID = ""
		}
	}

	return nil
}

type notificationLaunchErr struct {
	err       error
	exitCode  int
	detail    string
	cfgFile   string
	timestamp time.Time
}

func (e *notificationLaunchErr) Error() string {
	return fmt.Sprintf("%v: %s", e.err, e.detail)
}
