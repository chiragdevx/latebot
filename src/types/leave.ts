export interface OpenAILeaveResponse {
  start_time: string;
  end_time: string;
  duration: string;
  reason?: string;
  is_leave_request: boolean;
  is_running_late: boolean;
  is_working_from_home: boolean;
  is_valid: boolean;
}
