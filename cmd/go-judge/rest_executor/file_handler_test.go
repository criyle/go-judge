package restexecutor

import (
	"bytes"
	"github.com/criyle/go-judge/filestore"
	"github.com/gin-gonic/gin"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
)

func TestFilePost(t *testing.T) {
	// Create a temporary directory for the file store
	tempDir, err := os.MkdirTemp("", "test_storage")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir) // Clean up after test

	// Initialize the file store
	router := gin.Default()
	f := &fileHandle{fs: filestore.NewFileLocalStore(tempDir)}
	router.POST("/file", f.filePost)

	// Create a buffer to simulate multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create a form file
	fileWriter, err := writer.CreateFormFile("file", "test.py")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	// Write some content to the file
	contentToWrite := "print(58 - 7 * 3)"
	_, err = fileWriter.Write([]byte(contentToWrite))
	if err != nil {
		t.Fatalf("Failed to write to form file: %v", err)
	}

	err = writer.Close()
	if err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	// Create HTTP request
	req := httptest.NewRequest("POST", "/file", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Record the response
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check the response status code
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	fileID := w.Body.String()
	// Check if the length of fileID is correct
	if len(fileID) <= 3 {
		t.Fatalf("Expected file ID length greater than 3, got %d", len(fileID))
	}
	// Remove quotes from the response
	fileID = fileID[1 : len(fileID)-1]

	// Check if the file is stored correctly
	filePath := path.Join(tempDir, fileID)
	_, err = os.Stat(filePath)
	if os.IsNotExist(err) {
		t.Fatalf("File should exist in the storage: %v", err)
	}
	// Read the file content
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()
	content := make([]byte, len(contentToWrite))
	_, err = file.Read(content)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	// Check if the content matches
	if string(content) != contentToWrite {
		t.Fatalf("File content does not match: expected %s, got %s", contentToWrite, string(content))
	}
}
