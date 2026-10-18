import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { environment } from '../../../environments/environment';
import {
  LeaderboardTopResponse,
  RankResponse,
  ScoreEntry,
  SubmitScoreRequest,
  TopPlayersReportResponse,
} from '../models/api.models';

@Injectable({ providedIn: 'root' })
export class LeaderboardApiService {
  private readonly base = environment.apiBaseUrl;

  constructor(private http: HttpClient) {}

  getTop(gameCode: string, n = 10): Observable<LeaderboardTopResponse> {
    return this.http.get<LeaderboardTopResponse>(
      `${this.base}/api/leaderboard/${gameCode}/top`,
      { params: { n } },
    );
  }

  getMyRank(gameCode: string): Observable<RankResponse> {
    return this.http.get<RankResponse>(`${this.base}/api/leaderboard/${gameCode}/me`);
  }

  getRankForUser(gameCode: string, userId: string): Observable<RankResponse> {
    return this.http.get<RankResponse>(
      `${this.base}/api/leaderboard/${gameCode}/rank/${userId}`,
    );
  }

  submitScore(payload: SubmitScoreRequest): Observable<{ score: ScoreEntry }> {
    return this.http.post<{ score: ScoreEntry }>(`${this.base}/api/scores`, payload);
  }

  getMyHistory(gameCode?: string, limit = 50): Observable<{ history: ScoreEntry[] }> {
    const params: Record<string, string | number> = { limit };
    if (gameCode) params['game_code'] = gameCode;
    return this.http.get<{ history: ScoreEntry[] }>(`${this.base}/api/scores/history`, {
      params,
    });
  }

  getTopPlayersReport(
    period: 'day' | 'week' | 'month' | 'all',
    gameCode?: string,
    limit = 10,
  ): Observable<TopPlayersReportResponse> {
    const params: Record<string, string | number> = { period, limit };
    if (gameCode) params['game_code'] = gameCode;
    return this.http.get<TopPlayersReportResponse>(`${this.base}/api/reports/top-players`, {
      params,
    });
  }
}
