import { Component, Input } from '@angular/core';

/** The handful of line icons the app needs, drawn inline (no icon font to load). */
@Component({
  selector: 'app-icon',
  standalone: true,
  template: `
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      @switch (name) {
        @case ('menu') { <path d="M4 7h16M4 12h16M4 17h16" /> }
        @case ('close') { <path d="M6 6l12 12M18 6L6 18" /> }
        @case ('eye') {
          <path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z" />
          <circle cx="12" cy="12" r="3" />
        }
        @case ('eye-off') {
          <path d="M3 3l18 18" />
          <path d="M10.6 5.1A10.8 10.8 0 0112 5c6.4 0 10 7 10 7a17 17 0 01-3.2 4.2M6.6 6.6A16.6 16.6 0 002 12s3.6 7 10 7a10 10 0 004.2-.9" />
          <path d="M9.9 9.9a3 3 0 004.2 4.2" />
        }
      }
    </svg>
  `,
  styles: [':host { display: inline-flex; line-height: 0; }'],
})
export class IconComponent {
  @Input({ required: true }) name!: 'menu' | 'close' | 'eye' | 'eye-off';
}
