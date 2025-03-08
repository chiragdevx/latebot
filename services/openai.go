package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

type Metrics struct {
	Count     string `json:"count"`
	Frequency string `json:"frequency"`
}

type QueryResponse struct {
	QueryType       string   `json:"query_type"`         // "top_employee", "period_stats", "employee_stats", etc.
	AnalysisSubtype string   `json:"analysis_subtype"`   // "most_leaves", "late_arrival_trend", etc.
	StartDate       string   `json:"start_date"`         // Change to string for JSON response
	EndDate         string   `json:"end_date"`           // Change to string for JSON response
	Username        string   `json:"username,omitempty"` // Specific employee
	Department      string   `json:"department,omitempty"`
	Limit           int      `json:"limit,omitempty"`
	ComparisonType  string   `json:"comparison_type,omitempty"` // "greater_than", "less_than", etc.
	ComparisonValue int      `json:"comparison_value,omitempty"`
	LeaveTypes      []string `json:"leave_types,omitempty"` // Types: "WFH", "FULL_DAY", etc.
	GroupBy         string   `json:"group_by,omitempty"`    // "day", "week", "month"
	Metrics         Metrics  `json:"metrics,omitempty"`     // Update to use the new Metrics struct
	Error           string   `json:"error,omitempty"`       // Error messages
	Suggestion      string   `json:"suggestion,omitempty"`  // New field for suggestions
}

type Statistics struct {
	TotalLeaves      int     `json:"total_leaves"`
	AverageLeaveDays float64 `json:"average_leave_days"`
	MostLeavesUser   string  `json:"most_leaves_user,omitempty"`
	HighestWFHUser   string  `json:"highest_wfh_user,omitempty"`
	MostLateArrivals string  `json:"most_late_arrivals,omitempty"`
	TotalEarlyDepart int     `json:"total_early_depart"`
}

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

func (s *OpenAIService) ParseQuery(query string) (*QueryResponse, error) {
	loc, _ := time.LoadLocation("Asia/Kolkata")
	now := time.Now().In(loc)

	// Updated prompt with better clarity and validation instructions
	prompt := fmt.Sprintf(`
Analyze this leave/attendance query and return a structured JSON response.

Query: "%s"
Current time: %s

### 🔍 Examples of Correct Queries:
- "Who took the most leave this month?"
- "How many people worked from home last week?"
- "Show WFH trends over the past year."
- "Which department has the most WFH employees?"

### 📌 Important Rules:
1. **Always return valid JSON** with all required fields.
2. **Detect and correct misspellings** in queries where possible.
3. If the query is **invalid or ambiguous**, return a **valid suggestion** in the 'suggestion' field.
4. If a query references a **future date or untracked data**, return {"error": "Invalid query"} and a **possible fix**.
5. **NEVER include markdown, bullet points, or extra text**, only return structured JSON.

#### 📜 JSON Output Format:
{
	"query_type": REQUIRED,
	"analysis_subtype": REQUIRED,
	"start_date": optional,
	"end_date": optional,
	"username": optional,
	"department": optional,
	"limit": optional,
	"comparison_type": optional,
	"comparison_value": optional,
	"leave_types": optional,
	"group_by": optional,
	"metrics": optional,
	"error": optional,
	"suggestion": optional
}`, query, now.Format(time.RFC3339))

	resp, err := s.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: "gpt-4o-mini",
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: "You are an AI trained to process attendance queries and return structured JSON. Never return markdown, code blocks, or plain text.",
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: 0.3, // Lower temp for more consistent responses
		},
	)

	if err != nil {
		return nil, fmt.Errorf("OpenAI API error: %v", err)
	}

	// Clean response from AI
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.ReplaceAll(content, "```json", "")
	content = strings.ReplaceAll(content, "```", "")

	s.log.Printf("Raw OpenAI response: %s", content)

	// Parse JSON response
	var queryResp QueryResponse
	if err := json.Unmarshal([]byte(content), &queryResp); err != nil {
		return nil, fmt.Errorf("JSON parse error: %v\nResponse: %s", err, content)
	}

	// If an error exists in the response, handle it properly
	if queryResp.Error != "" {
		s.log.Printf("Query error detected: %s", queryResp.Error)
		// Suggest a corrected query if available
		if queryResp.Suggestion != "" {
			return nil, fmt.Errorf("Query error: %s. Suggested fix: %s", queryResp.Error, queryResp.Suggestion)
		}
		return nil, fmt.Errorf("Query error: %s", queryResp.Error)
	}

	// If no results are found, provide a meaningful response
	if queryResp.Metrics.Count == "0" {
		return &queryResp, fmt.Errorf("No results found for the query: %s. Please try a different query.", query)
	}

	return &queryResp, nil
}

func GetStatistics(prompt string) (Statistics, error) {
	if prompt == "" {
		return Statistics{}, errors.New("query cannot be empty")
	}

	result, err := queryDatabaseForStatistics(prompt)
	if err != nil {
		return Statistics{}, err
	}

	if len(result) == 0 {
		return Statistics{}, errors.New("no statistics found for the given query")
	}

	return processStatistics(result), nil
}

func queryDatabaseForStatistics(prompt string) ([]Statistics, error) {
	var results []Statistics
	// Implement database query logic here
	return results, nil
}

func processStatistics(result []Statistics) Statistics {
	if len(result) == 0 {
		return Statistics{}
	}

	totalLeaves := 0
	for _, stat := range result {
		totalLeaves += stat.TotalLeaves
	}

	return Statistics{
		TotalLeaves:      totalLeaves,
		AverageLeaveDays: float64(totalLeaves) / float64(len(result)),
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
