SELECT text_content, image_url, image_direct_path, image_media_key,
	image_file_enc_sha256, image_file_sha256, image_file_length,
	image_mime_type, image_caption, image_jpeg_thumbnail, image_height, image_width
FROM forwardable_messages
WHERE message_id = $1