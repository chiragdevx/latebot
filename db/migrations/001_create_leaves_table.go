package migrations

import (
	"database/sql"
)

func CreateLeavesTable(db *sql.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS leaves (
			id SERIAL PRIMARY KEY,
			user_name VARCHAR(255),
			original_text TEXT NOT NULL,
			start_time TIMESTAMP,
			end_time TIMESTAMP,
			duration VARCHAR(255),
			reason TEXT,
			is_working_from_home BOOLEAN DEFAULT FALSE NOT NULL,
			is_leave_request BOOLEAN DEFAULT FALSE NOT NULL,
			is_running_late BOOLEAN DEFAULT FALSE NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP NOT NULL
		);
	`

	_, err := db.Exec(query)
	return err
} 