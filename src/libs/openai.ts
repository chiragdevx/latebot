import { OpenAI } from "openai";
import { singleton } from "./singleton";

require("dotenv").config();

export const openai = singleton<OpenAI>("openai", () => {
  return new OpenAI({
    apiKey: process.env.OPENAI_API_KEY,
  });
});
