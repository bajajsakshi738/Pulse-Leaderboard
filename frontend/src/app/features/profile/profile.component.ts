import { Component, OnInit, computed, signal } from '@angular/core';
import { DatePipe, DecimalPipe } from '@angular/common';
import { RouterLink } from '@angular/router';
import { forkJoin, of } from 'rxjs';
import { catchError } from 'rxjs/operators';

import { LeaderboardApiService } from '../../core/services/leaderboard-api.service';
import { AuthService } from '../../core/services/auth.service';
import { NotificationService } from '../../core/services/notification.service';
import {
  GAME_CODES,
  GAME_LABELS,
  PLAYABLE_GAME_CODES,
  RankResponse,
  ScoreEntry,
  gameClass,
} from '../../core/models/api.models';

interface BoardRank {
  code: string;
  label: string;
  rank: RankResponse | null;
}

/** How many past submissions to load for the history table and stats. */
const HISTORY_LIMIT = 100;

@Component({
  selector: 'app-profile',
  standalone: true,
  imports: [DecimalPipe, DatePipe, RouterLink],
  templateUrl: './profile.component.html',
  styleUrl: './profile.component.scss',
})
export class ProfileComponent implements OnInit {
  readonly gameClass = gameClass;
  readonly labels = GAME_LABELS;
  readonly filters = [
    { code: '', label: 'All games' },
    ...PLAYABLE_GAME_CODES.map((code) => ({ code, label: GAME_LABELS[code] })),
  ];

  readonly boards = signal<BoardRank[]>([]);
  readonly history = signal<ScoreEntry[]>([]);
  readonly isLoading = signal(true);
  readonly filter = signal<string>('');

  readonly totalPoints = computed(
    () => this.boards().find((b) => b.code === 'global')?.rank?.score ?? 0,
  );
  readonly bestScore = computed(() =>
    this.history().reduce((best, s) => Math.max(best, s.points), 0),
  );
  readonly visibleHistory = computed(() => {
    const f = this.filter();
    return f ? this.history().filter((s) => s.game_code === f) : this.history();
  });

  constructor(
    private api: LeaderboardApiService,
    public auth: AuthService,
    private notify: NotificationService,
  ) {}

  ngOnInit(): void {
    forkJoin(
      GAME_CODES.map((code) => this.api.getMyRank(code).pipe(catchError(() => of(null)))),
    ).subscribe((ranks) => {
      this.boards.set(
        GAME_CODES.map((code, i) => ({ code, label: GAME_LABELS[code], rank: ranks[i] })),
      );
    });

    this.api.getMyHistory(undefined, HISTORY_LIMIT).subscribe({
      next: (res) => {
        this.history.set(res.history ?? []);
        this.isLoading.set(false);
      },
      error: () => {
        this.isLoading.set(false);
        this.notify.error('Could not load your score history.');
      },
    });
  }

  setFilter(code: string): void {
    this.filter.set(code);
  }
}
