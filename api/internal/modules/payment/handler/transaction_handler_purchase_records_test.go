package handler

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	platformconsole "github.com/zgiai/ginext/internal/infra/platform/console"
	"github.com/zgiai/ginext/internal/modules/payment/model"
)

func TestMapTransactionTypeToPurchaseEvent(t *testing.T) {
	t.Parallel()

	eventKind, ok := mapTransactionTypeToPurchaseEvent("")
	require.True(t, ok)
	assert.Equal(t, "", eventKind)

	eventKind, ok = mapTransactionTypeToPurchaseEvent(string(model.TransactionTypeRechargePurchase))
	require.True(t, ok)
	assert.Equal(t, "purchase", eventKind)

	eventKind, ok = mapTransactionTypeToPurchaseEvent(string(model.TransactionTypeOther))
	require.True(t, ok)
	assert.Equal(t, "refund", eventKind)

	_, ok = mapTransactionTypeToPurchaseEvent(string(model.TransactionTypeSubscription))
	assert.False(t, ok)
}

func TestMapPurchaseRecordToTransactionResponse(t *testing.T) {
	t.Parallel()

	record := &platformconsole.PaymentPurchaseRecord{
		ID:                 "record-1",
		BatchID:            "ORD-1",
		TransactionType:    string(model.TransactionTypeOther),
		DetailText:         "10K Credits",
		RechargeAmount:     -12.5,
		WalletChangeAmount: 0,
		BalanceAfter:       100,
		CreatedAt:          time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC),
	}

	resp := mapPurchaseRecordToTransactionResponse(record)
	require.NotNil(t, resp)
	assert.Equal(t, string(model.TransactionTypeOther), resp.TransactionType)
	assert.Equal(t, "10K Credits", resp.DetailText)
	assert.Equal(t, -12.5, resp.RechargeAmount)
	assert.Equal(t, 0.0, resp.WalletChangeAmount)
	assert.Equal(t, 100.0, resp.BalanceAfter)
}

func TestMapPurchaseRecordToTransactionResponse_WalletPurchase(t *testing.T) {
	t.Parallel()

	record := &platformconsole.PaymentPurchaseRecord{
		ID:                 "record-2",
		BatchID:            "ORD-2",
		TransactionType:    string(model.TransactionTypeRechargePurchase),
		DetailText:         "100K Credits",
		RechargeAmount:     0,
		WalletChangeAmount: -10,
		BalanceAfter:       90,
		CreatedAt:          time.Date(2026, 4, 7, 11, 0, 0, 0, time.UTC),
	}

	resp := mapPurchaseRecordToTransactionResponse(record)
	require.NotNil(t, resp)
	assert.Equal(t, "100K Credits", resp.DetailText)
	assert.Equal(t, 0.0, resp.RechargeAmount)
	assert.Equal(t, -10.0, resp.WalletChangeAmount)
	assert.Equal(t, 90.0, resp.BalanceAfter)
}

func TestMapPurchaseRecordToTransactionResponse_MixedPurchase(t *testing.T) {
	t.Parallel()

	record := &platformconsole.PaymentPurchaseRecord{
		ID:                 "record-3",
		BatchID:            "ORD-3",
		TransactionType:    string(model.TransactionTypeRechargePurchase),
		DetailText:         "10K Credits",
		RechargeAmount:     5,
		WalletChangeAmount: -5,
		BalanceAfter:       95,
		CreatedAt:          time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC),
	}

	resp := mapPurchaseRecordToTransactionResponse(record)
	require.NotNil(t, resp)
	assert.Equal(t, "10K Credits", resp.DetailText)
	assert.Equal(t, 5.0, resp.RechargeAmount)
	assert.Equal(t, -5.0, resp.WalletChangeAmount)
	assert.Equal(t, 95.0, resp.BalanceAfter)
}

func TestNormalizeTransactionsExportWorkbook_RechargeLabelMatchesListDisplay(t *testing.T) {
	t.Parallel()

	f := excelize.NewFile()
	require.NoError(t, f.SetSheetName("Sheet1", transactionsExportSheetName))
	require.NoError(t, f.SetCellValue(transactionsExportSheetName, "A1", "\u65f6\u95f4"))
	require.NoError(t, f.SetCellValue(transactionsExportSheetName, "B1", "\u4ea4\u6613ID"))
	require.NoError(t, f.SetCellValue(transactionsExportSheetName, "C1", transactionTypeHeaderLabel))
	require.NoError(t, f.SetCellValue(transactionsExportSheetName, "D1", descriptionHeaderLabel))
	require.NoError(t, f.SetCellValue(transactionsExportSheetName, "A2", "2026-04-13"))
	require.NoError(t, f.SetCellValue(transactionsExportSheetName, "B2", "record-1"))
	require.NoError(t, f.SetCellValue(transactionsExportSheetName, "C2", rechargeExportLabel))
	require.NoError(t, f.SetCellValue(transactionsExportSheetName, "D2", rechargeExportLabel))

	buf, err := f.WriteToBuffer()
	require.NoError(t, err)

	normalized, err := normalizeTransactionsExportWorkbook(buf.Bytes())
	require.NoError(t, err)

	out, err := excelize.OpenReader(bytes.NewReader(normalized))
	require.NoError(t, err)
	defer out.Close()

	transactionType, err := out.GetCellValue(transactionsExportSheetName, "C2")
	require.NoError(t, err)
	assert.Equal(t, rechargePurchaseDisplayLabel, transactionType)

	detailHeader, err := out.GetCellValue(transactionsExportSheetName, "D1")
	require.NoError(t, err)
	assert.Equal(t, detailHeaderLabel, detailHeader)

	description, err := out.GetCellValue(transactionsExportSheetName, "D2")
	require.NoError(t, err)
	assert.Equal(t, rechargeExportLabel, description)
}
