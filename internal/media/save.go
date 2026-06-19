package media

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// SaveBytes writes data to uploadDir under a random <uuid>.<ext> name and returns
// its public URL plus the detected MIME type. The extension comes from the MIME
// type (sniffed from the first 512 bytes), so only the type whitelist in mimeExt
// is accepted; anything else is rejected. Shared by the HTTP upload handler and
// the cmd/photos backfill tool.
func SaveBytes(uploadDir, baseURL string, data []byte) (url, mime string, err error) {
	if len(data) == 0 {
		return "", "", fmt.Errorf("empty data")
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	mime = strings.Split(http.DetectContentType(head), ";")[0]
	ext, ok := mimeExt[mime]
	if !ok {
		return "", "", fmt.Errorf("unsupported file type: %s", mime)
	}

	filename := uuid.New().String() + ext
	if err := os.WriteFile(filepath.Join(uploadDir, filename), data, 0644); err != nil {
		return "", "", fmt.Errorf("write file: %w", err)
	}
	return strings.TrimRight(baseURL, "/") + "/uploads/" + filename, mime, nil
}
