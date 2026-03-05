package fleet

import "time"

// Notification represents a saved notification template that can be shown to
// hosts via the Hermes binary.
type Notification struct {
	ID                    uint      `json:"id" db:"id"`
	TeamID                *uint     `json:"team_id" renameto:"fleet_id" db:"team_id"`
	NotificationID        string    `json:"notification_id" db:"notification_id"`
	NotificationContentID uint      `json:"-" db:"notification_content_id"`
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
}

func (n Notification) AuthzType() string {
	return "notification"
}

// NotificationPayload is the payload used when applying notifications by batch
// (analogous to ScriptPayload for scripts).
type NotificationPayload struct {
	NotificationID string `json:"notification_id"`
	Config         []byte `json:"config"`
}

// NotificationResponse is returned when applying notifications by batch.
type NotificationResponse struct {
	TeamID         *uint  `json:"team_id" renameto:"fleet_id" db:"team_id"`
	ID             uint   `json:"id" db:"id"`
	NotificationID string `json:"notification_id" db:"notification_id"`
}

// HostNotificationResult tracks the delivery and response of a notification
// for a specific host.
type HostNotificationResult struct {
	ID               uint      `json:"id" db:"id"`
	HostID           uint      `json:"host_id" db:"host_id"`
	ExecutionID      string    `json:"execution_id" db:"execution_id"`
	NotificationDBID *uint     `json:"notification_db_id,omitempty" db:"notification_db_id"`
	NotificationID   string    `json:"notification_id" db:"notification_id"`
	Result           string    `json:"result" db:"result"`
	ExitCode         *int      `json:"exit_code" db:"exit_code"`
	PolicyID         *uint     `json:"policy_id,omitempty" db:"policy_id"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

// HostNotificationResultPayload is sent by Orbit when reporting a notification
// result.
type HostNotificationResultPayload struct {
	HostID         uint   `json:"host_id"`
	NotificationID string `json:"notification_id"`
	Result         string `json:"result"`
	ExitCode       int    `json:"exit_code"`
}

// PolicyNotification is the notification associated with a policy (shown in
// the policy API response).
type PolicyNotification struct {
	NotificationID string `json:"notification_id" db:"notification_id"`
}

// HostNotificationRequestPayload is used to create a new notification
// execution for a host (analogous to HostScriptRequestPayload).
type HostNotificationRequestPayload struct {
	HostID           uint
	NotificationDBID uint
	NotificationID   string
	PolicyID         *uint
}

// PolicyNotificationData links a policy to its notification template.
type PolicyNotificationData struct {
	PolicyID         uint   `db:"id"`
	NotificationDBID uint   `db:"notification_db_id"`
	NotificationID   string `db:"notification_id"`
}
