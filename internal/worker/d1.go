package worker

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v8 "github.com/tommie/v8go"

	// SQLite driver for database/sql (used by the glebarez/sqlite GORM driver).
	_ "github.com/glebarez/sqlite"
)

// GetDataDir returns the data directory path from env or default.
func GetDataDir() string {
	if dir := os.Getenv("HOSTEDAT_DATA_DIR"); dir != "" {
		return dir
	}
	return "./data"
}

// GetD1Path returns the full path to a D1 database file.
func GetD1Path(dataDir, databaseID string) string {
	return filepath.Join(dataDir, "d1", databaseID+".sqlite3")
}

// DeleteFile removes a file from disk (best-effort, ignores errors).
func DeleteFile(path string) error {
	return os.Remove(path)
}

// D1Bridge provides Go methods that back the D1 database JS bindings.
// Each D1 binding gets its own isolated SQLite database, completely
// separate from the application's main database.
type D1Bridge struct {
	db         *sql.DB
	DatabaseID string
}

// ValidateDatabaseID rejects database IDs that contain path traversal
// characters, null bytes, or are empty/too long.
func ValidateDatabaseID(id string) error {
	if id == "" {
		return fmt.Errorf("database ID must not be empty")
	}
	if len(id) > 128 {
		return fmt.Errorf("database ID too long")
	}
	if strings.Contains(id, "..") {
		return fmt.Errorf("database ID contains path traversal")
	}
	if strings.ContainsAny(id, "/\\") {
		return fmt.Errorf("database ID contains path separator")
	}
	if strings.ContainsRune(id, 0) {
		return fmt.Errorf("database ID contains null byte")
	}
	return nil
}

// OpenD1Database opens (or creates) an isolated SQLite database for the given
// database ID. The file is stored at {dataDir}/d1/{databaseID}.sqlite3.
func OpenD1Database(dataDir, databaseID string) (*D1Bridge, error) {
	if err := ValidateDatabaseID(databaseID); err != nil {
		return nil, err
	}
	d1Dir := filepath.Join(dataDir, "d1")
	if err := os.MkdirAll(d1Dir, 0755); err != nil {
		return nil, fmt.Errorf("creating D1 directory: %w", err)
	}
	dbPath := filepath.Join(d1Dir, databaseID+".sqlite3")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening D1 database %q: %w", databaseID, err)
	}
	// Enable WAL mode for better concurrent access.
	_, _ = db.Exec("PRAGMA journal_mode=WAL")
	return &D1Bridge{db: db, DatabaseID: databaseID}, nil
}

// NewD1BridgeMemory creates an in-memory D1Bridge for testing.
func NewD1BridgeMemory(databaseID string) (*D1Bridge, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("opening in-memory D1 database: %w", err)
	}
	return &D1Bridge{db: db, DatabaseID: databaseID}, nil
}

// Close closes the underlying database connection.
func (d *D1Bridge) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// D1ExecResult holds the result of executing a SQL statement.
type D1ExecResult struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	Meta    D1Meta          `json:"meta"`
}

// D1Meta holds metadata about a D1 query execution.
type D1Meta struct {
	ChangedDB   bool  `json:"changed_db"`
	Changes     int64 `json:"changes"`
	LastRowID   int64 `json:"last_row_id"`
	RowsRead    int   `json:"rows_read"`
	RowsWritten int   `json:"rows_written"`
}

// Exec runs a SQL statement with optional bindings and returns columns, rows, and metadata.
func (d *D1Bridge) Exec(sqlStr string, bindings []interface{}) (*D1ExecResult, error) {
	upperSQL := strings.TrimSpace(strings.ToUpper(sqlStr))
	isQuery := strings.HasPrefix(upperSQL, "SELECT") ||
		strings.HasPrefix(upperSQL, "PRAGMA") ||
		strings.HasPrefix(upperSQL, "WITH")

	if isQuery {
		rows, err := d.db.Query(sqlStr, bindings...)
		if err != nil {
			return nil, fmt.Errorf("D1: query error: %w", err)
		}
		defer func() { _ = rows.Close() }()

		columns, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("D1: columns error: %w", err)
		}

		var resultRows [][]interface{}
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, fmt.Errorf("D1: scan error: %w", err)
			}
			row := make([]interface{}, len(columns))
			for i, v := range values {
				if b, ok := v.([]byte); ok {
					row[i] = string(b)
				} else {
					row[i] = v
				}
			}
			resultRows = append(resultRows, row)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("D1: rows iteration error: %w", err)
		}

		return &D1ExecResult{
			Columns: columns,
			Rows:    resultRows,
			Meta: D1Meta{
				ChangedDB: false,
				RowsRead:  len(resultRows),
			},
		}, nil
	}

	// Non-query (INSERT, UPDATE, DELETE, CREATE, DROP, etc.)
	result, err := d.db.Exec(sqlStr, bindings...)
	if err != nil {
		return nil, fmt.Errorf("D1: exec error: %w", err)
	}

	changes, _ := result.RowsAffected()
	lastID, _ := result.LastInsertId()

	return &D1ExecResult{
		Columns: []string{},
		Rows:    [][]interface{}{},
		Meta: D1Meta{
			ChangedDB:   changes > 0,
			Changes:     changes,
			LastRowID:   lastID,
			RowsWritten: int(changes),
		},
	}, nil
}

// buildD1Binding creates a D1Database JS object with prepare(), batch(), exec(),
// and dump() methods backed by the given D1Bridge.
func buildD1Binding(iso *v8.Isolate, ctx *v8.Context, bridge *D1Bridge) (*v8.Value, error) {
	dbIDVal, _ := v8.NewValue(iso, bridge.DatabaseID)
	_ = ctx.Global().Set("__d1_db_id_"+bridge.DatabaseID, dbIDVal)

	execFnName := "__d1_exec_" + bridge.DatabaseID
	_ = ctx.Global().Set(execFnName, v8.NewFunctionTemplate(iso, func(info *v8.FunctionCallbackInfo) *v8.Value {
		args := info.Args()
		if len(args) < 2 {
			errVal, _ := v8.NewValue(iso, `{"error":"__d1_exec requires sql and bindings arguments"}`)
			return errVal
		}
		sqlStr := args[0].String()
		bindingsJSON := args[1].String()

		var bindings []interface{}
		if bindingsJSON != "" && bindingsJSON != "[]" {
			if err := json.Unmarshal([]byte(bindingsJSON), &bindings); err != nil {
				errStr, _ := json.Marshal(map[string]string{"error": "invalid bindings JSON: " + err.Error()})
				errVal, _ := v8.NewValue(iso, string(errStr))
				return errVal
			}
		}

		result, err := bridge.Exec(sqlStr, bindings)
		if err != nil {
			errStr, _ := json.Marshal(map[string]string{"error": err.Error()})
			errVal, _ := v8.NewValue(iso, string(errStr))
			return errVal
		}

		data, _ := json.Marshal(result)
		resultVal, _ := v8.NewValue(iso, string(data))
		return resultVal
	}).GetFunction(ctx))

	polyfill := fmt.Sprintf(`(function() {
	var execFn = globalThis[%q];

	function D1PreparedStatement(sql) {
		this._sql = sql;
		this._bindings = [];
	}

	D1PreparedStatement.prototype.bind = function() {
		var stmt = new D1PreparedStatement(this._sql);
		stmt._bindings = Array.prototype.slice.call(arguments);
		return stmt;
	};

	D1PreparedStatement.prototype._exec = function() {
		var bindingsJSON = JSON.stringify(this._bindings);
		var resultStr = execFn(this._sql, bindingsJSON);
		var result = JSON.parse(resultStr);
		if (result.error) {
			throw new Error(result.error);
		}
		return result;
	};

	D1PreparedStatement.prototype.all = function() {
		var self = this;
		return new Promise(function(resolve, reject) {
			try {
				var result = self._exec();
				var results = [];
				if (result.columns && result.rows) {
					for (var i = 0; i < result.rows.length; i++) {
						var obj = {};
						for (var j = 0; j < result.columns.length; j++) {
							obj[result.columns[j]] = result.rows[i][j];
						}
						results.push(obj);
					}
				}
				resolve({
					results: results,
					success: true,
					meta: result.meta || {}
				});
			} catch(e) {
				reject(e);
			}
		});
	};

	D1PreparedStatement.prototype.first = function(column) {
		var self = this;
		return new Promise(function(resolve, reject) {
			try {
				var result = self._exec();
				if (!result.rows || result.rows.length === 0) {
					resolve(null);
					return;
				}
				var row = {};
				for (var j = 0; j < result.columns.length; j++) {
					row[result.columns[j]] = result.rows[0][j];
				}
				if (column !== undefined && column !== null) {
					resolve(row[column] !== undefined ? row[column] : null);
				} else {
					resolve(row);
				}
			} catch(e) {
				reject(e);
			}
		});
	};

	D1PreparedStatement.prototype.raw = function(options) {
		var self = this;
		return new Promise(function(resolve, reject) {
			try {
				var result = self._exec();
				var rows = result.rows || [];
				if (options && options.columnNames) {
					rows = [result.columns].concat(rows);
				}
				resolve(rows);
			} catch(e) {
				reject(e);
			}
		});
	};

	D1PreparedStatement.prototype.run = function() {
		var self = this;
		return new Promise(function(resolve, reject) {
			try {
				var result = self._exec();
				resolve({
					results: [],
					success: true,
					meta: result.meta || {}
				});
			} catch(e) {
				reject(e);
			}
		});
	};

	function D1Database() {}

	D1Database.prototype.prepare = function(sql) {
		return new D1PreparedStatement(sql);
	};

	D1Database.prototype.batch = function(statements) {
		return new Promise(function(resolve, reject) {
			try {
				var results = [];
				for (var i = 0; i < statements.length; i++) {
					var stmt = statements[i];
					var result = stmt._exec();
					var rows = [];
					if (result.columns && result.rows) {
						for (var r = 0; r < result.rows.length; r++) {
							var obj = {};
							for (var c = 0; c < result.columns.length; c++) {
								obj[result.columns[c]] = result.rows[r][c];
							}
							rows.push(obj);
						}
					}
					results.push({
						results: rows,
						success: true,
						meta: result.meta || {}
					});
				}
				resolve(results);
			} catch(e) {
				reject(e);
			}
		});
	};

	D1Database.prototype.exec = function(sql) {
		var self = this;
		return new Promise(function(resolve, reject) {
			try {
				var statements = sql.split(";").filter(function(s) {
					return s.trim().length > 0;
				});
				var count = 0;
				for (var i = 0; i < statements.length; i++) {
					var bindingsJSON = "[]";
					var resultStr = execFn(statements[i].trim(), bindingsJSON);
					var result = JSON.parse(resultStr);
					if (result.error) {
						throw new Error(result.error);
					}
					count++;
				}
				resolve({ count: count, duration: 0 });
			} catch(e) {
				reject(e);
			}
		});
	};

	D1Database.prototype.dump = function() {
		return Promise.reject(new Error("D1 dump() is not supported in this runtime"));
	};

	return new D1Database();
})()`, execFnName)

	jsVal, err := ctx.RunScript(polyfill, "d1_polyfill.js")
	if err != nil {
		return nil, fmt.Errorf("D1 polyfill error: %w", err)
	}

	return jsVal, nil
}

// setupD1 is a no-op setup function for the D1 binding.
// The actual bindings are created per-database in buildD1Binding.
func setupD1(_ *v8.Isolate, _ *v8.Context, _ *eventLoop) error {
	return nil
}
