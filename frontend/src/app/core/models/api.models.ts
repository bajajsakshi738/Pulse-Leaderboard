// these match the JSON the Go backend returns (backend/internal/models/models.go).

export interface PublicUser {
  id: string;
  username: string;
  email: string;
  created_at: string;
}

export interface AuthResponse {
  token: string;
  user: {
    id: string;
    username: string;
    email: string;
  };
}

export interface LeaderboardEntry {
  rank: number;
  user_id: string;
  username: string;
  score: number;
}

export interface LeaderboardTopResponse {
  game_code: string;
  entries: LeaderboardEntry[];
  total_ranked: number;
}

export interface RankResponse {
  user_id: string;
  username: string;
  score: number;
  rank: number; // -1 if not ranked yet
}

export interface ScoreEntry {
  id: string;
  user_id: string;
  game_code: string;
  points: number;
  created_at: string;
}

export interface SubmitScoreRequest {
  game_code: string;
  points: number;
}

export interface TopPlayerReportRow {
  user_id: string;
  username: string;
  game_code: string;
  total_points: number;
  submissions: number;
}

export interface TopPlayersReportResponse {
  period: string;
  game_code: string;
  report: TopPlayerReportRow[];
}

export interface LeaderboardBroadcastMessage {
  type: string; // "leaderboard_update"
  game_code: string;
  entries: LeaderboardEntry[];
  timestamp: number;
}

/** Every board, in the order they appear in the sidebar. */
export const GAME_CODES = ['global', 'speed-run', 'trivia', 'puzzle'] as const;
export type GameCode = (typeof GAME_CODES)[number];

/** The games a score can be submitted to (the Global board is the sum of these). */
export const PLAYABLE_GAME_CODES = ['speed-run', 'trivia', 'puzzle'] as const;

export const GAME_LABELS: Record<string, string> = {
  global: 'Global',
  'speed-run': 'Speed Run Challenge',
  trivia: 'Trivia Clash',
  puzzle: 'Puzzle Master',
};

export interface GameInfo {
  code: string;
  label: string;
  description: string;
}

export const GAME_INFO: Record<string, GameInfo> = {
  global: {
    code: 'global',
    label: 'Global',
    description: 'Every point from every game, combined into one overall ranking.',
  },
  'speed-run': {
    code: 'speed-run',
    label: 'Speed Run Challenge',
    description: 'Race the clock. Faster finishes score higher.',
  },
  trivia: {
    code: 'trivia',
    label: 'Trivia Clash',
    description: 'Answer fast, answer right. Points per correct answer.',
  },
  puzzle: {
    code: 'puzzle',
    label: 'Puzzle Master',
    description: 'Solve it clean. Fewer moves, more points.',
  },
};

/** CSS class that sets the game's colour variables (see styles.scss). */
export function gameClass(code: string): string {
  return `g-${code}`;
}

/** Largest score the API accepts in one submission. */
export const MAX_POINTS_PER_SUBMISSION = 10000;
