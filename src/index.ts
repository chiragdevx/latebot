import { App } from "@slack/bolt";
import { OpenAIService } from "./services/openai.service";
import { Leave } from "./models/leave.model";
import { config } from "./utils/env";
import logger from "./libs/logger";
import connectDB from "./libs/db";
import { Pool } from "pg";

connectDB();

const app = new App({
  token: config.slackBotToken,
  socketMode: true,
  appToken: config.slackAppToken,
  signingSecret: config.slackSigningSecret,
  port: config.port,
});

const openaiService = new OpenAIService();

const pool = new Pool({
  connectionString: `postgresql://${config.dbUser}:${config.dbPassword}@localhost:5432/${config.dbName}`,
});

// Test database connection
pool
  .connect()
  .then(() => console.log("Connected to PostgreSQL database"))
  .catch((err) => console.error("Database connection error:", err));

app.message("", async ({ message, say }) => {
  const ts = message.ts;
  let text = "";

  if (message.type === "message" && !message.subtype) {
    text = message.text || "";
  }

  let userResult;
  if ("user" in message) {
    userResult = await app.client.users.info({
      token: config.slackBotToken,
      user: message.user as string,
    });
  }

  const [seconds, microseconds] = ts.split(".").map(Number);
  const timestamp = new Date(
    seconds * 1000 + microseconds / 1000
  ).toLocaleString("en-IN", { timeZone: "Asia/Kolkata" });
  const response = await openaiService.parseLeaveRequest(text, timestamp);

  if (!response?.is_valid) {
    logger.info("Not a leave request", response);
    return;
  }

  const leave = new Leave({
    user: userResult?.user?.name,
    original_text: text,
    start_time: response?.start_time,
    end_time: response?.end_time,
    duration: response?.duration,
    reason: response?.reason,
    is_working_from_home: response?.is_working_from_home,
    is_leave_request: response?.is_leave_request,
    is_running_late: response?.is_running_late,
  });

  await leave.save();

  logger.info("Leave saved successfully", leave);

  console.log("Received message:", message);
});

(async () => {
  await app.start();

  app.logger.info("⚡️ Bolt app is running!");
})();
