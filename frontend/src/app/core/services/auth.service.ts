import { Injectable, signal, computed } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, tap } from 'rxjs';
import { environment } from '../../../environments/environment';
import { AuthResponse } from '../models/api.models';

const TOKEN_STORAGE_KEY = 'pulse_leaderboard_token';
const USER_STORAGE_KEY = 'pulse_leaderboard_user';

export interface StoredUser {
  id: string;
  username: string;
  email: string;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  private readonly tokenSignal = signal<string | null>(this.readToken());
  private readonly userSignal = signal<StoredUser | null>(this.readUser());

  readonly isAuthenticated = computed(() => this.tokenSignal() !== null);
  readonly currentUser = computed(() => this.userSignal());

  constructor(private http: HttpClient) {}

  register(username: string, email: string, password: string): Observable<AuthResponse> {
    return this.http
      .post<AuthResponse>(`${environment.apiBaseUrl}/api/auth/register`, {
        username,
        email,
        password,
      })
      .pipe(tap((res) => this.setSession(res)));
  }

  login(email: string, password: string): Observable<AuthResponse> {
    return this.http
      .post<AuthResponse>(`${environment.apiBaseUrl}/api/auth/login`, {
        email,
        password,
      })
      .pipe(tap((res) => this.setSession(res)));
  }

  logout(): void {
    // tell the server first so it blocks this token. we log out here either
    // way: if the call fails the token keeps working until it expires.
    const token = this.tokenSignal();
    if (token) {
      this.http
        .post(
          `${environment.apiBaseUrl}/api/auth/logout`,
          {},
          { headers: { Authorization: `Bearer ${token}` } },
        )
        .subscribe({ error: () => {} });
    }

    localStorage.removeItem(TOKEN_STORAGE_KEY);
    localStorage.removeItem(USER_STORAGE_KEY);
    this.tokenSignal.set(null);
    this.userSignal.set(null);
  }

  getToken(): string | null {
    return this.tokenSignal();
  }

  private setSession(res: AuthResponse): void {
    localStorage.setItem(TOKEN_STORAGE_KEY, res.token);
    localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(res.user));
    this.tokenSignal.set(res.token);
    this.userSignal.set(res.user);
  }

  private readToken(): string | null {
    if (typeof localStorage === 'undefined') return null;
    return localStorage.getItem(TOKEN_STORAGE_KEY);
  }

  private readUser(): StoredUser | null {
    if (typeof localStorage === 'undefined') return null;
    const raw = localStorage.getItem(USER_STORAGE_KEY);
    if (!raw) return null;
    try {
      return JSON.parse(raw) as StoredUser;
    } catch {
      return null;
    }
  }
}
