import { Injectable, OnDestroy } from '@angular/core';
import { Observable, Subject } from 'rxjs';
import { environment } from '../../../environments/environment';
import { LeaderboardBroadcastMessage } from '../models/api.models';

/**
 * One WebSocket connection to /ws/leaderboard. Incoming leaderboard_update
 * messages come out as an Observable. If the connection drops it reconnects
 * with a growing delay, so the page recovers without a reload.
 */
@Injectable({ providedIn: 'root' })
export class LeaderboardSocketService implements OnDestroy {
  private socket: WebSocket | null = null;
  private readonly messages$ = new Subject<LeaderboardBroadcastMessage>();
  private readonly connectionStatus$ = new Subject<'connected' | 'disconnected'>();

  private reconnectAttempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private manuallyClosed = false;

  connect(): void {
    this.manuallyClosed = false;
    this.openSocket();
  }

  /** Observable stream of live leaderboard broadcasts from the server. */
  onMessage(): Observable<LeaderboardBroadcastMessage> {
    return this.messages$.asObservable();
  }

  /** Observable stream of connection status changes, for UI indicators. */
  onStatusChange(): Observable<'connected' | 'disconnected'> {
    return this.connectionStatus$.asObservable();
  }

  disconnect(): void {
    this.manuallyClosed = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.socket?.close();
    this.socket = null;
  }

  private openSocket(): void {
    this.socket = new WebSocket(environment.wsUrl);

    this.socket.onopen = () => {
      this.reconnectAttempt = 0;
      this.connectionStatus$.next('connected');
    };

    this.socket.onmessage = (event: MessageEvent) => {
      try {
        const parsed = JSON.parse(event.data) as LeaderboardBroadcastMessage;
        this.messages$.next(parsed);
      } catch {
        // Ignore malformed frames rather than crashing the stream.
      }
    };

    this.socket.onclose = () => {
      this.connectionStatus$.next('disconnected');
      if (!this.manuallyClosed) {
        this.scheduleReconnect();
      }
    };

    this.socket.onerror = () => {
      // onclose will fire next and handle reconnection.
    };
  }

  private scheduleReconnect(): void {
    const delayMs = Math.min(1000 * 2 ** this.reconnectAttempt, 15000);
    this.reconnectAttempt += 1;

    this.reconnectTimer = setTimeout(() => {
      if (!this.manuallyClosed) {
        this.openSocket();
      }
    }, delayMs);
  }

  ngOnDestroy(): void {
    this.disconnect();
  }
}
