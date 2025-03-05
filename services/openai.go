package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

type LeaveResponse struct {
	IsValid   bool      `json:"is_valid"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  string    `json:"duration"`
	Reason    string    `json:"reason"`
	LeaveType string    `json:"leave_type"`      // WFH, FULL_DAY, HALF_DAY, LATE_ARRIVAL, EARLY_DEPARTURE
	Error     string    `json:"error,omitempty"` // Add error field for validation messages
}

type QueryResponse struct {
	QueryType string    `json:"query_type"` // "top_employee", "period_stats", "employee_stats", etc.
	StartDate time.Time `json:"start_date"` // For period-based queries
	EndDate   time.Time `json:"end_date"`   // For period-based queries
	Username  string    `json:"username"`   // For employee-specific queries
	Error     string    `json:"error"`      // Any parsing errors
}

type OpenAIService struct {
	client *openai.Client
	log    *log.Logger
}

func NewOpenAIService(apiKey string) *OpenAIService {
	return &OpenAIService{
		client: openai.NewClient(apiKey),
		log:    log.New(os.Stdout, "🤖 OPENAI  | ", log.Ltime),
	}
}

func (s *OpenAIService) ParseLeaveRequest(text, timestamp string) (*LeaveResponse, error) {
	// Set timezone to IST
	loc, _ := time.LoadLocation("Asia/Kolkata")
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	tomorrow := today.AddDate(0, 0, 1)
	maxFutureDate := today.AddDate(0, 0, 30)

	prompt := `Parse this message for leave/attendance details. Return a JSON object only.

	Message: "` + text + `"
	Current time: ` + now.Format(time.RFC3339) + `

	Current context:
	- Today's date: ` + today.Format("2006-01-02") + `
	- Tomorrow's date: ` + tomorrow.Format("2006-01-02") + `
	- Maximum allowed date: ` + maxFutureDate.Format("2006-01-02") + `
	- Default work hours: 9:00 AM to 6:00 PM
	- Timezone: Asia/Kolkata (IST)
	- Current year: ` + fmt.Sprintf("%d", now.Year()) + `

	Rules for leave_type:
	- "WFH" for working from home
	- "FULL_DAY" for full day leave
	- "HALF_DAY" for half day leave
	- "LATE_ARRIVAL" for coming late
	- "EARLY_DEPARTURE" for leaving early

	Important validation rules:
	- Leave cannot be requested for past dates
	- Leave cannot be requested for dates more than 30 days in advance
	- Start time must be before end time
	- If validation fails, set is_valid to false and include error message
	- Use IST timezone (+05:30) for all dates
	- For "today", use ` + today.Format("2006-01-02") + `
	- For "tomorrow", use ` + tomorrow.Format("2006-01-02") + `
	- For specific dates (e.g. "march 10"):
	  * If the date is in the past this year, set is_valid to false with error
	  * If the date is in the future this year but more than 30 days away, set is_valid to false with error
	  * If the date is within next 30 days, use that date
	- For full day leave: set time to 9:00 AM - 6:00 PM IST
	- For half day leave: set time to either 9:00 AM - 1:00 PM or 2:00 PM - 6:00 PM IST
	- For WFH: set time to 9:00 AM - 6:00 PM IST

	Return a JSON object with these fields:
	{
		"is_valid": true/false,
		"leave_type": "WFH/FULL_DAY/HALF_DAY/LATE_ARRIVAL/EARLY_DEPARTURE",
		"start_time": "2024-03-01T09:00:00+05:30",
		"end_time": "2024-03-01T18:00:00+05:30",
		"duration": "9 hours",
		"reason": "reason for leave",
		"error": "error message if validation fails"
	}`

	resp, err := s.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are a date-aware JSON response bot. Use the current year for all dates. Never use markdown.",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: 0.1,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("OpenAI API error: %v", err)
	}

	content := resp.Choices[0].Message.Content
	content = strings.TrimSpace(content)

	var leaveResp LeaveResponse
	err = json.Unmarshal([]byte(content), &leaveResp)
	if err != nil {
		return nil, fmt.Errorf("JSON parse error: %v\nResponse: %s", err, content)
	}

	if !leaveResp.IsValid {
		return &leaveResp, nil
	}

	// Validate required fields
	if leaveResp.LeaveType == "" {
		return nil, fmt.Errorf("leave_type is required for valid requests")
	}

	// Convert response times to IST for comparison
	startTimeIST := leaveResp.StartTime.In(loc)
	endTimeIST := leaveResp.EndTime.In(loc)
	nowIST := now.In(loc)

	// Compare dates only (ignore time) for validation
	startDate := time.Date(startTimeIST.Year(), startTimeIST.Month(), startTimeIST.Day(), 0, 0, 0, 0, loc)
	todayDate := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 0, 0, 0, 0, loc)
	maxDate := todayDate.AddDate(0, 0, 30)

	if startDate.Before(todayDate) {
		leaveResp.IsValid = false
		leaveResp.Error = "Cannot request leave for past dates"
		return &leaveResp, nil
	}

	if startDate.After(maxDate) {
		leaveResp.IsValid = false
		leaveResp.Error = fmt.Sprintf("Cannot request leave more than 30 days in advance (maximum allowed date is %s)",
			maxDate.Format("January 2, 2006"))
		return &leaveResp, nil
	}

	if endTimeIST.Before(startTimeIST) {
		leaveResp.IsValid = false
		leaveResp.Error = "End time must be after start time"
		return &leaveResp, nil
	}

	leaveResp.StartTime = leaveResp.StartTime.In(loc)
	leaveResp.EndTime = leaveResp.EndTime.In(loc)

	return &leaveResp, nil
}

func (s *OpenAIService) ParseQuery(query string) (*QueryResponse, error) {
	loc, _ := time.LoadLocation("Asia/Kolkata")
	now := time.Now().In(loc)

	prompt := `Parse this leave statistics query and categorize it. Return a raw JSON object without any formatting.

	Query: "` + query + `"
	Current time: ` + now.Format(time.RFC3339) + `

	Understand natural language queries like:
	- "Who has taken the most leaves?"
	- "Show me leave stats for last month"
	- "How many leaves did John take?"
	- "Give me the top 3 employees with most leaves"

	IMPORTANT: Return ONLY the raw JSON object. Do not wrap it in code blocks. Do not use backticks. Do not use markdown.
	BAD: { ... } in code blocks
	GOOD: { ... }

	Return a JSON object with these fields:
	{
		"query_type": one of ["top_employee", "period_stats", "employee_stats"],
		"start_date": "2024-03-01T00:00:00+05:30",  // Optional, for period queries
		"end_date": "2024-03-31T23:59:59+05:30",    // Optional, for period queries
		"username": "john.doe",                      // Optional, for employee specific queries
		"error": "error message if query cannot be understood"
	}`

	resp, err := s.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are a JSON-only response bot. You must return raw JSON without any formatting, markdown, or code blocks. Never wrap JSON in backticks.",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: 0.1,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("OpenAI API error: %v", err)
	}

	content := resp.Choices[0].Message.Content
	content = strings.ReplaceAll(content, "```json", "")
	content = strings.ReplaceAll(content, "```", "")
	content = strings.ReplaceAll(content, "`", "")
	content = strings.ReplaceAll(content, "\n", "")
	content = strings.ReplaceAll(content, "\r", "")
	content = strings.TrimSpace(content)

	s.log.Printf("Cleaned response: %s", content)

	var queryResp QueryResponse
	if err := json.Unmarshal([]byte(content), &queryResp); err != nil {
		return nil, fmt.Errorf("JSON parse error: %v\nResponse: %s", err, content)
	}

	return &queryResp, nil
}
