import { Injectable, signal } from '@angular/core';

export interface Toast {
  id: number;
  kind: 'success' | 'error' | 'info';
  message: string;
}

/** small toast messages. AppComponent shows them and they disappear after a few seconds. */
@Injectable({ providedIn: 'root' })
export class NotificationService {
  readonly toasts = signal<Toast[]>([]);
  private nextId = 1;

  success(message: string): void {
    this.push('success', message, 3500);
  }

  error(message: string): void {
    this.push('error', message, 6000);
  }

  info(message: string): void {
    this.push('info', message, 3500);
  }

  dismiss(id: number): void {
    this.toasts.update((list) => list.filter((t) => t.id !== id));
  }

  private push(kind: Toast['kind'], message: string, durationMs: number): void {
    const id = this.nextId++;
    // keep at most three on screen.
    this.toasts.update((list) => [...list.slice(-2), { id, kind, message }]);
    setTimeout(() => this.dismiss(id), durationMs);
  }
}
