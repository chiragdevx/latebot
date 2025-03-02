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
	LeaveType string    `json:"leave_type"` // WFH, FULL_DAY, HALF_DAY, LATE_ARRIVAL, EARLY_DEPARTURE
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
	prompt := `Parse this message for leave/attendance details. Return a JSON object only.

	Message: "` + text + `"
	Current time: ` + timestamp + `

	Rules for leave_type:
	- "WFH" for working from home
	- "FULL_DAY" for full day leave
	- "HALF_DAY" for half day leave
	- "LATE_ARRIVAL" for coming late
	- "EARLY_DEPARTURE" for leaving early

	Return a raw JSON object with these fields (no markdown, no formatting):
	{
		"is_valid": true/false,
		"leave_type": "WFH/FULL_DAY/HALF_DAY/LATE_ARRIVAL/EARLY_DEPARTURE",
		"start_time": "2025-03-02T10:00:00Z",
		"end_time": "2025-03-02T18:00:00Z",
		"duration": "8 hours",
		"reason": "reason for leave"
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

	return &leaveResp, nil
}
