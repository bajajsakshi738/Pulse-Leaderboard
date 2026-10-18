import { Component, signal } from '@angular/core';
import { FormBuilder, FormControl, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { AuthService } from '../../../core/services/auth.service';
import { NotificationService } from '../../../core/services/notification.service';
import { IconComponent } from '../../../shared/components/icon/icon.component';
import { GAME_LABELS, PLAYABLE_GAME_CODES, gameClass } from '../../../core/models/api.models';

interface LoginForm {
  email: FormControl<string>;
  password: FormControl<string>;
}

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [ReactiveFormsModule, RouterLink, IconComponent],
  templateUrl: './login.component.html',
})
export class LoginComponent {
  readonly gameClass = gameClass;
  readonly games = PLAYABLE_GAME_CODES.map((code) => ({ code, label: GAME_LABELS[code] }));

  readonly isSubmitting = signal(false);
  readonly showPassword = signal(false);
  readonly errorMessage = signal<string | null>(null);

  readonly form: FormGroup<LoginForm>;

  constructor(
    private fb: FormBuilder,
    private auth: AuthService,
    private notify: NotificationService,
    private router: Router,
  ) {
    this.form = this.fb.nonNullable.group({
      email: ['', [Validators.required, Validators.email]],
      password: ['', [Validators.required]],
    });
  }

  get email() {
    return this.form.controls.email;
  }

  get password() {
    return this.form.controls.password;
  }

  submit(): void {
    if (this.form.invalid) {
      this.form.markAllAsTouched();
      return;
    }

    const { email, password } = this.form.getRawValue();
    this.isSubmitting.set(true);
    this.errorMessage.set(null);

    this.auth.login(email, password).subscribe({
      next: () => {
        this.isSubmitting.set(false);
        this.notify.success('Welcome back!');
        this.router.navigate(['/leaderboard/global']);
      },
      error: (err) => {
        this.isSubmitting.set(false);
        this.errorMessage.set(
          err?.status === 0
            ? 'Could not reach the server. Check that the backend is running.'
            : (err?.error?.error ?? 'Invalid email or password.'),
        );
      },
    });
  }
}
