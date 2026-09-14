package whatsapp

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var db *sql.DB

func InitDB() {
	var err error
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Println("Warning: DATABASE_URL not set, forwardable messages won't be persisted")
		return
	}

	db, err = sql.Open("pgx", dbURL)
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		return
	}

	if err = db.Ping(); err != nil {
		fmt.Printf("Error pinging database: %v\n", err)
		return
	}

	createTables()
	fmt.Println("Database initialized successfully")
}

func createTables() {
	query := `
	CREATE TABLE IF NOT EXISTS forwardable_messages (
		message_id TEXT PRIMARY KEY,
		sender_lid TEXT,
		text_content TEXT,
		image_url TEXT,
		image_direct_path TEXT,
		image_media_key BYTEA,
		image_file_enc_sha256 BYTEA,
		image_file_sha256 BYTEA,
		image_file_length BIGINT,
		image_mime_type TEXT,
		image_caption TEXT,
		image_jpeg_thumbnail BYTEA,
		image_height INTEGER,
		image_width INTEGER,
		created_at TIMESTAMP DEFAULT NOW()
	);`

	if _, err := db.Exec(query); err != nil {
		fmt.Printf("Error creating tables: %v\n", err)
	}
}

func SaveForwardableMessage(messageID, senderLID string, fm *forwardableMessage) bool {
	if db == nil {
		return true
	}

	query := `
	INSERT INTO forwardable_messages (
		message_id, sender_lid, text_content, image_url, image_direct_path,
		image_media_key, image_file_enc_sha256, image_file_sha256,
		image_file_length, image_mime_type, image_caption, image_jpeg_thumbnail,
		image_height, image_width
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	ON CONFLICT (message_id) DO UPDATE SET
		text_content = EXCLUDED.text_content,
		image_url = EXCLUDED.image_url,
		image_direct_path = EXCLUDED.image_direct_path,
		image_media_key = EXCLUDED.image_media_key,
		image_file_enc_sha256 = EXCLUDED.image_file_enc_sha256,
		image_file_sha256 = EXCLUDED.image_file_sha256,
		image_file_length = EXCLUDED.image_file_length,
		image_mime_type = EXCLUDED.image_mime_type,
		image_caption = EXCLUDED.image_caption,
		image_jpeg_thumbnail = EXCLUDED.image_jpeg_thumbnail,
		image_height = EXCLUDED.image_height,
		image_width = EXCLUDED.image_width`

	_, err := db.Exec(query,
		messageID, senderLID, fm.text, fm.imageURL, fm.imageDirectPath,
		fm.imageMediaKey, fm.imageFileEncSHA256, fm.imageFileSHA256,
		fm.imageFileLength, fm.imageMimeType, fm.imageCaption, fm.imageJPEGThumbnail,
		fm.imageHeight, fm.imageWidth,
	)
	if err != nil {
		fmt.Printf("Error saving forwardable message: %v\n", err)
		return false
	}
	return true
}

func LoadForwardableMessage(messageID string) *forwardableMessage {
	if db == nil {
		return nil
	}

	query := `
	SELECT text_content, image_url, image_direct_path, image_media_key,
		image_file_enc_sha256, image_file_sha256, image_file_length,
		image_mime_type, image_caption, image_jpeg_thumbnail, image_height, image_width
	FROM forwardable_messages
	WHERE message_id = $1`

	fm := &forwardableMessage{}
	err := db.QueryRow(query, messageID).Scan(
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

func DeleteForwardableMessage(messageID string) {
	if db == nil {
		return
	}
	if _, err := db.Exec("DELETE FROM forwardable_messages WHERE message_id = $1", messageID); err != nil {
		fmt.Printf("Error deleting forwardable message: %v\n", err)
	}
}

func DeleteOldForwardableMessages(retentionDays int) int64 {
	if db == nil {
		return 0
	}
	res, err := db.Exec(
		"DELETE FROM forwardable_messages WHERE created_at < NOW() - ($1::int * INTERVAL '1 day')",
		retentionDays,
	)
	if err != nil {
		fmt.Printf("Error cleaning old forwardable messages: %v\n", err)
		return 0
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		fmt.Printf("Cleaned up %d old forwardable message(s)\n", n)
	}
	return n
}
