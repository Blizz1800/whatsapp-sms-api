INSERT INTO forwardable_messages (
	message_id, sender_lid, text_content, image_url, image_direct_path,
	image_media_key, image_file_enc_sha256, image_file_sha256,
	image_file_length, image_mime_type, image_caption, image_jpeg_thumbnail,
	image_height, image_width
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (message_id) DO UPDATE SET
	text_content = excluded.text_content,
	image_url = excluded.image_url,
	image_direct_path = excluded.image_direct_path,
	image_media_key = excluded.image_media_key,
	image_file_enc_sha256 = excluded.image_file_enc_sha256,
	image_file_sha256 = excluded.image_file_sha256,
	image_file_length = excluded.image_file_length,
	image_mime_type = excluded.image_mime_type,
	image_caption = excluded.image_caption,
	image_jpeg_thumbnail = excluded.image_jpeg_thumbnail,
	image_height = excluded.image_height,
	image_width = excluded.image_width