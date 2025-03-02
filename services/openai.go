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

	prompt := `Parse this message for leave/attendance details. Return a JSON object only.

	Message: "` + text + `"
	Current time: ` + now.Format(time.RFC3339) + `

	Current context:
	- Today's date: ` + today.Format("2006-01-02") + `
	- Tomorrow's date: ` + tomorrow.Format("2006-01-02") + `
	- Default work hours: 9:00 AM to 6:00 PM
	- Timezone: Asia/Kolkata (IST)

	Rules for leave_type:
	- "WFH" for working from home
	- "FULL_DAY" for full day leave
	- "HALF_DAY" for half day leave
	- "LATE_ARRIVAL" for coming late
	- "EARLY_DEPARTURE" for leaving early

	Important validation rules:
	- Leave cannot be requested for past dates
	- Start time must be before end time
	- If validation fails, set is_valid to false and include error message
	- Use IST timezone (+05:30) for all dates
	- For "today", use ` + today.Format("2006-01-02") + `
	- For "tomorrow", use ` + tomorrow.Format("2006-01-02") + `
	- For full day leave: set time to 9:00 AM - 6:00 PM IST
	- For half day leave: set time to either 9:00 AM - 1:00 PM or 2:00 PM - 6:00 PM IST
	- For WFH: set time to 9:00 AM - 6:00 PM IST

	Return a raw JSON object with these fields (no markdown, no formatting):
	{
		"is_valid": true/false,
		"leave_type": "WFH/FULL_DAY/HALF_DAY/LATE_ARRIVAL/EARLY_DEPARTURE",
		"start_time": "` + today.Format("2006-01-02") + `T09:00:00+05:30",
		"end_time": "` + today.Format("2006-01-02") + `T18:00:00+05:30",
		"duration": "9 hours",
		"reason": "reason for leave",
		"error": "error message if validation fails"
	}

	Example responses:
	For "wfh tomorrow":
	{
		"is_valid": true,
		"leave_type": "WFH",
		"start_time": "` + tomorrow.Format("2006-01-02") + `T09:00:00+05:30",
		"end_time": "` + tomorrow.Format("2006-01-02") + `T18:00:00+05:30",
		"duration": "9 hours",
		"reason": "Working from home tomorrow"
	}

	For "half day leave tomorrow morning":
	{
		"is_valid": true,
		"leave_type": "HALF_DAY",
		"start_time": "` + tomorrow.Format("2006-01-02") + `T09:00:00+05:30",
		"end_time": "` + tomorrow.Format("2006-01-02") + `T13:00:00+05:30",
		"duration": "4 hours",
		"reason": "Half day leave in morning"
	}

	If the message is not a leave request, return only {"is_valid": false} without any formatting or markdown`

	s.log.Printf("Processing message: %s", text)

	resp, err := s.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are a JSON-only response bot. Never use markdown or code blocks. Return raw JSON only.",
				},
			},
			Temperature: 0.1,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("OpenAI API error: %v", err)
	}

	content := resp.Choices[0].Message.Content
	// Remove any markdown formatting if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	s.log.Printf("Response: %s", content)

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

	// Compare dates only (ignore time) for past date validation
	startDate := time.Date(startTimeIST.Year(), startTimeIST.Month(), startTimeIST.Day(), 0, 0, 0, 0, loc)
	todayDate := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 0, 0, 0, 0, loc)

	if startDate.Before(todayDate) {
		leaveResp.IsValid = false
		leaveResp.Error = "Cannot request leave for past dates"
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
