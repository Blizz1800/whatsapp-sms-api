package whatsapp

import (
	"database/sql"
	"fmt"
	"os"

	"main/config"

	_ "github.com/jackc/pgx/v5/stdlib"
	"modernc.org/sqlite"
)

var db *sql.DB

func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
}

func InitDB() {
	cfg := config.Load()
	if cfg.DBType != "sqlite" && cfg.DatabaseURL == "" {
		fmt.Println("Warning: DATABASE_URL not set, forwardable messages won't be persisted")
		return
	}

	var err error
	db, err = sql.Open(cfg.Driver(), cfg.DSN())
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		return
	}
	if err = db.Ping(); err != nil {
		fmt.Printf("Error pinging database: %v\n", err)
		return
	}

	createTables()
	fmt.Printf("Database initialized successfully (%s)\n", cfg.DBType)
}

func loadQuery(query string) (string, error) {
	cfg := config.Load()
	data, err := os.ReadFile("sql/" + cfg.DBType + "/" + query + ".sql")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func runQuery(query string) error {
	_query, err := loadQuery(query)
	if err != nil {
		return err
	}
	if _, err := db.Exec(_query); err != nil {
		return err
	}
	return nil
}

func createTables() {
	if err := runQuery("create_table_forwardable_messages"); err != nil {
		fmt.Printf("Error creating tables: %v\n", err)
	}
}

func SaveForwardableMessage(messageID, senderLID string, fm *forwardableMessage) {
	if db == nil {
		return
	}

	query, err := loadQuery("save_forwardable_message")
	if err != nil {
		fmt.Printf("Error loading query: %v\n", err)
		return
	}

	_, err = db.Exec(query,
		messageID, senderLID, fm.text, fm.imageURL, fm.imageDirectPath,
		fm.imageMediaKey, fm.imageFileEncSHA256, fm.imageFileSHA256,
		fm.imageFileLength, fm.imageMimeType, fm.imageCaption, fm.imageJPEGThumbnail,
		fm.imageHeight, fm.imageWidth,
	)
	if err != nil {
		fmt.Printf("Error saving forwardable message: %v\n", err)
	}
}

func LoadForwardableMessage(messageID string) *forwardableMessage {
	if db == nil {
		return nil
	}

	query, err := loadQuery("load_forwardable_message")
	if err != nil {
		fmt.Printf("Error loading query: %v\n", err)
		return nil
	}

	fm := &forwardableMessage{}
	err = db.QueryRow(query, messageID).Scan(
		&fm.text, &fm.imageURL, &fm.imageDirectPath, &fm.imageMediaKey,
		&fm.imageFileEncSHA256, &fm.imageFileSHA256, &fm.imageFileLength,
		&fm.imageMimeType, &fm.imageCaption, &fm.imageJPEGThumbnail,
		&fm.imageHeight, &fm.imageWidth,
	)
	if err != nil {
		if err != sql.ErrNoRows {
			fmt.Printf("Error loading forwardable message: %v\n", err)
		}
		return nil
	}

	return fm
}
