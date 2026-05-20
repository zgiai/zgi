package pdf

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
)

// BusinessDocVLMRouteHint captures lightweight signals for business PDFs such as
// invoices, statements, and transaction records. Suggest=true means render+VLM is preferred.
type BusinessDocVLMRouteHint struct {
	Suggest bool     `json:"suggest"`
	Kinds   []string `json:"kinds,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
}

// CJK lexicon terms are named constants so production Go source stays ASCII
// while preserving multilingual business-document routing quality.
const (
	zhEInvoice             = "\u7535\u5b50\u53d1\u7968"
	zhVatElectronicInvoice = "\u589e\u503c\u7a0e\u7535\u5b50\u666e\u901a\u53d1\u7968"
	zhVatElectronicSpecial = "\u589e\u503c\u7a0e\u7535\u5b50\u4e13\u7528\u53d1\u7968"
	zhDigitalInvoice       = "\u6570\u7535\u53d1\u7968"
	zhFullDigitalInvoice   = "\u5168\u7535\u53d1\u7968"
	zhVatInvoice           = "\u589e\u503c\u7a0e\u53d1\u7968"
	zhElectronicReceipt    = "\u7535\u5b50\u7968\u636e"
	zhInvoice              = "\u53d1\u7968"
	zhBill                 = "\u8d26\u5355"
	zhStatement            = "\u5bf9\u8d26\u5355"
	zhClosingStatement     = "\u7ed3\u5355"
	zhTransactionFlow      = "\u6d41\u6c34"
	zhTransactionDetails   = "\u4ea4\u6613\u660e\u7ec6"
	zhAccountDetails       = "\u8d26\u6237\u660e\u7ec6"
	zhDebit                = "\u501f\u65b9"
	zhCredit               = "\u8d37\u65b9"
	zhMedical              = "\u533b\u7597"
	zhMedicalInsurance     = "\u533b\u4fdd"
	zhPolicy               = "\u4fdd\u5355"
	zhClaim                = "\u7406\u8d54"
	zhAccountNumber        = "\u8d26\u53f7"
	zhAccount              = "\u8d26\u6237"
	zhAccountName          = "\u6237\u540d"
	zhPolicyNumber         = "\u4fdd\u5355\u53f7"
	zhMemberNumber         = "\u4f1a\u5458\u53f7"
	zhDocumentNumber       = "\u5355\u53f7"
	zhInvoiceCode          = "\u53d1\u7968\u4ee3\u7801"
	zhInvoiceNumber        = "\u53d1\u7968\u53f7\u7801"
	zhIssueDate            = "\u5f00\u7968\u65e5\u671f"
	zhTotalTaxIncluded     = "\u4ef7\u7a0e\u5408\u8ba1"
	zhSellerName           = "\u9500\u65b9\u540d\u79f0"
	zhSellerNameLong       = "\u9500\u552e\u65b9\u540d\u79f0"
	zhBuyerName            = "\u8d2d\u65b9\u540d\u79f0"
	zhBuyerNameLong        = "\u8d2d\u4e70\u65b9\u540d\u79f0"
	zhDateRange            = "\u8d77\u6b62\u65e5\u671f"
	zhAccountingDate       = "\u4f1a\u8ba1\u65e5\u671f"
	zhAccountBalance       = "\u8d26\u6237\u4f59\u989d"
	zhTransactionTime      = "\u4ea4\u6613\u65f6\u95f4"
	zhIncomeAmount         = "\u6536\u5165\u91d1\u989d"
	zhExpenseAmount        = "\u652f\u51fa\u91d1\u989d"
	zhBalance              = "\u4f59\u989d"
	zhYear                 = "\u5e74"
	zhMonth                = "\u6708"
	zhDay                  = "\u65e5"
)

var eInvoiceRawMarkers = [][]byte{
	[]byte(zhEInvoice),
	[]byte(zhVatElectronicInvoice),
	[]byte(zhVatElectronicSpecial),
	[]byte(zhDigitalInvoice),
	[]byte(zhFullDigitalInvoice),
}

var (
	businessAmountLikeRE       = regexp.MustCompile(`(?:[$¥￥]\s*)?\d{1,3}(?:,\d{3})*(?:\.\d{2})?`)
	businessDateLikeRE         = regexp.MustCompile("(?:\\b\\d{4}[-/]\\d{1,2}[-/]\\d{1,2}\\b|\\b\\d{1,2}[-/]\\d{1,2}[-/]\\d{2,4}\\b|\\d{4}" + zhYear + "\\d{1,2}" + zhMonth + "\\d{1,2}" + zhDay + ")")
	businessLongValueLikeRE    = regexp.MustCompile(`\b[A-Z0-9-]{6,}\b`)
	businessCheckboxLikeRE     = regexp.MustCompile(`[☑☐□✓✔]`)
	businessStrongMetaTermsRE  = regexp.MustCompile(`(?i)(medical\s+claim\s+form|claim\s+form|explanation\s+of\s+benefits|remittance\s+advice|billing\s+statement|account\s+statement|bank\s+statement|transaction\s+(?:detail|list|history)|invoice\s+(?:statement|summary)|member\s+statement|policy\s+statement)`)
	businessInvoiceLineTerms   = []string{"invoice", "bill", "billing", zhInvoice, zhBill, zhElectronicReceipt}
	businessStatementLineTerms = []string{"statement", "summary", zhStatement, zhBill, zhClosingStatement}
	businessBankFlowLineTerms  = []string{"transaction", "debit", "credit", zhTransactionFlow, zhTransactionDetails, zhAccountDetails, zhDebit, zhCredit}
	businessMedicalLineTerms   = []string{"claim", "patient", "provider", "subscriber", "member", "diagnosis", "insurance", "policy", "medical", zhMedical, zhMedicalInsurance, zhPolicy, zhClaim}
	businessAccountLineTerms   = []string{"account", "acct", "member id", "member number", "policy", "claim", "invoice", "reference", "patient id", "provider id", zhAccountNumber, zhAccount, zhAccountName, zhPolicyNumber, zhMemberNumber, zhDocumentNumber}
)

type businessDocTextSignals struct {
	KVLineCount               int
	AmountLineCount           int
	DateLineCount             int
	AccountLikeLineCount      int
	CheckboxLineCount         int
	InvoiceKeywordLineCount   int
	StatementKeywordLineCount int
	BankFlowKeywordLineCount  int
	MedicalKeywordLineCount   int
}

// BuildBusinessDocVLMRouteHint combines raw markers, PDF metadata, and extracted
// text signals to decide whether VLM should be preferred. It is intentionally
// conservative: structured signals such as amount/date/account/KV lines are
// required instead of a single generic keyword.
func BuildBusinessDocVLMRouteHint(data []byte, combinedText string, basic *BasicInfo) BusinessDocVLMRouteHint {
	var h BusinessDocVLMRouteHint
	if len(data) == 0 {
		return h
	}
	text := combinedText
	meta := ""
	if basic != nil {
		meta = strings.TrimSpace(basic.Title + " " + basic.Subject + " " + basic.Keywords)
	}

	if rawScanEInvoiceMarkers(data) {
		h.Suggest = true
		h.Kinds = append(h.Kinds, "e_invoice")
		h.Reasons = append(h.Reasons, "raw_marker_e_invoice")
	}
	if meta != "" && metaSuggestsEInvoice(meta) {
		if !h.Suggest {
			h.Suggest = true
			h.Kinds = append(h.Kinds, "e_invoice")
		}
		h.Reasons = append(h.Reasons, "metadata_e_invoice")
	}
	if kind, reason := metaSuggestsBusinessForm(meta); kind != "" {
		h.Suggest = true
		h.Kinds = append(h.Kinds, kind)
		h.Reasons = append(h.Reasons, reason)
	}
	if n, detail := eInvoiceTextBuckets(text); n >= 4 {
		h.Suggest = true
		if !stringSliceContains(h.Kinds, "e_invoice") {
			h.Kinds = append(h.Kinds, "e_invoice")
		}
		h.Reasons = append(h.Reasons, "text_e_invoice_fields:"+detail)
	}

	signals := collectBusinessDocTextSignals(text)
	if statementLikeText(text, signals) {
		h.Suggest = true
		h.Kinds = append(h.Kinds, "account_statement")
		h.Reasons = append(h.Reasons, "text_account_statement")
	}
	if bankFlowLikeText(text, signals) {
		h.Suggest = true
		if !stringSliceContains(h.Kinds, "bank_transaction_list") {
			h.Kinds = append(h.Kinds, "bank_transaction_list")
		}
		h.Reasons = append(h.Reasons, "text_bank_flow")
	}
	if ok, detail := genericBusinessFormLikeText(text, signals); ok {
		h.Suggest = true
		if !stringSliceContains(h.Kinds, "business_form") {
			h.Kinds = append(h.Kinds, "business_form")
		}
		h.Reasons = append(h.Reasons, "text_business_form:"+detail)
	}

	h.Kinds = uniqStrings(h.Kinds)
	h.Reasons = uniqStrings(h.Reasons)
	return h
}

func rawScanEInvoiceMarkers(data []byte) bool {
	n := len(data)
	const maxScan = 512 * 1024
	if n > maxScan {
		n = maxScan
	}
	chunk := data[:n]
	for _, m := range eInvoiceRawMarkers {
		if len(m) == 0 {
			continue
		}
		if bytes.Contains(chunk, m) {
			return true
		}
	}
	return false
}

func metaSuggestsEInvoice(meta string) bool {
	if strings.Contains(meta, zhEInvoice) ||
		strings.Contains(meta, zhVatInvoice) ||
		strings.Contains(meta, zhVatElectronicInvoice) ||
		strings.Contains(meta, zhVatElectronicSpecial) ||
		strings.Contains(meta, zhElectronicReceipt) {
		return true
	}
	low := strings.ToLower(meta)
	if strings.Contains(low, "invoice") && strings.Contains(meta, zhInvoice) {
		return true
	}
	return false
}

func metaSuggestsBusinessForm(meta string) (kind string, reason string) {
	meta = strings.TrimSpace(meta)
	if meta == "" {
		return "", ""
	}
	if businessStrongMetaTermsRE.MatchString(meta) {
		return "business_form", "metadata_business_form"
	}
	low := strings.ToLower(meta)
	if strings.Contains(low, "statement") && (strings.Contains(low, "account") || strings.Contains(low, "billing") || strings.Contains(low, "bank")) {
		return "account_statement", "metadata_account_statement"
	}
	return "", ""
}

func normalizeRouteHintText(s string) string {
	if s == "" {
		return ""
	}
	s = strings.NewReplacer("\r", "\n", "\t", " ", "\u00a0", " ").Replace(s)
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(line)), " "))
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func containsAnyNormalized(text string, parts ...string) bool {
	for _, p := range parts {
		p = normalizeRouteHintText(p)
		if p == "" {
			continue
		}
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

func eInvoiceTextBuckets(text string) (score int, detail string) {
	if text == "" {
		return 0, ""
	}
	var parts []string
	if strings.Contains(text, zhInvoiceCode) {
		score++
		parts = append(parts, "code")
	}
	if strings.Contains(text, zhInvoiceNumber) {
		score++
		parts = append(parts, "number")
	}
	if strings.Contains(text, zhIssueDate) {
		score++
		parts = append(parts, "date")
	}
	if strings.Contains(text, zhTotalTaxIncluded) {
		score++
		parts = append(parts, "tax_total")
	}
	if strings.Contains(text, zhSellerName) || strings.Contains(text, zhSellerNameLong) {
		score++
		parts = append(parts, "seller")
	}
	if strings.Contains(text, zhBuyerName) || strings.Contains(text, zhBuyerNameLong) {
		score++
		parts = append(parts, "buyer")
	}
	return score, strings.Join(parts, ",")
}

func collectBusinessDocTextSignals(text string) businessDocTextSignals {
	var sig businessDocTextSignals
	if text == "" {
		return sig
	}
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		low := strings.ToLower(line)
		if looksLikeKeyValueLine(line) {
			sig.KVLineCount++
		}
		if businessAmountLikeRE.MatchString(line) {
			sig.AmountLineCount++
		}
		if businessDateLikeRE.MatchString(line) {
			sig.DateLineCount++
		}
		if businessCheckboxLikeRE.MatchString(line) {
			sig.CheckboxLineCount++
		}
		if lineLooksAccountLike(line, low) {
			sig.AccountLikeLineCount++
		}
		if containsAny(low, businessInvoiceLineTerms) {
			sig.InvoiceKeywordLineCount++
		}
		if containsAny(low, businessStatementLineTerms) {
			sig.StatementKeywordLineCount++
		}
		if containsAny(low, businessBankFlowLineTerms) {
			sig.BankFlowKeywordLineCount++
		}
		if containsAny(low, businessMedicalLineTerms) {
			sig.MedicalKeywordLineCount++
		}
	}
	return sig
}

func statementLikeText(text string, sig businessDocTextSignals) bool {
	if !(strings.Contains(text, zhStatement) || sig.StatementKeywordLineCount > 0) {
		return false
	}
	hasAcct := sig.AccountLikeLineCount > 0 ||
		strings.Contains(text, zhAccountNumber) || strings.Contains(text, zhAccountName) || strings.Contains(text, zhAccount) ||
		strings.Contains(strings.ToLower(text), "account number")
	hasPeriodOrBal := sig.DateLineCount > 0 ||
		strings.Contains(text, zhDateRange) || strings.Contains(text, zhAccountingDate) ||
		strings.Contains(text, zhAccountBalance) || strings.Contains(text, zhTransactionTime) ||
		strings.Contains(strings.ToLower(text), "billing period") ||
		strings.Contains(strings.ToLower(text), "statement period") ||
		strings.Contains(strings.ToLower(text), "balance due") ||
		strings.Contains(strings.ToLower(text), "current balance")
	return hasAcct && hasPeriodOrBal
}

func bankFlowLikeText(text string, sig businessDocTextSignals) bool {
	if !(strings.Contains(text, zhTransactionDetails) || strings.Contains(text, zhAccountDetails) || sig.BankFlowKeywordLineCount > 0) {
		return false
	}
	hasAmt := sig.AmountLineCount >= 2 ||
		(strings.Contains(text, zhIncomeAmount) || strings.Contains(text, zhExpenseAmount)) ||
		(strings.Contains(text, zhDebit) && strings.Contains(text, zhCredit)) ||
		(strings.Contains(strings.ToLower(text), "debit") && strings.Contains(strings.ToLower(text), "credit"))
	hasBal := strings.Contains(text, zhBalance) || strings.Contains(text, zhAccountBalance) ||
		strings.Contains(strings.ToLower(text), "balance")
	return hasAmt && hasBal
}

func genericBusinessFormLikeText(text string, sig businessDocTextSignals) (bool, string) {
	if text == "" {
		return false, ""
	}
	evidence := 0
	parts := make([]string, 0, 6)
	if sig.KVLineCount >= 3 {
		evidence++
		parts = append(parts, "kv="+itoa(sig.KVLineCount))
	}
	if sig.AmountLineCount >= 2 {
		evidence++
		parts = append(parts, "amount="+itoa(sig.AmountLineCount))
	}
	if sig.DateLineCount >= 1 {
		evidence++
		parts = append(parts, "date="+itoa(sig.DateLineCount))
	}
	if sig.AccountLikeLineCount >= 1 {
		evidence++
		parts = append(parts, "account="+itoa(sig.AccountLikeLineCount))
	}
	if sig.CheckboxLineCount >= 2 {
		evidence++
		parts = append(parts, "checkbox="+itoa(sig.CheckboxLineCount))
	}
	if sig.InvoiceKeywordLineCount+sig.StatementKeywordLineCount+sig.MedicalKeywordLineCount >= 1 {
		evidence++
		parts = append(parts, "keywords="+itoa(sig.InvoiceKeywordLineCount+sig.StatementKeywordLineCount+sig.MedicalKeywordLineCount))
	}
	if evidence >= 4 {
		return true, strings.Join(parts, ",")
	}
	if sig.MedicalKeywordLineCount > 0 && sig.KVLineCount >= 2 && (sig.AccountLikeLineCount > 0 || sig.DateLineCount > 0) {
		return true, strings.Join(parts, ",")
	}
	return false, ""
}

func looksLikeKeyValueLine(line string) bool {
	if line == "" {
		return false
	}
	idx := strings.Index(line, ":")
	if idx < 0 {
		idx = strings.Index(line, "：")
	}
	if idx <= 0 || idx > 32 {
		return false
	}
	label := strings.TrimSpace(line[:idx])
	value := strings.TrimSpace(line[idx+1:])
	return label != "" && value != ""
}

func lineLooksAccountLike(line string, low string) bool {
	if containsAny(low, businessAccountLineTerms) && (businessLongValueLikeRE.MatchString(line) || looksLikeKeyValueLine(line)) {
		return true
	}
	return strings.Contains(line, zhAccountNumber) || strings.Contains(line, zhAccount) || strings.Contains(low, "account number")
}

func containsAny(s string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(s, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func stringSliceContains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func uniqStrings(xs []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
}
