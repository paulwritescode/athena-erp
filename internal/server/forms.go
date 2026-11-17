package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type formDefinition struct {
	Name        string      `json:"name"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Module      string      `json:"module"`
	Fields      []formField `json:"fields"`
}

type formField struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

func formHandler(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			http.Error(w, "MariaDB is not configured", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			rows, err := db.QueryContext(r.Context(), `SELECT definition FROM athena_forms ORDER BY label`)
			if err != nil {
				http.Error(w, "could not read Form definitions", http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			forms := make([]formDefinition, 0)
			for rows.Next() {
				var payload []byte
				var form formDefinition
				if err := rows.Scan(&payload); err != nil || json.Unmarshal(payload, &form) != nil {
					http.Error(w, "stored Form definition is invalid", http.StatusInternalServerError)
					return
				}
				forms = append(forms, form)
			}
			if err := rows.Err(); err != nil {
				http.Error(w, "could not read Form definitions", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(forms)
		case http.MethodPut:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			var form formDefinition
			if err := json.NewDecoder(r.Body).Decode(&form); err != nil {
				http.Error(w, "invalid Form definition", http.StatusBadRequest)
				return
			}
			if form.Name == "" || form.Label == "" || form.Module == "" {
				http.Error(w, "Form name, label, and module are required", http.StatusBadRequest)
				return
			}
			payload, err := json.Marshal(form)
			if err != nil {
				http.Error(w, "could not encode Form definition", http.StatusBadRequest)
				return
			}
			_, err = db.ExecContext(r.Context(), `INSERT INTO athena_forms (name,label,module_name,definition) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE label=VALUES(label), module_name=VALUES(module_name), definition=VALUES(definition)`, form.Name, form.Label, form.Module, payload)
			if err != nil {
				http.Error(w, "could not save Form definition", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}
