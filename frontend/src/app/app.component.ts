import { Component, computed, inject, signal } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router, RouterOutlet } from '@angular/router';
import { filter, map } from 'rxjs/operators';

import { SidebarComponent } from './shared/components/sidebar/sidebar.component';
import { IconComponent } from './shared/components/icon/icon.component';
import { NotificationService } from './core/services/notification.service';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [RouterOutlet, SidebarComponent, IconComponent],
  templateUrl: './app.component.html',
  styleUrl: './app.component.scss',
})
export class AppComponent {
  private readonly router = inject(Router);
  readonly notify = inject(NotificationService);

  readonly menuOpen = signal(false);

  // Start from the real browser path so /login does not flash the sidebar.
  private readonly path = toSignal(
    this.router.events.pipe(
      filter((e): e is NavigationEnd => e instanceof NavigationEnd),
      map((e) => e.urlAfterRedirects),
    ),
    { initialValue: typeof window !== 'undefined' ? window.location.pathname : '/' },
  );

  /** Login and sign-up use the full window, without the sidebar. */
  readonly isAuthPage = computed(() => /^\/(login|register)(\/|\?|$)/.test(this.path()));

  closeMenu(): void {
    this.menuOpen.set(false);
  }
}
