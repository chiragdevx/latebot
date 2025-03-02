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

	if ev.SubType != "" {
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
		log.Printf("Not a leave request")
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
					if ev.SubType != "" || ev.BotID != "" || ev.ThreadTimeStamp != "" {
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
		default:
			logger.Debug("Unhandled event type: %v", evt.Type)
		}
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

	// Add HTTP endpoint
	http.HandleFunc("/api/leave", app.handleLeaveRequest)
	go http.ListenAndServe(":"+config.Port, nil)

	if err := setupSocketModeHandler(app, config); err != nil {
		logger.Error("Socket mode error: %v", err)
		os.Exit(1)
	}
}
