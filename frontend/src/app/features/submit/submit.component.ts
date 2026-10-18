import { Component, OnInit, computed, signal } from '@angular/core';
import { DecimalPipe } from '@angular/common';
import { FormControl, ReactiveFormsModule, Validators } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { of } from 'rxjs';
import { catchError, map, switchMap } from 'rxjs/operators';

import { LeaderboardApiService } from '../../core/services/leaderboard-api.service';
import {
  GAME_LABELS,
  MAX_POINTS_PER_SUBMISSION,
  PLAYABLE_GAME_CODES,
  gameClass,
} from '../../core/models/api.models';

interface SubmitResult {
  game: string;
  points: number;
  total: number;
  /** Rank before the score; null if the player was not ranked yet. */
  before: number | null;
  /** Rank after the score; null if it could not be read. */
  after: number | null;
}

@Component({
  selector: 'app-submit',
  standalone: true,
  imports: [ReactiveFormsModule, RouterLink, DecimalPipe],
  templateUrl: './submit.component.html',
  styleUrl: './submit.component.scss',
})
export class SubmitComponent implements OnInit {
  readonly gameClass = gameClass;
  readonly labels = GAME_LABELS;
  readonly games = PLAYABLE_GAME_CODES.map((code) => ({ code, label: GAME_LABELS[code] }));
  readonly maxPoints = MAX_POINTS_PER_SUBMISSION;

  readonly game = signal<string>(PLAYABLE_GAME_CODES[0]);
  readonly isSubmitting = signal(false);
  readonly errorMessage = signal<string | null>(null);
  readonly result = signal<SubmitResult | null>(null);

  readonly points = new FormControl<number | null>(100, {
    validators: [Validators.required, Validators.min(1), Validators.max(MAX_POINTS_PER_SUBMISSION)],
  });

  /** "12 places up", "Rank unchanged", or a first-ranking message. */
  readonly rankChange = computed(() => {
    const r = this.result();
    if (!r || r.after === null) return null;
    if (r.before === null) return { text: `You are now ranked #${r.after}`, moved: true };
    const diff = r.before - r.after;
    if (diff > 0) return { text: `${diff} ${diff === 1 ? 'place' : 'places'} up`, moved: true };
    return { text: 'Rank unchanged', moved: false };
  });

  constructor(
    private api: LeaderboardApiService,
    private route: ActivatedRoute,
  ) {}

  ngOnInit(): void {
    const requested = this.route.snapshot.queryParamMap.get('game');
    if (requested && (PLAYABLE_GAME_CODES as readonly string[]).includes(requested)) {
      this.game.set(requested);
    }
  }

  chooseGame(code: string): void {
    this.game.set(code);
    this.result.set(null);
    this.errorMessage.set(null);
  }

  submit(): void {
    if (this.points.invalid) {
      this.points.markAsTouched();
      return;
    }

    const game = this.game();
    const points = this.points.value as number;
    this.isSubmitting.set(true);
    this.errorMessage.set(null);

    // Read the rank first so we can show how far the score moved you.
    this.api
      .getMyRank(game)
      .pipe(
        catchError(() => of(null)),
        switchMap((before) =>
          this.api.submitScore({ game_code: game, points }).pipe(map(() => before)),
        ),
        switchMap((before) =>
          this.api.getMyRank(game).pipe(
            catchError(() => of(null)),
            map((after) => ({ before, after })),
          ),
        ),
      )
      .subscribe({
        next: ({ before, after }) => {
          const beforeRank = before && before.rank > 0 ? before.rank : null;
          const afterRank = after && after.rank > 0 ? after.rank : null;
          this.result.set({
            game,
            points,
            total: after ? after.score : (before?.score ?? 0) + points,
            before: beforeRank,
            after: afterRank,
          });
          this.isSubmitting.set(false);
        },
        error: (err) => {
          this.isSubmitting.set(false);
          this.errorMessage.set(
            err?.status === 0
              ? 'Could not reach the server. Check that the backend is running.'
              : (err?.error?.error ?? 'Score submission failed. Please try again.'),
          );
        },
      });
  }
}
