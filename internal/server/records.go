package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

type recordDefinition struct {
	Name        string        `json:"name"`
	Label       string        `json:"label"`
	Description string        `json:"description"`
	Fields      []recordField `json:"fields"`
}

type recordField struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Type         string `json:"type"`
	Required     bool   `json:"required"`
	TargetRecord string `json:"target_record,omitempty"`
}

var identifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

func recordTableName(name string) string {
	return "athena_record_" + strings.ToLower(name)
}

func recordHandler(db *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			http.Error(w, "MariaDB is not configured", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			rows, err := db.QueryContext(r.Context(), `SELECT definition FROM athena_records ORDER BY label`)
			if err != nil {
				http.Error(w, "could not read Record definitions", http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			records := make([]recordDefinition, 0)
			for rows.Next() {
				var payload []byte
				var record recordDefinition
				if err := rows.Scan(&payload); err != nil || json.Unmarshal(payload, &record) != nil {
					http.Error(w, "stored Record definition is invalid", http.StatusInternalServerError)
					return
				}
				records = append(records, record)
			}
			if err := rows.Err(); err != nil {
				http.Error(w, "could not read Record definitions", http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(records)
		case http.MethodPut:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			var record recordDefinition
			if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
				http.Error(w, "invalid Record definition", http.StatusBadRequest)
				return
			}
			if !identifierPattern.MatchString(record.Name) || record.Label == "" {
				http.Error(w, "Record name must be a valid identifier and label is required", http.StatusBadRequest)
				return
			}
			if err := ensureRecordTable(r.Context(), db, record); err != nil {
				http.Error(w, "could not provision MariaDB Record table: "+err.Error(), http.StatusBadRequest)
				return
			}
			payload, err := json.Marshal(record)
			if err != nil {
				http.Error(w, "could not encode Record definition", http.StatusBadRequest)
				return
			}
			_, err = db.ExecContext(r.Context(), `INSERT INTO athena_records (name,label,definition) VALUES (?,?,?) ON DUPLICATE KEY UPDATE label=VALUES(label), definition=VALUES(definition)`, record.Name, record.Label, payload)
			if err != nil {
				http.Error(w, "could not save Record definition", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Allow", "GET, PUT")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func ensureRecordTable(ctx context.Context, db *sql.DB, record recordDefinition) error {
	if len(record.Fields) == 0 {
		return fmt.Errorf("a Record needs at least one field")
	}
	tableName := recordTableName(record.Name)
	columns := []string{"`id` CHAR(36) NOT NULL PRIMARY KEY", "`created_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP", "`updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"}
	for _, field := range record.Fields {
		if !identifierPattern.MatchString(field.Name) || strings.EqualFold(field.Name, "id") || strings.EqualFold(field.Name, "created_at") || strings.EqualFold(field.Name, "updated_at") {
			return fmt.Errorf("field %q is not a valid or available database column name", field.Name)
		}
		columnType, err := mariadbColumnType(field.Type)
		if err != nil {
			return fmt.Errorf("field %q: %w", field.Name, err)
		}
		if field.Type == "Link" {
			if !identifierPattern.MatchString(field.TargetRecord) {
				return fmt.Errorf("Link field %q needs a target Record", field.Name)
			}
			var targetExists bool
			if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM athena_records WHERE name = ?)`, field.TargetRecord).Scan(&targetExists); err != nil {
				return err
			}
			if !targetExists {
				return fmt.Errorf("Link field %q refers to unknown Record %q", field.Name, field.TargetRecord)
			}
		}
		nullable := " NULL"
		if field.Required {
			nullable = " NOT NULL"
		}
		columns = append(columns, fmt.Sprintf("`%s` %s%s", field.Name, columnType, nullable))
	}
	statement := fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` (%s) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci", tableName, strings.Join(columns, ","))
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return err
	}
	for _, field := range record.Fields {
		columnType, _ := mariadbColumnType(field.Type)
		statement := fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN IF NOT EXISTS `%s` %s NULL", tableName, field.Name, columnType)
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
		if field.Type == "Link" {
			targetTable := recordTableName(field.TargetRecord)
			var targetTableExists bool
			if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?)`, targetTable).Scan(&targetTableExists); err != nil {
				return err
			}
			if !targetTableExists {
				return fmt.Errorf("Link target %q has no MariaDB table yet", field.TargetRecord)
			}
			constraintName := recordForeignKeyName(record.Name, field.Name)
			var constraintExists bool
			if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = ? AND constraint_name = ?)`, tableName, constraintName).Scan(&constraintExists); err != nil {
				return err
			}
			if !constraintExists {
				statement := fmt.Sprintf("ALTER TABLE `%s` ADD CONSTRAINT `%s` FOREIGN KEY (`%s`) REFERENCES `%s` (`id`) ON DELETE RESTRICT", tableName, constraintName, field.Name, targetTable)
				if _, err := db.ExecContext(ctx, statement); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func recordForeignKeyName(recordName, fieldName string) string {
	sum := sha256.Sum256([]byte(recordName + ":" + fieldName))
	return fmt.Sprintf("fk_record_%x", sum[:8])
}

func mariadbColumnType(fieldType string) (string, error) {
	switch fieldType {
	case "Text":
		return "VARCHAR(255)", nil
	case "Long text":
		return "TEXT", nil
	case "Email":
		return "VARCHAR(320)", nil
	case "Phone":
		return "VARCHAR(40)", nil
	case "Number", "Currency":
		return "DECIMAL(18,6)", nil
	case "Date":
		return "DATE", nil
	case "Select":
		return "VARCHAR(120)", nil
	case "Link":
		return "CHAR(36)", nil
	case "Checkbox":
		return "BOOLEAN", nil
	default:
		return "", fmt.Errorf("unsupported field type %q", fieldType)
	}
}
