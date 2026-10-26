package migrations

import (
	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"testing"
)

func TestSocialOperationsJournalMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s)CREATE TABLE public.social_operation_artifacts.*CREATE TABLE public.social_operation_image_inspections.*CREATE TABLE public.social_operation_records.*CREATE TABLE public.social_operation_publications.*CREATE TABLE public.social_operation_metrics.*CREATE TABLE public.social_operation_outbound").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := up202610201100000000(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("(?s)DROP TABLE IF EXISTS public.social_operation_outbound.*DROP TABLE IF EXISTS public.social_operation_metrics.*DROP TABLE IF EXISTS public.social_operation_image_inspections.*DROP TABLE IF EXISTS public.social_operation_artifacts").WillReturnResult(sqlmock.NewResult(0, 0))
	if err := down202610201100000000(mschema.New(db).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
