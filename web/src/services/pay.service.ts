import { BaseService } from '@/lib/http/services';
import type {
  Wallet,
  AiCredits,
  RechargePaymentRequest,
  RechargePaymentResponse,
  PaymentStatusResponse,
  AiCreditProduct,
  BuyAiCreditRequest,
  BuyAiCreditResponse,
  TransactionsResponse,
  BillFilters,
  MonthlyStats,
} from './types/pay';
import type { ApiResponseData } from './types/common';

/**
 * PayService
 * ---------------------------------------------------------------------------
 * Handles wallet balance, AI credits and related payment operations.
 */
class PayService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api',
    });
  }

  /**
   * Get current user wallet info
   * GET /console/api/wallets/me
   */
  getMyWallet(): Promise<ApiResponseData<Wallet>> {
    return this.request('get', '/wallets/me');
  }

  /**
   * Get current user AI credits info
   * GET /console/api/ai-credits/me
   */
  getMyAiCredits(): Promise<ApiResponseData<AiCredits>> {
    return this.request('get', '/ai-credits/me');
  }

  /**
   * Create a recharge order and get payment QR code
   */
  createRechargePayment(
    data: RechargePaymentRequest
  ): Promise<ApiResponseData<RechargePaymentResponse>> {
    return this.request('post', '/orders/recharge/pay', data);
  }

  /**
   * Get payment status by order number
   */
  getPaymentStatus(orderNo: string): Promise<ApiResponseData<PaymentStatusResponse>> {
    return this.request('get', `/orders/${orderNo}/payment-status`);
  }

  /**
   * Get AI credit products
   * GET /v1/public/credit-products
   */
  getAiCreditProducts(): Promise<ApiResponseData<AiCreditProduct[]>> {
    return this.request('get', 'v1/public/credit-products', undefined, {
      endpoint: 'market',
      skipAuth: true,
    });
  }

  /**
   * Buy AI credits
   * POST /console/api/orders/ai-credits/pay
   */
  buyAiCredits(data: BuyAiCreditRequest): Promise<ApiResponseData<BuyAiCreditResponse>> {
    return this.request('post', '/orders/ai-credits/pay', {
      quantity: 1,
      use_wallet_balance: false,
      wallet_deduction_amount: 0,
      ...data,
    });
  }

  /**
   * Get bill transactions with filters
   * GET /console/api/transactions
   */
  getBillTransactions(filters: BillFilters): Promise<ApiResponseData<TransactionsResponse>> {
    const params = new URLSearchParams();
    if (filters.start_time) params.append('start_time', filters.start_time);
    if (filters.end_time) params.append('end_time', filters.end_time);
    if (filters.transaction_type && filters.transaction_type !== 'all') {
      params.append('transaction_type', filters.transaction_type);
    }
    if (filters.keyword) params.append('keyword', filters.keyword);
    if (filters.page) params.append('page', filters.page.toString());
    if (filters.limit) params.append('limit', filters.limit.toString());

    const queryString = params.toString();
    return this.request('get', `/transactions${queryString ? `?${queryString}` : ''}`);
  }

  /**
   * Export bill transactions as Excel
   * GET /console/api/transactions/export
   */
  exportBillTransactions(filters: BillFilters): Promise<Blob> {
    const params = new URLSearchParams();
    if (filters.start_time) params.append('start_time', filters.start_time);
    if (filters.end_time) params.append('end_time', filters.end_time);
    if (filters.transaction_type && filters.transaction_type !== 'all') {
      params.append('transaction_type', filters.transaction_type);
    }
    if (filters.keyword) params.append('keyword', filters.keyword);

    const queryString = params.toString();
    return this.request<Blob>(
      'get',
      `/transactions/export${queryString ? `?${queryString}` : ''}`,
      undefined,
      { responseType: 'blob' }
    );
  }

  /**
   * Get monthly statistics
   * GET /console/api/transactions/monthly-stats
   */
  getMonthlyStats(): Promise<ApiResponseData<MonthlyStats>> {
    return this.request('get', '/transactions/monthly-stats');
  }
}

export const payService = new PayService();
export default payService;
