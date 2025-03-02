import { pool } from "../libs/db";

export interface LeaveData {
  user?: string;
  original_text: string;
  start_time?: string;
  end_time?: string;
  duration?: string;
  reason?: string;
  is_working_from_home: boolean;
  is_leave_request: boolean;
  is_running_late: boolean;
}

export class Leave {
  private data: LeaveData;

  constructor(data: LeaveData) {
    this.data = data;
  }

  async save(): Promise<void> {
    const query = `
      INSERT INTO leaves (
        user_name,
        original_text,
        start_time,
        end_time,
        duration,
        reason,
        is_working_from_home,
        is_leave_request,
        is_running_late,
        created_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
    `;

    const values = [
      this.data.user || null,
      this.data.original_text,
      this.data.start_time,
      this.data.end_time,
      this.data.duration,
      this.data.reason,
      this.data.is_working_from_home,
      this.data.is_leave_request,
      this.data.is_running_late,
    ];

    try {
      await pool.query(query, values);
    } catch (error) {
      console.error("Error saving leave:", error);
      throw error;
    }
  }
}
