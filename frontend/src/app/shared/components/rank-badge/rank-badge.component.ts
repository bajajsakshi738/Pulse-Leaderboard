import { Component, Input } from '@angular/core';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-rank-badge',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './rank-badge.component.html',
  styleUrl: './rank-badge.component.scss',
})
export class RankBadgeComponent {
  @Input({ required: true }) rank!: number;

  get tier(): 'gold' | 'silver' | 'bronze' | 'standard' {
    if (this.rank === 1) return 'gold';
    if (this.rank === 2) return 'silver';
    if (this.rank === 3) return 'bronze';
    return 'standard';
  }
}
