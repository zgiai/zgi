export interface NotificationSMSDraft {
  recipients: string[];
  notificationTitle: string;
  linkCode: string;
}

export interface NotificationSMSErrors {
  recipients?: string;
  notificationTitle?: string;
  linkCode?: string;
}
