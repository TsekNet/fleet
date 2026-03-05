package mysql

import (
	"context"
	"crypto/md5" //nolint:gosec
	"database/sql"
	"fmt"

	"github.com/fleetdm/fleet/v4/server/contexts/ctxerr"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func (ds *Datastore) BatchSetNotifications(ctx context.Context, tmID *uint, payloads []*fleet.NotificationPayload) ([]fleet.NotificationResponse, error) {
	globalOrTeamID := uint(0)
	if tmID != nil {
		globalOrTeamID = *tmID
	}

	var results []fleet.NotificationResponse
	err := ds.withRetryTxx(ctx, func(tx sqlx.ExtContext) error {
		keepNames := make([]string, 0, len(payloads))
		for _, p := range payloads {
			keepNames = append(keepNames, p.NotificationID)
		}

		if len(keepNames) > 0 {
			delStmt := `DELETE FROM notifications WHERE global_or_team_id = ? AND notification_id NOT IN (?)`
			stmt, args, err := sqlx.In(delStmt, globalOrTeamID, keepNames)
			if err != nil {
				return ctxerr.Wrap(ctx, err, "prepare delete notifications")
			}
			if _, err := tx.ExecContext(ctx, stmt, args...); err != nil {
				return ctxerr.Wrap(ctx, err, "delete obsolete notifications")
			}

			unsetStmt := `UPDATE policies SET notification_db_id = NULL
				WHERE notification_db_id IS NOT NULL
				AND notification_db_id NOT IN (SELECT id FROM notifications WHERE global_or_team_id = ?)`
			if _, err := tx.ExecContext(ctx, unsetStmt, globalOrTeamID); err != nil {
				return ctxerr.Wrap(ctx, err, "unset policy notification references")
			}
		} else {
			if _, err := tx.ExecContext(ctx, `DELETE FROM notifications WHERE global_or_team_id = ?`, globalOrTeamID); err != nil {
				return ctxerr.Wrap(ctx, err, "delete all notifications for team")
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE policies SET notification_db_id = NULL WHERE notification_db_id IS NOT NULL AND team_id <=> ?`, tmID); err != nil {
				return ctxerr.Wrap(ctx, err, "unset all policy notification references")
			}
		}

		for _, p := range payloads {
			checksum := md5.Sum(p.Config) //nolint:gosec
			const insContent = `INSERT INTO notification_contents (md5_checksum, contents) VALUES (?, ?)
				ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`
			res, err := tx.ExecContext(ctx, insContent, checksum[:], string(p.Config))
			if err != nil {
				return ctxerr.Wrap(ctx, err, "insert notification contents")
			}
			contentID, _ := res.LastInsertId()

			const insNotif = `INSERT INTO notifications (team_id, global_or_team_id, notification_id, notification_content_id)
				VALUES (?, ?, ?, ?)
				ON DUPLICATE KEY UPDATE notification_content_id = VALUES(notification_content_id), id = LAST_INSERT_ID(id)`
			if _, err := tx.ExecContext(ctx, insNotif, tmID, globalOrTeamID, p.NotificationID, contentID); err != nil {
				return ctxerr.Wrap(ctx, err, "upsert notification")
			}
		}

		if err := sqlx.SelectContext(ctx, tx, &results,
			`SELECT id, team_id, notification_id FROM notifications WHERE global_or_team_id = ? ORDER BY notification_id`, globalOrTeamID); err != nil {
			return ctxerr.Wrap(ctx, err, "select notification responses")
		}
		return nil
	})
	return results, err
}

func (ds *Datastore) GetNotificationByNotificationID(ctx context.Context, notificationID string, teamID *uint) (*fleet.Notification, error) {
	globalOrTeamID := uint(0)
	if teamID != nil {
		globalOrTeamID = *teamID
	}

	var n fleet.Notification
	err := sqlx.GetContext(ctx, ds.reader(ctx), &n,
		`SELECT id, team_id, notification_id, notification_content_id, created_at, updated_at
		FROM notifications WHERE global_or_team_id = ? AND notification_id = ?`,
		globalOrTeamID, notificationID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ctxerr.Wrap(ctx, notFound("Notification").WithName(notificationID), "get notification")
		}
		return nil, ctxerr.Wrap(ctx, err, "get notification by notification_id")
	}
	return &n, nil
}

func (ds *Datastore) GetNotificationContents(ctx context.Context, notificationContentID uint) ([]byte, error) {
	var contents string
	err := sqlx.GetContext(ctx, ds.reader(ctx), &contents,
		`SELECT contents FROM notification_contents WHERE id = ?`, notificationContentID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ctxerr.Wrap(ctx, notFound("NotificationContents"), "get notification contents")
		}
		return nil, ctxerr.Wrap(ctx, err, "get notification contents")
	}
	return []byte(contents), nil
}

func (ds *Datastore) NewHostNotificationExecution(ctx context.Context, request *fleet.HostNotificationRequestPayload) (string, error) {
	var execID string
	err := ds.withRetryTxx(ctx, func(tx sqlx.ExtContext) error {
		const insUA = `INSERT INTO upcoming_activities
			(host_id, priority, fleet_initiated, activity_type, execution_id, payload)
			VALUES (?, 0, 1, 'notification', ?, JSON_OBJECT())`

		execID = uuid.New().String()
		result, err := tx.ExecContext(ctx, insUA, request.HostID, execID)
		if err != nil {
			return ctxerr.Wrap(ctx, err, "insert notification upcoming activity")
		}
		activityID, _ := result.LastInsertId()

		const insNUA = `INSERT INTO notification_upcoming_activities
			(upcoming_activity_id, notification_db_id, policy_id) VALUES (?, ?, ?)`
		if _, err := tx.ExecContext(ctx, insNUA, activityID, request.NotificationDBID, request.PolicyID); err != nil {
			return ctxerr.Wrap(ctx, err, "insert notification upcoming activity join")
		}

		if _, err := ds.activateNextUpcomingActivity(ctx, tx, request.HostID, ""); err != nil {
			return ctxerr.Wrap(ctx, err, "activate next activity for notification")
		}

		return nil
	})
	return execID, err
}

func (ds *Datastore) SetHostNotificationResult(ctx context.Context, result *fleet.HostNotificationResultPayload) error {
	const stmt = `UPDATE host_notification_results
		SET result = ?, exit_code = ?
		WHERE host_id = ? AND notification_id = ? AND result = ''
		ORDER BY created_at DESC LIMIT 1`

	res, err := ds.writer(ctx).ExecContext(ctx, stmt,
		result.Result, result.ExitCode, result.HostID, result.NotificationID)
	if err != nil {
		return ctxerr.Wrap(ctx, err, "set host notification result")
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ctxerr.Wrap(ctx, notFound("HostNotificationResult"), "no pending notification result found")
	}

	return nil
}

func (ds *Datastore) ListPendingNotificationsForHost(ctx context.Context, hostID uint) ([]string, error) {
	const stmt = `SELECT hnr.notification_id
		FROM host_notification_results hnr
		WHERE hnr.host_id = ? AND hnr.result = ''
		ORDER BY hnr.created_at ASC`

	var ids []string
	if err := sqlx.SelectContext(ctx, ds.reader(ctx), &ids, stmt, hostID); err != nil {
		return nil, ctxerr.Wrap(ctx, err, "list pending notifications for host")
	}
	return ids, nil
}

func (ds *Datastore) GetPoliciesWithAssociatedNotification(ctx context.Context, teamID *uint) ([]fleet.PolicyNotificationData, error) {
	stmt := `SELECT p.id, p.notification_db_id, n.notification_id
		FROM policies p
		INNER JOIN notifications n ON n.id = p.notification_db_id
		WHERE p.notification_db_id IS NOT NULL AND p.team_id %s`

	var args []interface{}
	if teamID != nil {
		stmt = fmt.Sprintf(stmt, "= ?")
		args = append(args, *teamID)
	} else {
		stmt = fmt.Sprintf(stmt, "IS NULL")
	}

	var results []fleet.PolicyNotificationData
	if err := sqlx.SelectContext(ctx, ds.reader(ctx), &results, stmt, args...); err != nil {
		return nil, ctxerr.Wrap(ctx, err, "get policies with associated notification")
	}
	return results, nil
}
