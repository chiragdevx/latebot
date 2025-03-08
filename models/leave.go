package models

import (
	"encoding/json"
	"time"
)

type Leave struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	OriginalText string    `json:"original_text"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	Duration     string    `json:"duration"`
	Reason       string    `json:"reason"`
	LeaveType    string    `json:"leave_type"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type EmployeeLeaveStats struct {
	Username   string  `json:"username"`
	LeaveCount int     `json:"leave_count"`
	LeaveTypes string  `json:"leave_types"`
	TotalHours float64 `json:"total_hours"`
}

type Employee struct {
	Username string `json:"username"`
	// Add other relevant fields as necessary
}

type LeaveResponse struct {
	IsValid   bool      `json:"is_valid"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  string    `json:"duration"`
	Reason    string    `json:"reason"`
	LeaveType string    `json:"leave_type"`
	Error     string    `json:"error,omitempty"`
}

// When encoding to JSON, ensure to format the time fields
func (r LeaveResponse) MarshalJSON() ([]byte, error) {
	type Alias LeaveResponse
	return json.Marshal(&struct {
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
		*Alias
	}{
		StartTime: r.StartTime.Format(time.RFC3339),
		EndTime:   r.EndTime.Format(time.RFC3339),
		Alias:     (*Alias)(&r),
	})
}

type QueryResponse struct {
	QueryType string    `json:"query_type"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	Username  string    `json:"username"`
	Error     string    `json:"error"`
}

// When encoding to JSON, ensure to format the time fields
func (q QueryResponse) MarshalJSON() ([]byte, error) {
	type Alias QueryResponse
	return json.Marshal(&struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		*Alias
	}{
		StartDate: q.StartDate.Format(time.RFC3339),
		EndDate:   q.EndDate.Format(time.RFC3339),
		Alias:     (*Alias)(&q),
	})
}
