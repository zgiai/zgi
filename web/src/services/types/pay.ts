export interface Wallet {
  balance: number;
  currency?: string;
  updated_at?: string;
  available_balance: number;
}

// Official AI Credits from console service
export interface OfficialAiCredits {
  balance: number;
  source: 'console' | string;
}

// Private channel fund item
export interface PrivateChannelFund {
  channel_id: string;
  channel_name: string;
  balance: number;
  status: 'ACTIVE' | 'DEBT' | string;
  currency: string;
  updated_at: string;
}

// Private channel funds aggregation
export interface PrivateChannelFunds {
  total: number;
  channels: PrivateChannelFund[];
}

// AI Credits response from /console/api/ai-credits/me
export interface AiCredits {
  id: string;
  account_id: string;
  group_id: string;

  // Legacy fields (always 0, for backward compatibility)
  subscription_credits: number;
  purchased_credits: number;
  total_earned: number;
  total_spent: number;

  last_reset_at: string | null;
  next_reset_at: string | null;
  created_at: string;
  updated_at: string;

  // New fields - use these for balance display
  official_ai_credits: OfficialAiCredits;
  private_channel_funds: PrivateChannelFunds;
}

// Order and payment related types

export type PaymentMethod = 'wechat' | 'alipay' | 'stripe' | 'wallet';
export type PaymentSubMethod = 'qrcode' | 'native' | 'wap' | 'app' | string;

export interface RechargePaymentRequest {
  amount: number;
  payment_method: PaymentMethod;
  currency?: string;
}

export interface Payment {
  order_id: string;
  order_no: string;
  transaction_no?: string;
  order_amount?: string;
  wallet_deducted_amount?: string;
  external_payable_amount?: string;
  payment_url?: string;
  qr_code_content?: string;
  payment_form?: string;
  app_pay_params?: Record<string, string>;
  status: OrderStatus;
  // Legacy fields for backward compatibility
  payment_id?: string;
  payment_method?: PaymentMethod;
  expires_at?: string;
}

export interface Order {
  id: string;
  order_no: string;
  amount: number;
  currency: string;
  status: OrderStatus;
  order_status?: OrderStatus; // Legacy alias
  created_at: string;
  updated_at?: string;
}

export type OrderStatus =
  | 'pending'
  | 'paid'
  | 'completed'
  | 'cancelled'
  | 'expired'
  | 'failed'
  | 'refunded';

export interface RechargePaymentResponse {
  order: Order;
  payment: Payment;
}

export interface PaymentStatusResponse {
  id: string;
  order_no: string;
  status: OrderStatus;
  paid_at?: string;
  amount?: number;
  currency?: string;
}

export interface AiCreditProduct {
  id: string;
  product_name: string;
  credit_amount: number;
  price: number;
  currency: string;
  product_code: string;
  validity_days?: number | null;
  description?: string | null;
  display_order: number;
  tags: string[];
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export type AiCreditPaymentMethod = PaymentMethod;

export interface BuyAiCreditRequest {
  product_id: string;
  quantity?: number;
  coupon_code?: string;
  payment_method: PaymentMethod;
  payment_sub_method?: PaymentSubMethod;
  use_wallet_balance?: boolean;
  wallet_deduction_amount?: number;
  return_url?: string;
}

export interface BuyAiCreditResponse {
  order: Order;
  payment: Payment;
}

// Bill/Transaction related types

// The bills endpoint now exposes a purchase/refund view only.
export type TransactionType = 'recharge_purchase' | 'other';

export interface TransactionDetail {
  expired_amount: number;
  new_credits: number;
  next_reset_at: string;
  plan_code: string;
  purchased_balance_after: number;
  purchased_balance_before: number;
  reset_type: string;
  subscription_balance_after: number;
  subscription_balance_before: number;
}

export interface Transaction {
  id: string;
  batch_id?: string;
  created_at: string;
  transaction_type: TransactionType;
  detail_text: string;
  recharge_amount: number;
  wallet_change_amount: number;
  balance_after: number;

  // Legacy fields kept for backward compatibility with older consumers.
  amount?: number;
  balance_before?: number;
  currency_type?: string;
  group_id?: string;
  transaction_detail?: TransactionDetail;
  description?: string;
}

export interface TransactionsResponse {
  data: Transaction[];
  has_more: boolean;
  limit: number;
  page: number;
  total: number;
}

export interface BillFilters {
  start_time?: string; // RFC3339 format: 2025-12-01T00:00:00Z
  end_time?: string; // RFC3339 format: 2025-12-01T00:00:00Z
  transaction_type?: TransactionType | 'all';
  keyword?: string;
  page?: number;
  limit?: number;
}

// Monthly statistics
export interface MonthlyStats {
  cash: {
    total_consumed: number;
  };
  credits: {
    total_credits_consumed: number;
    subscription_credits_consumed: number;
    purchased_credits_consumed: number;
  };
}
