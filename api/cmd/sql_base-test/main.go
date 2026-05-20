package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/zgiai/ginext/config"
	sql_base "github.com/zgiai/ginext/pkg/sql_base"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if cfg.SQLBase.Type == "" {
		cfg.SQLBase.Type = string(sql_base.SQLBaseTypeExternal)
	}
	if cfg.SQLBase.PostgresMetaBaseURL == "" {
		cfg.SQLBase.PostgresMetaBaseURL = fmt.Sprintf("http://localhost:%d", cfg.Server.Port)
	}

	client, err := sql_base.NewSQLBaseClient()
	if err != nil {
		log.Fatalf("Failed to create SQL base client: %v", err)
	}

	// FormatQuery test endpoint
	http.HandleFunc("/query/format", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req sql_base.FormatQueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		result, err := client.FormatQuery(r.Context(), req)
		response := map[string]interface{}{
			"result": result,
			"error":  errToString(err),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// ParseQuery test endpoint
	http.HandleFunc("/query/parse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req sql_base.ParseQueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		result, err := client.ParseQuery(r.Context(), req)
		response := map[string]interface{}{
			"result": result,
			"error":  errToString(err),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// ListSchemas test endpoint (supports trailing slash)
	http.HandleFunc("/schemas", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var opts sql_base.ListSchemasOptions
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
				http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		result, err := client.ListSchemas(r.Context(), opts)
		response := map[string]interface{}{
			"result": result,
			"error":  errToString(err),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Redirect handling added to support trailing slashes
	http.HandleFunc("/schemas/", func(w http.ResponseWriter, r *http.Request) {
		// Redirect to the path without a trailing slash
		path := strings.TrimSuffix(r.URL.Path, "/")
		http.Redirect(w, r, path, http.StatusMovedPermanently)
	})

	// ListTables test endpoint (supports trailing slash)
	http.HandleFunc("/tables", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var opts sql_base.ListTablesOptions
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
				http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		result, err := client.ListTables(r.Context(), opts)
		response := map[string]interface{}{
			"result": result,
			"error":  errToString(err),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Redirect handling added to support trailing slashes
	http.HandleFunc("/tables/", func(w http.ResponseWriter, r *http.Request) {
		// Redirect to the path without a trailing slash
		path := strings.TrimSuffix(r.URL.Path, "/")
		http.Redirect(w, r, path, http.StatusMovedPermanently)
	})

	// ListColumns test endpoint (supports trailing slash)
	http.HandleFunc("/columns", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var opts sql_base.ListColumnsOptions
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
				http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		result, err := client.ListColumns(r.Context(), opts)
		response := map[string]interface{}{
			"result": result,
			"error":  errToString(err),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Redirect handling added to support trailing slashes
	http.HandleFunc("/columns/", func(w http.ResponseWriter, r *http.Request) {
		// Redirect to the path without a trailing slash
		path := strings.TrimSuffix(r.URL.Path, "/")
		http.Redirect(w, r, path, http.StatusMovedPermanently)
	})

	// ListViews test endpoint (supports trailing slash)
	http.HandleFunc("/views", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var opts sql_base.ListViewsOptions
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
				http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		result, err := client.ListViews(r.Context(), opts)
		response := map[string]interface{}{
			"result": result,
			"error":  errToString(err),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Redirect handling added to support trailing slashes
	http.HandleFunc("/views/", func(w http.ResponseWriter, r *http.Request) {
		// Redirect to the path without a trailing slash
		path := strings.TrimSuffix(r.URL.Path, "/")
		http.Redirect(w, r, path, http.StatusMovedPermanently)
	})

	// ListRoles test endpoint (supports trailing slash)
	http.HandleFunc("/roles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var opts sql_base.ListRolesOptions
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
				http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		result, err := client.ListRoles(r.Context(), opts)
		response := map[string]interface{}{
			"result": result,
			"error":  errToString(err),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Redirect handling added to support trailing slashes
	http.HandleFunc("/roles/", func(w http.ResponseWriter, r *http.Request) {
		// Redirect to the path without a trailing slash
		path := strings.TrimSuffix(r.URL.Path, "/")
		http.Redirect(w, r, path, http.StatusMovedPermanently)
	})

	// ListTypes test endpoint (supports trailing slash)
	http.HandleFunc("/types", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		result, err := client.ListTypes(r.Context())
		response := map[string]interface{}{
			"result": result,
			"error":  errToString(err),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Redirect handling added to support trailing slashes
	http.HandleFunc("/types/", func(w http.ResponseWriter, r *http.Request) {
		// Redirect to the path without a trailing slash
		path := strings.TrimSuffix(r.URL.Path, "/")
		http.Redirect(w, r, path, http.StatusMovedPermanently)
	})

	log.Println("SQL base test service started: http://localhost:9876")
	log.Println("Supported endpoints:")
	log.Println("  POST /query/format  - FormatQuery test endpoint")
	log.Println("  POST /query/parse   - ParseQuery test endpoint")
	log.Println("  GET/POST /schemas   - ListSchemas test endpoint")
	log.Println("  GET/POST /tables    - ListTables test endpoint")
	log.Println("  GET/POST /columns   - ListColumns test endpoint")
	log.Println("  GET/POST /views     - ListViews test endpoint")
	log.Println("  GET/POST /roles     - ListRoles test endpoint")
	log.Println("  GET /types          - ListTypes test endpoint")
	http.ListenAndServe(":9876", nil)
}

func errToString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
