import { Component, OnDestroy, OnInit, computed, signal } from '@angular/core';
import { DecimalPipe } from '@angular/common';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { Subscription } from 'rxjs';

import { LeaderboardApiService } from '../../core/services/leaderboard-api.service';
import { LeaderboardSocketService } from '../../core/services/leaderboard-socket.service';
import { AuthService } from '../../core/services/auth.service';
import { NotificationService } from '../../core/services/notification.service';
import {
  GAME_CODES,
  GAME_INFO,
  LeaderboardEntry,
  RankResponse,
  gameClass,
} from '../../core/models/api.models';
import { RankBadgeComponent } from '../../shared/components/rank-badge/rank-badge.component';
import { ScoreTickerComponent } from '../../shared/components/score-ticker/score-ticker.component';

@Component({
  selector: 'app-leaderboard',
  standalone: true,
  imports: [DecimalPipe, RouterLink, RankBadgeComponent, ScoreTickerComponent],
  templateUrl: './leaderboard.component.html',
  styleUrl: './leaderboard.component.scss',
})
export class LeaderboardComponent implements OnInit, OnDestroy {
  readonly gameClass = gameClass;

  /** Which board is showing; comes from the URL (/leaderboard/:game). */
  readonly game = signal<string>('global');
  readonly info = computed(() => GAME_INFO[this.game()]);

  readonly entries = signal<LeaderboardEntry[]>([]);
  readonly totalRanked = signal<number>(0);
  readonly myRank = signal<RankResponse | null>(null);
  readonly isLoading = signal<boolean>(true);

  readonly topThree = computed(() => this.entries().slice(0, 3));
  readonly rest = computed(() => this.entries().slice(3));
  readonly topScore = computed(() => this.entries()[0]?.score ?? 0);
  readonly myUserId = computed(() => this.auth.currentUser()?.id ?? null);
  readonly submitParams = computed(() =>
    this.game() === 'global' ? {} : { game: this.game() },
  );

  private routeSub: Subscription | null = null;
  private socketSub: Subscription | null = null;

  constructor(
    private route: ActivatedRoute,
    private router: Router,
    private api: LeaderboardApiService,
    private socket: LeaderboardSocketService,
    public auth: AuthService,
    private notify: NotificationService,
  ) {}

  ngOnInit(): void {
    this.routeSub = this.route.paramMap.subscribe((params) => {
      const code = params.get('game') ?? 'global';
      if (!(GAME_CODES as readonly string[]).includes(code)) {
        this.router.navigate(['/leaderboard/global'], { replaceUrl: true });
        return;
      }
      this.game.set(code);
      this.loadLeaderboard(code);
      if (this.auth.isAuthenticated()) {
        this.loadMyRank(code);
      } else {
        this.myRank.set(null);
      }
    });

    // The server pushes the Global top 20 over a WebSocket. Other
    // boards are fetched over REST when you open them.
    this.socket.connect();
    this.socketSub = this.socket.onMessage().subscribe((message) => {
      if (this.game() === 'global' && message.game_code === 'global') {
        this.entries.set(message.entries);
        this.totalRanked.update((n) => Math.max(n, message.entries.length));
      }
    });
  }

  ngOnDestroy(): void {
    this.routeSub?.unsubscribe();
    this.socketSub?.unsubscribe();
    this.socket.disconnect();
  }

  private loadLeaderboard(code: string): void {
    this.isLoading.set(true);
    this.api.getTop(code, 20).subscribe({
      next: (res) => {
        // Ignore a slow response for a board the user has already left.
        if (this.game() !== code) return;
        this.entries.set(res.entries ?? []);
        this.totalRanked.set(res.total_ranked);
        this.isLoading.set(false);
      },
      error: () => {
        if (this.game() !== code) return;
        this.isLoading.set(false);
        this.notify.error('Could not load the leaderboard. Is the backend running?');
      },
    });
  }

  private loadMyRank(code: string): void {
    this.api.getMyRank(code).subscribe({
      next: (res) => {
        if (this.game() === code) this.myRank.set(res);
      },
      error: () => {
        if (this.game() === code) this.myRank.set(null);
      },
    });
  }

  percentOfTop(score: number): number {
    const top = this.topScore();
    return top > 0 ? Math.max(2, Math.round((score / top) * 100)) : 0;
  }
}
