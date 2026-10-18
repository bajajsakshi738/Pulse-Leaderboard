import {
  Component,
  Input,
  OnChanges,
  OnDestroy,
  SimpleChanges,
  signal,
} from '@angular/core';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-score-ticker',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './score-ticker.component.html',
  styleUrl: './score-ticker.component.scss',
})
export class ScoreTickerComponent implements OnChanges, OnDestroy {
  @Input({ required: true }) value!: number;

  readonly direction = signal<'up' | 'down' | null>(null);
  private previousValue: number | null = null;
  private flashTimeout: ReturnType<typeof setTimeout> | null = null;

  ngOnChanges(changes: SimpleChanges): void {
    if (!changes['value']) return;

    const newValue = changes['value'].currentValue as number;

    if (this.previousValue !== null && newValue !== this.previousValue) {
      this.direction.set(newValue > this.previousValue ? 'up' : 'down');

      if (this.flashTimeout) clearTimeout(this.flashTimeout);
      this.flashTimeout = setTimeout(() => this.direction.set(null), 700);
    }

    this.previousValue = newValue;
  }

  ngOnDestroy(): void {
    // clear the timeout, or it can fire after the row is gone and update a
    // destroyed component.
    if (this.flashTimeout) {
      clearTimeout(this.flashTimeout);
      this.flashTimeout = null;
    }
  }
}
