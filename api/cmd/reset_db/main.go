package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/zgiai/zgi/api/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	dbCfg := cfg.Database

	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		dbCfg.Host, dbCfg.Port, dbCfg.Username, dbCfg.Password, dbCfg.DBName, dbCfg.SSLMode)

	fmt.Printf("Connecting to %s@%s:%d/%s\n", dbCfg.Username, dbCfg.Host, dbCfg.Port, dbCfg.DBName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("Failed to ping DB: %v", err))
	}

	fmt.Println("Resetting database (DROP SCHEMA public CASCADE)...")
	_, err = db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO public;")
	if err != nil {
		panic(err)
	}
	fmt.Println("Database reset successful.")
}
