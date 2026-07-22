import type { User } from '@/services/types/auth';

export function getUserContactDisplay(...users: Array<User | null | undefined>): string {
  for (const user of users) {
    const email = user?.email?.trim();
    if (email) {
      return email;
    }
  }

  for (const user of users) {
    const mobile = user?.extension?.mobile?.trim();
    if (mobile) {
      return mobile;
    }
  }

  return '';
}
