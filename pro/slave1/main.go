package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"bufio"
	"strconv"
	"time"
	_ "github.com/go-sql-driver/mysql"
)

var (
    db           *sql.DB
    masterAddress   string
    isMaster     bool
    electionInProgress bool
)

// Create any MySQL user you choose, set a password and give them permissions:
/*
	CREATE USER 'user'@'%' IDENTIFIED BY 'rootroot';
	GRANT ALL PRIVILEGES ON *.* TO 'user'@'%' WITH GRANT OPTION;
	FLUSH PRIVILEGES;

*/
func main() {
    reader := bufio.NewReader(os.Stdin)

    // 1. Prompt for MySQL username
    fmt.Print("MySQL username (default \"root\"): ")
    userInput, _ := reader.ReadString('\n')
    user := strings.TrimSpace(userInput)
    if user == "" {
        user = "root"
    }

    // 2. Prompt for MySQL password
    fmt.Print("MySQL password (default \"rootroot\"): ")
    passInput, _ := reader.ReadString('\n')
    pass := strings.TrimSpace(passInput)
    if pass == "" {
        pass = "rootroot"
    }

    // 3. Prompt for MySQL host
    fmt.Print("MySQL host (default \"127.0.0.1\"): ")
    hostInput, _ := reader.ReadString('\n')
    host := strings.TrimSpace(hostInput)
    if host == "" {
        host = "127.0.0.1"
    }

    // 4. Prompt for MySQL port
    fmt.Print("MySQL port (default \"3306\"): ")
    mysqlPortInput, _ := reader.ReadString('\n')
    mysqlPortInput = strings.TrimSpace(mysqlPortInput)
    mysqlPort := 3306
    if mysqlPortInput != "" {
        p, err := strconv.Atoi(mysqlPortInput)
        if err != nil {
            log.Fatalf("Invalid MySQL port: %v", err)
        }
        mysqlPort = p
    }

    // 5. Build the DSN from the four parts
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/", user, pass, host, mysqlPort)

    // 6. Prompt for the HTTP server port
    fmt.Print("HTTP server port (default \"8002\"): ")
    httpPortInput, _ := reader.ReadString('\n')
    httpPortInput = strings.TrimSpace(httpPortInput)
    httpPort := 8002
    if httpPortInput != "" {
        p, err := strconv.Atoi(httpPortInput)
        if err != nil {
            log.Fatalf("Invalid HTTP port: %v", err)
        }
        httpPort = p
    }

    // 7. Prompt for the master server address
    fmt.Print("Master server address (e.g. http://localhost:8001): ")
    masterInput, _ := reader.ReadString('\n')
    masterAddress = strings.TrimSpace(masterInput)
    if masterAddress == "" {
        masterAddress = "http://localhost:8001"
    }

    // 8. Open the MySQL connection
    var err error
    db, err = sql.Open("mysql", dsn)
    if err != nil {
        log.Fatalf("Failed to open database connection: %v", err)
    }
    defer db.Close()

    // 9. Verify the database connection is alive
    if err = db.Ping(); err != nil {
        log.Fatalf("Failed to ping database: %v", err)
    }
    fmt.Println("✔️ Database connection successful")

    // 10. Define all HTTP routes and start monitoring the master
    defineBasicRoutes()
    go checkMasterHealth()

    // 11. Start the HTTP server
    addr := fmt.Sprintf(":%d", httpPort)
    fmt.Printf("🚀 Slave server listening on %s (master = %s)\n", addr, masterAddress)
    log.Fatal(http.ListenAndServe(addr, nil))
}
func defineBasicRoutes() {
	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})

	http.HandleFunc("/is-master", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{
			"isMaster": isMaster,
		})
	})

	// Define replication routes
	http.HandleFunc("/replicate/db", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		replicateDB(w, r)
	})

	http.HandleFunc("/replicate/dropdb", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		replicateDropDB(w, r)
	})

	http.HandleFunc("/replicate/table", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		replicateTable(w, r)
	})

	http.HandleFunc("/replicate/insert", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		replicateInsert(w, r)
	})

	http.HandleFunc("/replicate/update", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		replicateUpdate(w, r)
	})

	http.HandleFunc("/replicate/delete", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		replicateDelete(w, r)
	})
}

func allowCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func replicateDB(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Database name is required", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("CREATE DATABASE IF NOT EXISTS " + name)
	if err != nil {
		http.Error(w, "Failed to create database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Database replicated successfully",
		"dbname":  name,
	})
}

func replicateDropDB(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		http.Error(w, "Database name is required", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("DROP DATABASE IF EXISTS " + name)
	if err != nil {
		http.Error(w, "Failed to drop database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Database dropped successfully",
		"dbname":  name,
	})
}

func replicateTable(w http.ResponseWriter, r *http.Request) {
	dbname := r.URL.Query().Get("dbname")
	table := r.URL.Query().Get("table")
	schema := r.URL.Query().Get("schema")

	if dbname == "" || table == "" || schema == "" {
		http.Error(w, "All parameters (dbname, table, schema) are required", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s.%s (%s)", dbname, table, schema)
	_, err := db.Exec(query)
	if err != nil {
		http.Error(w, "Failed to create table: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Table replicated successfully",
		"dbname":  dbname,
		"table":   table,
	})
}

func replicateInsert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DBName string `json:"dbname"`
		Table  string `json:"table"`
		Values string `json:"values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DBName == "" || req.Table == "" || req.Values == "" {
		http.Error(w, "All fields (dbname, table, values) are required", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("INSERT INTO %s.%s VALUES (%s)", req.DBName, req.Table, req.Values)
	result, err := db.Exec(query)
	if err != nil {
		http.Error(w, "Failed to insert record: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":      "Record inserted successfully",
		"rowsAffected": rowsAffected,
	})
}

func replicateUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DBName string `json:"dbname"`
		Table  string `json:"table"`
		Set    string `json:"set"`
		Where  string `json:"where"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DBName == "" || req.Table == "" || req.Set == "" || req.Where == "" {
		http.Error(w, "All fields (dbname, table, set, where) are required", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("UPDATE %s.%s SET %s WHERE %s", req.DBName, req.Table, req.Set, req.Where)
	result, err := db.Exec(query)
	if err != nil {
		http.Error(w, "Failed to update record: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":      "Record updated successfully",
		"rowsAffected": rowsAffected,
	})
}

func replicateDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DBName string `json:"dbname"`
		Table  string `json:"table"`
		Where  string `json:"where"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DBName == "" || req.Table == "" || req.Where == "" {
		http.Error(w, "All fields (dbname, table, where) are required", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("DELETE FROM %s.%s WHERE %s", req.DBName, req.Table, req.Where)
	result, err := db.Exec(query)
	if err != nil {
		http.Error(w, "Failed to delete record: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":      "Record deleted successfully",
		"rowsAffected": rowsAffected,
	})
}

func startElection() {
	if electionInProgress {
		return
	}
	electionInProgress = true

	log.Println("Starting master election...")
	time.Sleep(time.Second * 2)

	// Check if there's already a new master
	client := &http.Client{Timeout: 5 * time.Second}
	_, err := client.Get(masterAddress + "/ping")
	if err == nil {
		electionInProgress = false
		return // Another node already became master
	}

	// Promote this node to master if it's the first slave (8002)
	if os.Getenv("PORT") == "8002" {
		promoteToMaster()
	}
}

func promoteToMaster() {
	isMaster = true
	masterAddress = "http://localhost:8002"
	log.Println("This node has been promoted to master")

	// Add master endpoints
	http.HandleFunc("/createdb", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		createDB(w, r)
	})

	http.HandleFunc("/dropdb", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		dropDB(w, r)
	})

	http.HandleFunc("/createtable", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		createTable(w, r)
	})

	http.HandleFunc("/insert", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		insertRecord(w, r)
	})

	http.HandleFunc("/select", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		selectRecords(w, r)
	})

	http.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		updateRecord(w, r)
	})

	http.HandleFunc("/delete", func(w http.ResponseWriter, r *http.Request) {
		allowCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		deleteRecord(w, r)
	})
}

func checkMasterHealth() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !isMaster {
			client := &http.Client{Timeout: 5 * time.Second}
			_, err := client.Get(masterAddress + "/ping")
			if err != nil {
				log.Printf("Master is down: %v", err)
				startElection()
			}
		}
	}
}

// Master functions that will be used when promoted
func createDB(w http.ResponseWriter, r *http.Request) {
	dbname := r.URL.Query().Get("name")
	if dbname == "" {
		http.Error(w, "Database name is required", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("CREATE DATABASE IF NOT EXISTS " + dbname)
	if err != nil {
		http.Error(w, "Failed to create database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Database created successfully"})
}

func dropDB(w http.ResponseWriter, r *http.Request) {
	dbname := r.URL.Query().Get("name")
	if dbname == "" {
		http.Error(w, "Database name is required", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("DROP DATABASE IF EXISTS " + dbname)
	if err != nil {
		http.Error(w, "Failed to drop database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Database dropped successfully"})
}

func createTable(w http.ResponseWriter, r *http.Request) {
	dbname := r.URL.Query().Get("dbname")
	table := r.URL.Query().Get("table")
	schema := r.URL.Query().Get("schema")

	if dbname == "" || table == "" || schema == "" {
		http.Error(w, "All parameters (dbname, table, schema) are required", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s.%s (%s)", dbname, table, schema)
	_, err := db.Exec(query)
	if err != nil {
		http.Error(w, "Failed to create table: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Table created successfully"})
}

func insertRecord(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DBName string `json:"dbname"`
		Table  string `json:"table"`
		Values string `json:"values"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DBName == "" || req.Table == "" || req.Values == "" {
		http.Error(w, "All fields (dbname, table, values) are required", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("INSERT INTO %s.%s VALUES (%s)", req.DBName, req.Table, req.Values)
	_, err := db.Exec(query)
	if err != nil {
		http.Error(w, "Failed to insert record: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Record inserted successfully"})
}

func selectRecords(w http.ResponseWriter, r *http.Request) {
	dbname := r.URL.Query().Get("dbname")
	table := r.URL.Query().Get("table")

	if dbname == "" || table == "" {
		http.Error(w, "Both dbname and table parameters are required", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("SELECT * FROM %s.%s", dbname, table)
	rows, err := db.Query(query)
	if err != nil {
		http.Error(w, "Failed to query records: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		http.Error(w, "Failed to get columns: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var results []map[string]interface{}
	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			http.Error(w, "Failed to scan row: "+err.Error(), http.StatusInternalServerError)
			return
		}

		row := make(map[string]interface{})
		for i, col := range cols {
			val := columnPointers[i].(*interface{})
			row[col] = *val
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Error during rows iteration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func updateRecord(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DBName string `json:"dbname"`
		Table  string `json:"table"`
		Set    string `json:"set"`
		Where  string `json:"where"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DBName == "" || req.Table == "" || req.Set == "" || req.Where == "" {
		http.Error(w, "All fields (dbname, table, set, where) are required", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("UPDATE %s.%s SET %s WHERE %s", req.DBName, req.Table, req.Set, req.Where)
	_, err := db.Exec(query)
	if err != nil {
		http.Error(w, "Failed to update record: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Record updated successfully"})
}

func deleteRecord(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DBName string `json:"dbname"`
		Table  string `json:"table"`
		Where  string `json:"where"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DBName == "" || req.Table == "" || req.Where == "" {
		http.Error(w, "All fields (dbname, table, where) are required", http.StatusBadRequest)
		return
	}

	query := fmt.Sprintf("DELETE FROM %s.%s WHERE %s", req.DBName, req.Table, req.Where)
	_, err := db.Exec(query)
	if err != nil {
		http.Error(w, "Failed to delete record: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"message": "Record deleted successfully"})
}
