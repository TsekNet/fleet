package tables

import "database/sql"

func init() {
	MigrationClient.AddMigration(Up_20260305100000, Down_20260305100000)
}

func Up_20260305100000(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE notification_contents (
			id INT UNSIGNED NOT NULL AUTO_INCREMENT,
			md5_checksum BINARY(16) NOT NULL,
			contents MEDIUMTEXT COLLATE utf8mb4_unicode_ci NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY idx_notification_contents_md5 (md5_checksum)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`CREATE TABLE notifications (
			id INT UNSIGNED NOT NULL AUTO_INCREMENT,
			team_id INT UNSIGNED DEFAULT NULL,
			global_or_team_id INT UNSIGNED NOT NULL DEFAULT 0,
			notification_id VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL,
			notification_content_id INT UNSIGNED DEFAULT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY idx_notifications_team_nid (global_or_team_id, notification_id),
			KEY fk_notifications_team (team_id),
			KEY fk_notifications_content (notification_content_id),
			CONSTRAINT fk_notifications_team FOREIGN KEY (team_id) REFERENCES teams (id) ON DELETE CASCADE ON UPDATE CASCADE,
			CONSTRAINT fk_notifications_content FOREIGN KEY (notification_content_id) REFERENCES notification_contents (id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`ALTER TABLE upcoming_activities
			MODIFY COLUMN activity_type ENUM('script','software_install','software_uninstall','vpp_app_install','in_house_app_install','notification') NOT NULL`,

		`CREATE TABLE notification_upcoming_activities (
			upcoming_activity_id BIGINT UNSIGNED NOT NULL,
			notification_db_id INT UNSIGNED NOT NULL,
			policy_id INT UNSIGNED DEFAULT NULL,
			PRIMARY KEY (upcoming_activity_id),
			KEY fk_nua_notification (notification_db_id),
			KEY fk_nua_policy (policy_id),
			CONSTRAINT fk_nua_ua FOREIGN KEY (upcoming_activity_id) REFERENCES upcoming_activities (id) ON DELETE CASCADE,
			CONSTRAINT fk_nua_notification FOREIGN KEY (notification_db_id) REFERENCES notifications (id) ON DELETE CASCADE,
			CONSTRAINT fk_nua_policy FOREIGN KEY (policy_id) REFERENCES policies (id) ON DELETE SET NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`CREATE TABLE host_notification_results (
			id INT UNSIGNED NOT NULL AUTO_INCREMENT,
			host_id INT UNSIGNED NOT NULL,
			execution_id VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL,
			notification_db_id INT UNSIGNED DEFAULT NULL,
			notification_id VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL,
			result VARCHAR(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
			exit_code INT DEFAULT NULL,
			policy_id INT UNSIGNED DEFAULT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (id),
			UNIQUE KEY idx_hnr_execution_id (execution_id),
			KEY idx_hnr_host_id (host_id),
			KEY fk_hnr_notification (notification_db_id),
			KEY fk_hnr_policy (policy_id),
			CONSTRAINT fk_hnr_notification FOREIGN KEY (notification_db_id) REFERENCES notifications (id) ON DELETE SET NULL,
			CONSTRAINT fk_hnr_policy FOREIGN KEY (policy_id) REFERENCES policies (id) ON DELETE SET NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		`ALTER TABLE policies ADD COLUMN notification_db_id INT UNSIGNED DEFAULT NULL,
			ADD KEY fk_policies_notification (notification_db_id),
			ADD CONSTRAINT fk_policies_notification FOREIGN KEY (notification_db_id) REFERENCES notifications (id) ON DELETE SET NULL`,
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func Down_20260305100000(tx *sql.Tx) error {
	return nil
}
