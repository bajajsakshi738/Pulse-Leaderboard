import { Component, OnInit, computed, signal } from '@angular/core';
import { DecimalPipe } from '@angular/common';

import { LeaderboardApiService } from '../../core/services/leaderboard-api.service';
import { NotificationService } from '../../core/services/notification.service';
import {
  GAME_LABELS,
  PLAYABLE_GAME_CODES,
  TopPlayerReportRow,
  gameClass,
} from '../../core/models/api.models';

type Period = 'day' | 'week' | 'month' | 'all';

interface Segment {
  code: string;
  label: string;
  points: number;
  widthPct: number;
}

interface PlayerBar {
  userId: string;
  username: string;
  total: number;
  segments: Segment[];
}

/** How many players to chart. */
const MAX_PLAYERS = 12;

@Component({
  selector: 'app-reports',
  standalone: true,
  imports: [DecimalPipe],
  templateUrl: './reports.component.html',
  styleUrl: './reports.component.scss',
})
export class ReportsComponent implements OnInit {
  readonly gameClass = gameClass;
  readonly periods: { value: Period; label: string }[] = [
    { value: 'day', label: '24 hours' },
    { value: 'week', label: '7 days' },
    { value: 'month', label: '30 days' },
    { value: 'all', label: 'All time' },
  ];
  readonly gameOptions = [
    { code: '', label: 'All games' },
    ...PLAYABLE_GAME_CODES.map((code) => ({ code, label: GAME_LABELS[code] })),
  ];
  readonly legend = PLAYABLE_GAME_CODES.map((code) => ({ code, label: GAME_LABELS[code] }));

  readonly period = signal<Period>('week');
  readonly selectedGame = signal<string>('');
  readonly rows = signal<TopPlayerReportRow[]>([]);
  readonly isLoading = signal(true);

  /** One bar per player, split by game, biggest total first. */
  readonly players = computed<PlayerBar[]>(() => {
    const byUser = new Map<string, PlayerBar>();

    for (const row of this.rows()) {
      let bar = byUser.get(row.user_id);
      if (!bar) {
        bar = { userId: row.user_id, username: row.username, total: 0, segments: [] };
        byUser.set(row.user_id, bar);
      }
      bar.total += row.total_points;
      bar.segments.push({
        code: row.game_code,
        label: GAME_LABELS[row.game_code] ?? row.game_code,
        points: row.total_points,
        widthPct: 0,
      });
    }

    const bars = [...byUser.values()].sort((a, b) => b.total - a.total).slice(0, MAX_PLAYERS);
    const max = bars[0]?.total ?? 0;

    for (const bar of bars) {
      bar.segments.sort(
        (a, b) =>
          (PLAYABLE_GAME_CODES as readonly string[]).indexOf(a.code) -
          (PLAYABLE_GAME_CODES as readonly string[]).indexOf(b.code),
      );
      for (const seg of bar.segments) {
        seg.widthPct = max > 0 ? (seg.points / max) * 100 : 0;
      }
    }
    return bars;
  });

  constructor(
    private api: LeaderboardApiService,
    private notify: NotificationService,
  ) {}

  ngOnInit(): void {
    this.loadReport();
  }

  setPeriod(period: Period): void {
    this.period.set(period);
    this.loadReport();
  }

  setGame(code: string): void {
    this.selectedGame.set(code);
    this.loadReport();
  }

  summary(bar: PlayerBar): string {
    return bar.segments.map((s) => `${s.label} ${s.points}`).join(', ');
  }

  private loadReport(): void {
    this.isLoading.set(true);
    // the API gives one row per player per game, so ask for plenty of rows.
    this.api.getTopPlayersReport(this.period(), this.selectedGame() || undefined, 60).subscribe({
      next: (res) => {
        this.rows.set(res.report ?? []);
        this.isLoading.set(false);
      },
      error: () => {
        this.isLoading.set(false);
        this.notify.error('Could not load the report.');
      },
    });
  }
}
