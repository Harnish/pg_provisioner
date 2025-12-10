package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

type DatabaseConfig struct {
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type Config struct {
	RootConnectionString string           `json:"root_connection_string"`
	Databases            []DatabaseConfig `json:"databases"`
}

func main() {
	log.Println("PostgreSQL Database Provisioner starting...")

	// Check if running in watch mode
	watchMode := os.Getenv("WATCH_MODE")
	if watchMode == "true" {
		runWatchMode()
	} else {
		runOnce()
	}
}

func runOnce() {
	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := processConfig(config); err != nil {
		log.Fatalf("Failed to process config: %v", err)
	}

	log.Println("Database provisioning completed")
}

func runWatchMode() {
	log.Println("Running in WATCH MODE - will monitor config file for changes")
	
	configPath := getConfigPath()
	var lastModTime time.Time
	checkInterval := 10 * time.Second

	for {
		fileInfo, err := os.Stat(configPath)
		if err != nil {
			log.Printf("Error checking config file: %v", err)
			time.Sleep(checkInterval)
			continue
		}

		currentModTime := fileInfo.ModTime()
		
		if currentModTime.After(lastModTime) {
			log.Println("Config file changed, reprocessing...")
			lastModTime = currentModTime

			config, err := loadConfig()
			if err != nil {
				log.Printf("Failed to load config: %v", err)
				time.Sleep(checkInterval)
				continue
			}

			if err := processConfig(config); err != nil {
				log.Printf("Failed to process config: %v", err)
			} else {
				log.Println("Config processed successfully")
			}
		}

		time.Sleep(checkInterval)
	}
}

func getConfigPath() string {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/config/config.json"
	}
	return configPath
}

func loadConfig() (*Config, error) {
	configPath := getConfigPath()
	
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate configuration
	if config.RootConnectionString == "" {
		return nil, fmt.Errorf("root connection string is required")
	}

	if len(config.Databases) == 0 {
		return nil, fmt.Errorf("at least one database configuration is required")
	}

	return &config, nil
}

func processConfig(config *Config) error {
	// Connect to PostgreSQL as root
	db, err := connectWithRetry(config.RootConnectionString, 5, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	defer db.Close()

	log.Println("Connected to PostgreSQL successfully")

	// Process each database configuration
	for i, dbConfig := range config.Databases {
		log.Printf("Processing database %d/%d: %s", i+1, len(config.Databases), dbConfig.Database)

		if err := provisionDatabase(db, dbConfig); err != nil {
			log.Printf("Failed to provision database %s: %v", dbConfig.Database, err)
			continue
		}

		log.Printf("Successfully provisioned database: %s with user: %s", dbConfig.Database, dbConfig.User)
	}

	return nil
}

func connectWithRetry(connStr string, maxRetries int, delay time.Duration) (*sql.DB, error) {
	var db *sql.DB
	var err error

	for i := 0; i < maxRetries; i++ {
		db, err = sql.Open("postgres", connStr)
		if err != nil {
			log.Printf("Attempt %d/%d: Failed to open connection: %v", i+1, maxRetries, err)
			time.Sleep(delay)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()

		if err == nil {
			return db, nil
		}

		log.Printf("Attempt %d/%d: Failed to ping database: %v", i+1, maxRetries, err)
		db.Close()
		time.Sleep(delay)
	}

	return nil, fmt.Errorf("failed to connect after %d attempts: %w", maxRetries, err)
}

func provisionDatabase(db *sql.DB, config DatabaseConfig) error {
	ctx := context.Background()

	// Check if user exists
	userExists, err := checkUserExists(ctx, db, config.User)
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}

	// Create user if it doesn't exist
	if !userExists {
		log.Printf("Creating user: %s", config.User)
		createUserSQL := fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s'", 
			quoteIdentifier(config.User), 
			escapeString(config.Password))
		
		if _, err := db.ExecContext(ctx, createUserSQL); err != nil {
			return fmt.Errorf("failed to create user: %w", err)
		}
		log.Printf("User %s created successfully", config.User)
	} else {
		log.Printf("User %s already exists", config.User)
		// Update password if user exists
		updatePasswordSQL := fmt.Sprintf("ALTER USER %s WITH PASSWORD '%s'", 
			quoteIdentifier(config.User), 
			escapeString(config.Password))
		
		if _, err := db.ExecContext(ctx, updatePasswordSQL); err != nil {
			return fmt.Errorf("failed to update user password: %w", err)
		}
		log.Printf("Password updated for user %s", config.User)
	}

	// Check if database exists
	dbExists, err := checkDatabaseExists(ctx, db, config.Database)
	if err != nil {
		return fmt.Errorf("failed to check database existence: %w", err)
	}

	// Create database if it doesn't exist
	if !dbExists {
		log.Printf("Creating database: %s", config.Database)
		createDbSQL := fmt.Sprintf("CREATE DATABASE %s OWNER %s", 
			quoteIdentifier(config.Database), 
			quoteIdentifier(config.User))
		
		if _, err := db.ExecContext(ctx, createDbSQL); err != nil {
			return fmt.Errorf("failed to create database: %w", err)
		}
		log.Printf("Database %s created successfully", config.Database)
	} else {
		log.Printf("Database %s already exists", config.Database)
		// Update owner if database exists
		alterOwnerSQL := fmt.Sprintf("ALTER DATABASE %s OWNER TO %s", 
			quoteIdentifier(config.Database), 
			quoteIdentifier(config.User))
		
		if _, err := db.ExecContext(ctx, alterOwnerSQL); err != nil {
			return fmt.Errorf("failed to alter database owner: %w", err)
		}
		log.Printf("Owner of database %s set to %s", config.Database, config.User)
	}

	// Grant all privileges
	grantSQL := fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s", 
		quoteIdentifier(config.Database), 
		quoteIdentifier(config.User))
	
	if _, err := db.ExecContext(ctx, grantSQL); err != nil {
		return fmt.Errorf("failed to grant privileges: %w", err)
	}

	return nil
}

func checkUserExists(ctx context.Context, db *sql.DB, username string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)"
	err := db.QueryRowContext(ctx, query, username).Scan(&exists)
	return exists, err
}

func checkDatabaseExists(ctx context.Context, db *sql.DB, database string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	err := db.QueryRowContext(ctx, query, database).Scan(&exists)
	return exists, err
}

func quoteIdentifier(s string) string {
	return fmt.Sprintf(`"%s"`, s)
}

func escapeString(s string) string {
	// Basic SQL string escaping - replace single quotes with two single quotes
	result := ""
	for _, c := range s {
		if c == '\'' {
			result += "''"
		} else {
			result += string(c)
		}
	}
	return result
}
