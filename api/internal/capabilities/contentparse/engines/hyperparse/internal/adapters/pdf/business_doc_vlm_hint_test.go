package pdf

import (
	"strings"
	"testing"
)

func TestBuildBusinessDocVLMRouteHint_rawMarker(t *testing.T) {
	data := []byte("%PDF-1.4\n%âãÏÓ\n电子发票 blah")
	h := BuildBusinessDocVLMRouteHint(data, "", nil)
	if !h.Suggest || !stringSliceContains(h.Kinds, "e_invoice") {
		t.Fatalf("got %+v", h)
	}
}

func TestBuildBusinessDocVLMRouteHint_metadata(t *testing.T) {
	b := &BasicInfo{Title: "项目验收报告"}
	h := BuildBusinessDocVLMRouteHint([]byte("%PDF-1.4\n"), "", b)
	if h.Suggest {
		t.Fatalf("unexpected: %+v", h)
	}
	b2 := &BasicInfo{Subject: "电子发票"}
	h2 := BuildBusinessDocVLMRouteHint([]byte("%PDF-1.4\n"), "", b2)
	if !h2.Suggest {
		t.Fatalf("expected metadata hit: %+v", h2)
	}
	b3 := &BasicInfo{Title: "增值税电子普通发票"}
	h3 := BuildBusinessDocVLMRouteHint([]byte("%PDF-1.4\n"), "", b3)
	if !h3.Suggest {
		t.Fatalf("expected title phrase: %+v", h3)
	}
}

func TestBuildBusinessDocVLMRouteHint_textBuckets(t *testing.T) {
	text := "发票代码 12 发票号码 34 开票日期 价税合计 销方名称 购方名称"
	h := BuildBusinessDocVLMRouteHint([]byte("%PDF-1.4\n"), text, nil)
	if !h.Suggest || !stringSliceContains(h.Kinds, "e_invoice") {
		t.Fatalf("got %+v", h)
	}
	found := false
	for _, r := range h.Reasons {
		if strings.HasPrefix(r, "text_e_invoice_fields:") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("reasons: %v", h.Reasons)
	}
}

func TestBuildBusinessDocVLMRouteHint_statement(t *testing.T) {
	text := "对账单 账号 6222 户名 测试 起止日期 账户余额 100"
	h := BuildBusinessDocVLMRouteHint([]byte("%PDF-1.4\n"), text, nil)
	if !h.Suggest || !stringSliceContains(h.Kinds, "account_statement") {
		t.Fatalf("got %+v", h)
	}
}

func TestBuildBusinessDocVLMRouteHint_bankFlow(t *testing.T) {
	text := "交易明细 收入金额 支出金额 账户余额"
	h := BuildBusinessDocVLMRouteHint([]byte("%PDF-1.4\n"), text, nil)
	if !h.Suggest || !stringSliceContains(h.Kinds, "bank_transaction_list") {
		t.Fatalf("got %+v", h)
	}
}

func TestBuildBusinessDocVLMRouteHint_metadataEnglishBusinessForm(t *testing.T) {
	b := &BasicInfo{Title: "Medical Claim Form"}
	h := BuildBusinessDocVLMRouteHint([]byte("%PDF-1.4\n"), "", b)
	if !h.Suggest || !stringSliceContains(h.Kinds, "business_form") {
		t.Fatalf("got %+v", h)
	}
}

func TestBuildBusinessDocVLMRouteHint_textEnglishStatement(t *testing.T) {
	text := strings.Join([]string{
		"Billing Statement",
		"Account Number: 123456789",
		"Statement Period: 2025-03-01 to 2025-03-31",
		"Amount Due: $120.50",
		"Current Balance: $302.10",
	}, "\n")
	h := BuildBusinessDocVLMRouteHint([]byte("%PDF-1.4\n"), text, nil)
	if !h.Suggest || !stringSliceContains(h.Kinds, "account_statement") {
		t.Fatalf("got %+v", h)
	}
}

func TestBuildBusinessDocVLMRouteHint_textEnglishBusinessForm(t *testing.T) {
	text := strings.Join([]string{
		"Medical Claim",
		"Patient Name: Jane Doe",
		"Member ID: AB123456",
		"Claim Number: CLM-123456",
		"Service Date: 2025-03-01",
		"Amount Claimed: $420.00",
		"Provider: City Clinic",
		"Accept Assignment: ☑",
		"Preauthorized: ☑",
	}, "\n")
	h := BuildBusinessDocVLMRouteHint([]byte("%PDF-1.4\n"), text, nil)
	if !h.Suggest || !stringSliceContains(h.Kinds, "business_form") {
		t.Fatalf("got %+v", h)
	}
}

func TestBuildBusinessDocVLMRouteHint_textInvoiceWordAloneDoesNotTrigger(t *testing.T) {
	text := "Invoice"
	h := BuildBusinessDocVLMRouteHint([]byte("%PDF-1.4\n"), text, nil)
	if h.Suggest {
		t.Fatalf("unexpected: %+v", h)
	}
}
