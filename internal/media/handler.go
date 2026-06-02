package media

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"riders-connect/internal/respond"
)

const maxUploadSize = 100 << 20 // 100 MB

// mimeMessageType maps detected MIME types to chat message types.
var mimeMessageType = map[string]string{
	"image/jpeg":      "image",
	"image/png":       "image",
	"image/gif":       "image",
	"image/webp":      "image",
	"video/mp4":       "video",
	"video/quicktime": "video",
	"video/webm":      "video",
	"audio/mpeg":      "audio",
	"audio/ogg":       "audio",
	"audio/wav":       "audio",
	"audio/mp4":       "audio",
	"audio/webm":      "audio",
	"application/pdf": "file",
}

var mimeExt = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/gif":       ".gif",
	"image/webp":      ".webp",
	"video/mp4":       ".mp4",
	"video/quicktime": ".mov",
	"video/webm":      ".webm",
	"audio/mpeg":      ".mp3",
	"audio/ogg":       ".ogg",
	"audio/wav":       ".wav",
	"audio/mp4":       ".m4a",
	"application/pdf": ".pdf",
}

type Handler struct {
	uploadDir string
	baseURL   string
}

func NewHandler(uploadDir, baseURL string) (*Handler, error) {
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}
	return &Handler{uploadDir: uploadDir, baseURL: strings.TrimRight(baseURL, "/")}, nil
}

type UploadResponse struct {
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	MIMEType    string `json:"mime_type"`
	Size        int64  `json:"size"`
	MessageType string `json:"message_type"`
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respond.Error(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		respond.Error(w, http.StatusBadRequest, "file field is required")
		return
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	mime := strings.Split(http.DetectContentType(buf[:n]), ";")[0]

	msgType, ok := mimeMessageType[mime]
	if !ok {
		respond.Error(w, http.StatusBadRequest, "unsupported file type: "+mime)
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = mimeExt[mime]
	}
	filename := uuid.New().String() + ext

	dst, err := os.Create(filepath.Join(h.uploadDir, filename))
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer dst.Close()

	size, err := io.Copy(dst, file)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	respond.JSON(w, http.StatusOK, UploadResponse{
		URL:         h.baseURL + "/uploads/" + filename,
		Filename:    header.Filename,
		MIMEType:    mime,
		Size:        size,
		MessageType: msgType,
	})
}
