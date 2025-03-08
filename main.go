package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"slack-leaves-ai-agent/models"
	"slack-leaves-ai-agent/repository"
	"slack-leaves-ai-agent/services"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type Config struct {
	Port               string
	SlackBotToken      string
	SlackAppToken      string
	SlackSigningSecret string
	DBHost             string
	DBPort             string
	DBUser             string
	DBPassword         string
	DBName             string
	OpenAIKey          string
}

func loadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("error loading .env file: %v", err)
	}

	return &Config{
		Port:               os.Getenv("PORT"),
		SlackBotToken:      os.Getenv("SLACK_BOT_TOKEN"),
		SlackAppToken:      os.Getenv("SLACK_APP_TOKEN"),
		SlackSigningSecret: os.Getenv("SLACK_SIGNING_SECRET"),
		DBHost:             os.Getenv("DB_HOST"),
		DBPort:             os.Getenv("DB_PORT"),
		DBUser:             os.Getenv("DB_USER"),
		DBPassword:         os.Getenv("DB_PASSWORD"),
		DBName:             os.Getenv("DB_NAME"),
		OpenAIKey:          os.Getenv("OPENAI_API_KEY"),
	}, nil
}

func initDB(config *Config) (*sql.DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		config.DBHost,
		config.DBPort,
		config.DBUser,
		config.DBPassword,
		config.DBName,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("error pinging database: %v", err)
	}

	log.Println("Connected to PostgreSQL database")
	return db, nil
}

type App struct {
	config        *Config
	db            *sql.DB
	openAI        *services.OpenAIService
	leaveRepo     *repository.LeaveRepository
	slackClient   *slack.Client
	processedMsgs map[string]bool
}

func NewApp(config *Config, db *sql.DB) *App {
	return &App{
		config:        config,
		db:            db,
		openAI:        services.NewOpenAIService(config.OpenAIKey),
		leaveRepo:     repository.NewLeaveRepository(db),
		slackClient:   slack.New(config.SlackBotToken, slack.OptionAppLevelToken(config.SlackAppToken)),
		processedMsgs: make(map[string]bool),
	}
}

func (a *App) handleMessage(ev *slack.MessageEvent) {
	if a.processedMsgs[ev.Timestamp] {
		logger.Debug("Skipping duplicate message: %s", ev.Timestamp)
		return
	}
	a.processedMsgs[ev.Timestamp] = true

	// Skip bot messages and system messages
	if ev.SubType != "" || ev.BotID != "" {
		logger.Debug("Skipping bot/system message")
		return
	}

	// Skip our own messages
	authTest, err := a.slackClient.AuthTest()
	if err == nil && ev.User == authTest.UserID {
		logger.Debug("Skipping our own message")
		return
	}

	// Get user info
	userInfo, err := a.slackClient.GetUserInfo(ev.User)
	if err != nil {
		log.Printf("Error getting user info: %v", err)
		return
	}

	response, err := a.openAI.ParseLeaveRequest(ev.Text, ev.Timestamp)
	if err != nil {
		log.Printf("Error parsing message: %v", err)
		return
	}

	if !response.IsValid {
		// If there's a validation error, inform the user
		if response.Error != "" {
			_, _, err = a.slackClient.PostMessage(ev.Channel, slack.MsgOptionText(
				fmt.Sprintf("❌ Unable to process leave request: %s", response.Error),
				false,
			))
			if err != nil {
				log.Printf("Error sending error message: %v", err)
			}
		}
		return
	}

	leave := &models.Leave{
		Username:     userInfo.Name,
		OriginalText: ev.Text,
		StartTime:    response.StartTime,
		EndTime:      response.EndTime,
		Duration:     response.Duration,
		Reason:       response.Reason,
		LeaveType:    response.LeaveType,
	}

	if err := a.leaveRepo.Create(leave); err != nil {
		log.Printf("Error saving leave: %v", err)
		return
	}

	// Send confirmation message
	var emoji, messageType string
	switch response.LeaveType {
	case "WFH":
		emoji = "🏠"
		messageType = "WFH"
	case "FULL_DAY":
		emoji = "🌴"
		messageType = "full day leave"
	case "HALF_DAY":
		emoji = "🌓"
		messageType = "half day leave"
	case "LATE_ARRIVAL":
		emoji = "⏰"
		messageType = "late arrival"
	case "EARLY_DEPARTURE":
		emoji = "🏃"
		messageType = "early departure"
	default:
		emoji = "✅"
		messageType = "request"
	}

	_, _, err = a.slackClient.PostMessage(ev.Channel, slack.MsgOptionText(
		fmt.Sprintf("%s Your %s has been recorded!\n"+
			"📅 From: %s\n"+
			"📅 To: %s\n"+
			"📝 Reason: %s\n\n"+
			"Status: %s\n"+
			"Have a great day! 🌟",
			emoji,
			messageType,
			leave.StartTime.Format("Jan 2, 2006 3:04 PM"),
			leave.EndTime.Format("Jan 2, 2006 3:04 PM"),
			leave.Reason,
			getStatusMessage(response.LeaveType),
		), false))

	if err != nil {
		log.Printf("Error sending confirmation: %v", err)
	}
}

func getStatusMessage(leaveType string) string {
	switch leaveType {
	case "WFH":
		return "🏠 Working remotely"
	case "FULL_DAY":
		return "🌴 Out of office"
	case "HALF_DAY":
		return "🌓 Partially available"
	case "LATE_ARRIVAL":
		return "⏰ Arriving late"
	case "EARLY_DEPARTURE":
		return "🏃 Leaving early"
	default:
		return "✅ Recorded"
	}
}

type PrettyLogger struct {
	*log.Logger
}

func NewPrettyLogger() *PrettyLogger {
	return &PrettyLogger{
		Logger: log.New(os.Stdout, "", log.Ltime),
	}
}

func (l *PrettyLogger) Info(format string, v ...interface{}) {
	l.Printf("ℹ️  INFO    | %s", fmt.Sprintf(format, v...))
}

func (l *PrettyLogger) Debug(format string, v ...interface{}) {
	l.Printf("🔍 DEBUG   | %s", fmt.Sprintf(format, v...))
}

func (l *PrettyLogger) Error(format string, v ...interface{}) {
	l.Printf("❌ ERROR   | %s", fmt.Sprintf(format, v...))
}

func (l *PrettyLogger) Socket(format string, v ...interface{}) {
	l.Printf("🔌 SOCKET  | %s", fmt.Sprintf(format, v...))
}

func (l *PrettyLogger) Event(format string, v ...interface{}) {
	l.Printf("📡 EVENT   | %s", fmt.Sprintf(format, v...))
}

var logger = NewPrettyLogger()

func setupSocketModeHandler(app *App, config *Config) error {
	slackClient := slack.New(
		config.SlackBotToken,
		slack.OptionAppLevelToken(config.SlackAppToken),
	)

	socketClient := socketmode.New(
		slackClient,
		socketmode.OptionLog(log.New(os.Stdout, "🔌 ", log.Ltime)),
	)

	go handleSocketModeEvents(socketClient, app)

	logger.Info("Starting Slack bot with Socket Mode...")
	return socketClient.Run()
}

func handleSocketModeEvents(client *socketmode.Client, app *App) {
	for evt := range client.Events {
		switch evt.Type {
		case socketmode.EventTypeConnecting:
			logger.Socket("Connecting to Slack...")
		case socketmode.EventTypeConnectionError:
			logger.Error("Connection failed. Retrying later...")
		case socketmode.EventTypeConnected:
			logger.Socket("Connected to Slack ✨")
		case socketmode.EventTypeEventsAPI:
			eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
			if !ok {
				logger.Debug("Failed to cast event to EventsAPIEvent: %+v", evt.Data)
				continue
			}

			client.Ack(*evt.Request)
			logger.Event("Received event: Type=%s", eventsAPIEvent.Type)

			if eventsAPIEvent.Type == slackevents.CallbackEvent {
				innerEvent := eventsAPIEvent.InnerEvent
				switch ev := innerEvent.Data.(type) {
				case *slackevents.MessageEvent:
					// Skip non-user messages
					if ev.SubType != "" || ev.BotID != "" || ev.ThreadTimeStamp != "" {
						logger.Debug("Skipping non-user message")
						continue
					}

					// Skip our own messages
					authTest, err := app.slackClient.AuthTest()
					if err == nil && ev.User == authTest.UserID {
						logger.Debug("Skipping our own message")
						continue
					}

					logger.Debug("Message from %s: %s", ev.User, ev.Text)
					messageEvent := &slack.MessageEvent{
						Msg: slack.Msg{
							Text:      ev.Text,
							User:      ev.User,
							Channel:   ev.Channel,
							Timestamp: ev.TimeStamp,
						},
					}
					go app.handleMessage(messageEvent)
				default:
					logger.Debug("Unhandled callback event type: %T", ev)
				}
			} else {
				logger.Debug("Unhandled event type: %s", eventsAPIEvent.Type)
			}
		case socketmode.EventTypeSlashCommand:
			cmd, ok := evt.Data.(slack.SlashCommand)
			if !ok {
				logger.Debug("Failed to cast slash command")
				continue
			}

			client.Ack(*evt.Request)

			switch cmd.Command {
			case "/query":
				go handleQueryCommand(app, cmd)
			}
		default:
			logger.Debug("Unhandled event type: %v", evt.Type)
		}
	}
}

func handleQueryCommand(app *App, cmd slack.SlashCommand) {
	// Parse the query using OpenAI
	queryResp, err := app.openAI.ParseQuery(cmd.Text)
	if err != nil {
		logger.Error("Failed to parse query: %v", err)
		return
	}

	if queryResp.Error != "" {
		app.slackClient.PostEphemeral(
			cmd.ChannelID,
			cmd.UserID,
			slack.MsgOptionText("❌ "+queryResp.Error, false),
		)
		return
	}

	var blocks []slack.Block
	blocks = append(blocks, slack.NewHeaderBlock(
		slack.NewTextBlockObject("plain_text", "📊 Leave Statistics Report", false, false),
	))

	switch queryResp.QueryType {
	case "top_employee":
		// Get employee with highest leaves
		stat, err := app.leaveRepo.GetTopLeaveEmployee()
		if err != nil {
			logger.Error("Failed to get top leave employee: %v", err)
			blocks = append(blocks, slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", "❌ "+err.Error(), false, false),
				nil, nil,
			))
		} else {
			blocks = append(blocks, slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn",
					fmt.Sprintf("👑 *Employee with Most Leaves*\n\n"+
						"*%s*\n"+
						"• Leave Count: %d\n"+
						"• Types: %s\n"+
						"• Total Hours: %.1f",
						stat.Username,
						stat.LeaveCount,
						stat.LeaveTypes,
						stat.TotalHours),
					false, false),
				nil, nil,
			))
		}

	case "employee_stats":
		// Get stats for specific employee
		stats, err := app.leaveRepo.GetEmployeeStats(queryResp.Username)
		if err != nil {
			logger.Error("Failed to get employee stats: %v", err)
			blocks = append(blocks, slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn", "❌ "+err.Error(), false, false),
				nil, nil,
			))
		} else {
			if len(stats) == 0 {
				blocks = append(blocks, slack.NewSectionBlock(
					slack.NewTextBlockObject("mrkdwn",
						fmt.Sprintf("No leave records found for *%s*. Please check if the username is correct or if they have taken any leave.", queryResp.Username),
						false, false),
					nil, nil,
				))
			} else {
				// Format employee stats
				// ... format blocks for employee stats ...
			}
		}

	case "period_stats":
		// Use the dates from query response
		startDate := queryResp.StartDate
		endDate := queryResp.EndDate

		// Parse the string dates back to time.Time
		startDateParsed, err := time.Parse("2006-01-02", startDate)
		if err != nil {
			fmt.Printf("Error parsing start date: %v\n", err)
			return
		}

		endDateParsed, err := time.Parse("2006-01-02", endDate)
		if err != nil {
			fmt.Printf("Error parsing end date: %v\n", err)
			return
		}

		var stats []repository.LeaveStats
		stats, err = app.leaveRepo.GetLeaveStatsByPeriod(startDateParsed, endDateParsed)
		if err != nil {
			logger.Error("Failed to get leave stats: %v", err)
			return
		}

		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("*Period:* %s to %s",
					startDateParsed.Format("Jan 2, 2006"),
					endDateParsed.Format("Jan 2, 2006")),
				false, false),
			nil, nil,
		))

		for _, stat := range stats {
			blocks = append(blocks, slack.NewSectionBlock(
				slack.NewTextBlockObject("mrkdwn",
					fmt.Sprintf("*%s*\n"+
						"• Leave Count: %d\n"+
						"• Types: %s\n"+
						"• Total Hours: %.1f",
						stat.Username,
						stat.LeaveCount,
						stat.LeaveTypes,
						stat.TotalHours),
					false, false),
				nil, nil,
			))
		}
	}

	// Post the message
	_, _, err = app.slackClient.PostMessage(
		cmd.ChannelID,
		slack.MsgOptionBlocks(blocks...),
	)

	if err != nil {
		logger.Error("Failed to post query response: %v", err)
		app.slackClient.PostEphemeral(
			cmd.ChannelID,
			cmd.UserID,
			slack.MsgOptionText("❌ Failed to get leave statistics", false),
		)
	}
}

type LeaveRequest struct {
	Message string `json:"message"`
}

func (a *App) handleLeaveRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LeaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	response, err := a.openAI.ParseLeaveRequest(req.Message, fmt.Sprintf("%d", time.Now().Unix()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *App) handleLeaveQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get the current time in IST
	loc, _ := time.LoadLocation("Asia/Kolkata")
	now := time.Now().In(loc)

	// Default to last month
	startDate := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, loc)
	endDate := startDate.AddDate(0, 1, 0).Add(-time.Second)

	// Get leave statistics
	stats, err := a.leaveRepo.GetLeaveStatsByPeriod(startDate, endDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Format response
	response := struct {
		Period struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"period"`
		Stats []repository.LeaveStats `json:"stats"`
	}{
		Period: struct {
			Start string `json:"start"`
			End   string `json:"end"`
		}{
			Start: startDate.Format("2006-01-02"),
			End:   endDate.Format("2006-01-02"),
		},
		Stats: stats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func main() {
	logger.Info("Starting application... 🚀")

	config, err := loadConfig()
	if err != nil {
		logger.Error("Failed to load config: %v", err)
		os.Exit(1)
	}

	db, err := initDB(config)
	if err != nil {
		logger.Error("Failed to initialize database: %v", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("Database connected successfully 🗄️")

	app := NewApp(config, db)

	// Add HTTP endpoints
	http.HandleFunc("/api/leave", app.handleLeaveRequest)
	http.HandleFunc("/api/leave/query", app.handleLeaveQuery)
	go http.ListenAndServe(":"+config.Port, nil)

	if err := setupSocketModeHandler(app, config); err != nil {
		logger.Error("Socket mode error: %v", err)
		os.Exit(1)
	}
}
