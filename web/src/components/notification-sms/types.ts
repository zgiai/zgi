export interface NotificationSMSDraft {
  recipients: string[];
  template: string;
  templateParams: Record<string, string>;
}

export interface NotificationSMSErrors {
  recipients?: string;
  template?: string;
  templateParams?: Record<string, string | undefined>;
}
