import { Component, EventEmitter, Input, OnInit, Output, signal } from '@angular/core';
import { Router, RouterLink, RouterLinkActive } from '@angular/router';
import { forkJoin, of } from 'rxjs';
import { catchError, map } from 'rxjs/operators';

import { AuthService } from '../../../core/services/auth.service';
import { LeaderboardApiService } from '../../../core/services/leaderboard-api.service';
import { GAME_CODES, GAME_LABELS, gameClass } from '../../../core/models/api.models';

@Component({
  selector: 'app-sidebar',
  standalone: true,
  imports: [RouterLink, RouterLinkActive],
  templateUrl: './sidebar.component.html',
  styleUrl: './sidebar.component.scss',
})
export class SidebarComponent implements OnInit {
  /** Whether the drawer is open (only matters on narrow screens). */
  @Input() open = false;
  @Output() closeMenu = new EventEmitter<void>();

  readonly games = GAME_CODES.map((code) => ({ code, label: GAME_LABELS[code] }));
  readonly gameClass = gameClass;
  /** Players ranked per board; null until loaded (or if the request fails). */
  readonly counts = signal<Record<string, number | null>>({});

  constructor(
    public auth: AuthService,
    private api: LeaderboardApiService,
    private router: Router,
  ) {}

  ngOnInit(): void {
    const requests = GAME_CODES.map((code) =>
      this.api.getTop(code, 1).pipe(
        map((res) => [code, res.total_ranked] as const),
        catchError(() => of([code, null] as const)),
      ),
    );
    forkJoin(requests).subscribe((pairs) => {
      this.counts.set(Object.fromEntries(pairs));
    });
  }

  logout(): void {
    this.auth.logout();
    this.closeMenu.emit();
    this.router.navigate(['/leaderboard/global']);
  }
}
