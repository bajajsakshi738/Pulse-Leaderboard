import { Routes } from '@angular/router';
import { authGuard } from './core/guards/auth.guard';

export const routes: Routes = [
  {
    path: '',
    pathMatch: 'full',
    redirectTo: 'leaderboard/global',
  },
  {
    path: 'login',
    loadComponent: () =>
      import('./features/auth/login/login.component').then((m) => m.LoginComponent),
  },
  {
    path: 'register',
    loadComponent: () =>
      import('./features/auth/register/register.component').then((m) => m.RegisterComponent),
  },
  {
    path: 'leaderboard',
    pathMatch: 'full',
    redirectTo: 'leaderboard/global',
  },
  {
    path: 'leaderboard/:game',
    loadComponent: () =>
      import('./features/leaderboard/leaderboard.component').then((m) => m.LeaderboardComponent),
  },
  {
    path: 'submit',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./features/submit/submit.component').then((m) => m.SubmitComponent),
  },
  {
    path: 'profile',
    canActivate: [authGuard],
    loadComponent: () =>
      import('./features/profile/profile.component').then((m) => m.ProfileComponent),
  },
  {
    path: 'reports',
    loadComponent: () =>
      import('./features/reports/reports.component').then((m) => m.ReportsComponent),
  },
  {
    path: '**',
    redirectTo: 'leaderboard/global',
  },
];
