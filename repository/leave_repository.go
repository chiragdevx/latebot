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

func (r *LeaveRepository) GetLeaveStatsByPeriod(startDate, endDate time.Time) ([]LeaveStats, error) {
	query := `
		SELECT 
			username,
			COUNT(*) as leave_count,
			STRING_AGG(leave_type, ', ') as leave_types,
			SUM(EXTRACT(EPOCH FROM (end_time - start_time))/3600) as total_hours
		FROM leaves 
		WHERE start_time BETWEEN $1 AND $2
		GROUP BY username
		ORDER BY leave_count DESC
	`

	rows, err := r.db.Query(query, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []LeaveStats
	for rows.Next() {
		var stat LeaveStats
		err := rows.Scan(&stat.Username, &stat.LeaveCount, &stat.LeaveTypes, &stat.TotalHours)
		if err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

func (r *LeaveRepository) GetTopLeaveEmployee() (*LeaveStats, error) {
	query := `
		SELECT 
			username,
			COUNT(*) as leave_count,
			STRING_AGG(leave_type, ', ') as leave_types,
			SUM(EXTRACT(EPOCH FROM (end_time - start_time))/3600) as total_hours
		FROM leaves 
		GROUP BY username
		ORDER BY leave_count DESC
		LIMIT 1
	`

	var stat LeaveStats
	err := r.db.QueryRow(query).Scan(
		&stat.Username,
		&stat.LeaveCount,
		&stat.LeaveTypes,
		&stat.TotalHours,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &stat, nil
}

func (r *LeaveRepository) GetEmployeeStats(username string) ([]LeaveStats, error) {
	query := `
		SELECT 
			username,
			COUNT(*) as leave_count,
			STRING_AGG(leave_type, ', ') as leave_types,
			SUM(EXTRACT(EPOCH FROM (end_time - start_time))/3600) as total_hours
		FROM leaves 
		WHERE username = $1
		GROUP BY username
	`

	rows, err := r.db.Query(query, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []LeaveStats
	for rows.Next() {
		var stat LeaveStats
		err := rows.Scan(&stat.Username, &stat.LeaveCount, &stat.LeaveTypes, &stat.TotalHours)
		if err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, nil
}

type LeaveStats struct {
	Username   string  `json:"username"`
	LeaveCount int     `json:"leave_count"`
	LeaveTypes string  `json:"leave_types"`
	TotalHours float64 `json:"total_hours"`
}
