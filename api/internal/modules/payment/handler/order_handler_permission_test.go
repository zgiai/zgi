package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	platformconsole "github.com/zgiai/ginext/internal/infra/platform/console"
)

const (
	testPermissionAccountID      = "2f494361-67a0-4371-a0bf-8f357fbb140b"
	testPermissionOrganizationID = "fd97bd62-ca46-4901-b381-142d5c9a214b"
)

func TestOrderHandler_CreateWalletRechargeAndPay_NonAdminPermissionForbidden(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	handler := &OrderHandler{
		organizationAdminChecker: func(ctx context.Context, organizationID, accountID string) (bool, error) {
			return false, nil
		},
	}

	c, w := newOrderPermissionContext(http.MethodPost, "/orders/recharge/pay", `{"amount":10,"payment_method":"alipay"}`)
	handler.CreateWalletRechargeAndPay(c)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "Only group admins can recharge wallet")
}

func TestOrderHandler_CreateAICreditAndPay_NonAdminPermissionForbidden(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	handler := &OrderHandler{
		organizationAdminChecker: func(ctx context.Context, organizationID, accountID string) (bool, error) {
			return false, nil
		},
	}

	c, w := newOrderPermissionContext(http.MethodPost, "/orders/ai-credits/pay", `{"product_id":"0f7f09b2-9f55-4b3f-956d-5f570d4fe6d3","payment_method":"alipay"}`)
	handler.CreateAICreditAndPay(c)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "Only group admins can purchase AI credits")
}

func TestOrderHandler_CreateAICreditAndPay_QuantityMustBeOne(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	handler := &OrderHandler{
		organizationAdminChecker: func(ctx context.Context, organizationID, accountID string) (bool, error) {
			return true, nil
		},
	}

	c, w := newOrderPermissionContext(http.MethodPost, "/orders/ai-credits/pay", `{"product_id":"0f7f09b2-9f55-4b3f-956d-5f570d4fe6d3","quantity":2,"payment_method":"alipay"}`)
	handler.CreateAICreditAndPay(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "quantity must be 1")
}

func TestOrderHandler_CreateAICreditAndPay_ProxiesProductIDOnly(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	consoleProvider := &capturingConsoleProvider{Standalone: platformconsole.NewStandalone()}
	handler := &OrderHandler{
		consoleProvider: consoleProvider,
		organizationAdminChecker: func(ctx context.Context, organizationID, accountID string) (bool, error) {
			return true, nil
		},
	}

	c, w := newOrderPermissionContext(http.MethodPost, "/orders/ai-credits/pay", `{
		"product_id":" 0f7f09b2-9f55-4b3f-956d-5f570d4fe6d3 ",
		"quantity":1,
		"payment_method":"alipay",
		"payment_sub_method":"qrcode",
		"use_wallet_balance":true,
		"wallet_deduction_amount":20
	}`)
	handler.CreateAICreditAndPay(c)

	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, consoleProvider.checkoutReq)
	require.Equal(t, testPermissionOrganizationID, consoleProvider.checkoutReq.TenantID)
	require.Equal(t, testPermissionAccountID, consoleProvider.checkoutReq.AccountID)
	require.Equal(t, orderTypeCreditPurchase, consoleProvider.checkoutReq.OrderType)
	require.Equal(t, "0f7f09b2-9f55-4b3f-956d-5f570d4fe6d3", consoleProvider.checkoutReq.ProductID)
	require.Empty(t, consoleProvider.checkoutReq.ProductCode)
	require.Equal(t, "alipay", consoleProvider.checkoutReq.PaymentMethod)
	require.Equal(t, "qrcode", consoleProvider.checkoutReq.PaymentSubMethod)
	require.True(t, consoleProvider.checkoutReq.UseWalletBalance)
	require.Equal(t, float64(20), consoleProvider.checkoutReq.WalletDeductionAmount)
	require.Zero(t, consoleProvider.checkoutReq.Amount)
	require.Zero(t, consoleProvider.checkoutReq.CreditAmount)
	require.Empty(t, consoleProvider.checkoutReq.ProductName)
	require.Empty(t, consoleProvider.checkoutReq.Currency)
}

func TestOrderHandler_CreateAICreditAndPay_RejectsProductCodeOnly(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	consoleProvider := &capturingConsoleProvider{Standalone: platformconsole.NewStandalone()}
	handler := &OrderHandler{
		consoleProvider: consoleProvider,
		organizationAdminChecker: func(ctx context.Context, organizationID, accountID string) (bool, error) {
			return true, nil
		},
	}

	c, w := newOrderPermissionContext(http.MethodPost, "/orders/ai-credits/pay", `{"product_code":"credits_1m","payment_method":"alipay"}`)
	handler.CreateAICreditAndPay(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Nil(t, consoleProvider.checkoutReq)
}

func TestAICreditHandler_ListProducts_ProxiesConsoleProducts(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	consoleProvider := &capturingConsoleProvider{
		Standalone: platformconsole.NewStandalone(),
		products: []*platformconsole.CreditProductInfo{
			{
				ID:           "product-id",
				ProductCode:  "credits_1m",
				ProductName:  "Basic Credits Pack",
				CreditAmount: 1000000000,
				Price:        200,
				Currency:     "CNY",
				DisplayOrder: 1,
				IsActive:     true,
			},
		},
	}
	handler := &AICreditHandler{consoleProvider: consoleProvider}

	c, w := newOrderPermissionContext(http.MethodGet, "/ai-credit-products", "")
	handler.ListProducts(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "credits_1m")
}

func TestOrderHandler_RegisterRoutes_IncludesAICreditPay(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	handler := &OrderHandler{}
	require.NotPanics(t, func() {
		handler.RegisterRoutes(router.Group("/api/v1"))
	})
}

type capturingConsoleProvider struct {
	*platformconsole.Standalone
	checkoutReq *platformconsole.PaymentCheckoutRequest
	products    []*platformconsole.CreditProductInfo
}

func (p *capturingConsoleProvider) IsAvailable() bool {
	return true
}

func (p *capturingConsoleProvider) GetMode() string {
	return "CLOUD"
}

func (p *capturingConsoleProvider) PaymentCheckout(ctx context.Context, req *platformconsole.PaymentCheckoutRequest) (*platformconsole.PaymentCheckoutResponse, error) {
	p.checkoutReq = req
	return &platformconsole.PaymentCheckoutResponse{
		OrderID:       "order-id",
		OrderNo:       "order-no",
		OrderAmount:   "200.00",
		QRCodeContent: "https://qr.example/pay",
		Status:        "pending",
	}, nil
}

func (p *capturingConsoleProvider) ListCreditProducts(ctx context.Context) ([]*platformconsole.CreditProductInfo, error) {
	return p.products, nil
}

func newOrderPermissionContext(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("account_id", testPermissionAccountID)
	c.Set("tenant_id", testPermissionOrganizationID)
	return c, recorder
}
