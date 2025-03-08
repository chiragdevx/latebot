package repository

import (
	"database/sql"
	"fmt"
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
		return nil, fmt.Errorf("no leave records found for any employee. Please ensure that leave data is available for this period.")
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

	if len(stats) == 0 {
		return nil, fmt.Errorf("no leave records found for *%s*. Please check if the username is correct or if they have taken any leave.", username)
	}

	return stats, nil
}

func (r *LeaveRepository) GetMostLeavesThisMonth() ([]models.EmployeeLeaveStats, error) {
	query := `
		SELECT 
			username,
			COUNT(*) as leave_count,
			STRING_AGG(leave_type, ', ') as leave_types,
			SUM(EXTRACT(EPOCH FROM (end_time - start_time))/3600) as total_hours
		FROM leaves 
		WHERE start_time >= date_trunc('month', CURRENT_DATE)
		GROUP BY username
		ORDER BY leave_count DESC
		LIMIT 1
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []models.EmployeeLeaveStats
	for rows.Next() {
		var stat models.EmployeeLeaveStats
		err := rows.Scan(&stat.Username, &stat.LeaveCount, &stat.LeaveTypes, &stat.TotalHours)
		if err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	if len(stats) == 0 {
		return nil, fmt.Errorf("no leave records found for this month.")
	}

	return stats, nil
}

func (r *LeaveRepository) GetTopEmployeesWithMostLeaves(year int, limit int) ([]models.EmployeeLeaveStats, error) {
	// Implement logic to query the database for the top N employees with the most leaves in a given year
	return []models.EmployeeLeaveStats{}, nil
}

func (r *LeaveRepository) GetLeaveCountToday() (int, error) {
	query := `
		SELECT COUNT(*)
		FROM leaves
		WHERE start_time <= CURRENT_DATE AND end_time >= CURRENT_DATE
	`

	var count int
	err := r.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (r *LeaveRepository) GetEmployeesNeverTakenLeaveThisYear() ([]models.Employee, error) {
	query := `
		SELECT username
		FROM employees
		WHERE username NOT IN (
			SELECT username
			FROM leaves
			WHERE EXTRACT(YEAR FROM start_time) = EXTRACT(YEAR FROM CURRENT_DATE)
		)
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var employees []models.Employee
	for rows.Next() {
		var employee models.Employee
		if err := rows.Scan(&employee.Username); err != nil {
			return nil, err
		}
		employees = append(employees, employee)
	}

	return employees, nil
}

func (r *LeaveRepository) GetAllEmployeesCurrentlyOnLeave() ([]models.Employee, error) {
	query := `
		SELECT DISTINCT username
		FROM leaves
		WHERE start_time <= CURRENT_DATE AND end_time >= CURRENT_DATE
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var employees []models.Employee
	for rows.Next() {
		var employee models.Employee
		if err := rows.Scan(&employee.Username); err != nil {
			return nil, err
		}
		employees = append(employees, employee)
	}

	return employees, nil
}

type LeaveStats struct {
	Username   string  `json:"username"`
	LeaveCount int     `json:"leave_count"`
	LeaveTypes string  `json:"leave_types"`
	TotalHours float64 `json:"total_hours"`
}
