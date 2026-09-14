CREATE TABLE IF NOT EXISTS forwardable_messages (
		message_id TEXT PRIMARY KEY,
		sender_lid TEXT,
		text_content TEXT,
		image_url TEXT,
		image_direct_path TEXT,
		image_media_key BLOB,
		image_file_enc_sha256 BLOB,
		image_file_sha256 BLOB,
		image_file_length BIGINT,
		image_mime_type TEXT,
		image_caption TEXT,
		image_jpeg_thumbnail BLOB,
		image_height INTEGER,
		image_width INTEGER,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);