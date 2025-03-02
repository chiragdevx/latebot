package repository

import (
	"database/sql"
	"time"

	"slack-leaves-ai-agent/models"
)

type LeaveRepository struct {
	db *sql.DB
}

func NewLeaveRepository(db *sql.DB) *LeaveRepository {
	return &LeaveRepository{db: db}
}

func (r *LeaveRepository) Create(leave *models.Leave) error {
	query := `
		INSERT INTO leaves (
			username, original_text, start_time, end_time, 
			duration, reason, leave_type, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`

	now := time.Now()
	err := r.db.QueryRow(
		query,
		leave.Username,
		leave.OriginalText,
		leave.StartTime,
		leave.EndTime,
		leave.Duration,
		leave.Reason,
		leave.LeaveType,
		now,
		now,
	).Scan(&leave.ID)

	return err
}
