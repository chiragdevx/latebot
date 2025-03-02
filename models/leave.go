package models

import "time"

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
