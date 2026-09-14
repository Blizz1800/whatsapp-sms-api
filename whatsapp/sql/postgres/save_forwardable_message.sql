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
	image_width = EXCLUDED.image_width