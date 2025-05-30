package restexecutor

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/criyle/go-judge/filestore"
	"github.com/gin-gonic/gin"
)

func TestFilePost(t *testing.T) {
	// Create a temporary directory for the file store
	tempDir := t.TempDir()

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
	filePath := filepath.Join(tempDir, fileID)
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

// CreateFileWithContent creates a file with the specified content in the given directory
func CreateFileWithContent(filePath, content string) error {
	// Create a file with the specified content
	return os.WriteFile(filePath, []byte(content), 0644)
}

// TestFileGet tests the file retrieval functionality
func TestFileGet(t *testing.T) {
	// Create a temporary directory for the file store
	tempDir := t.TempDir()

	// Initialize the file store
	router := gin.Default()
	f := &fileHandle{fs: filestore.NewFileLocalStore(tempDir)}
	router.GET("/file", f.fileGet)

	type FileToCreate struct {
		Name    string
		Content string
	}

	filesToCreate := []FileToCreate{
		{"test1.py", "print(58 - 7 * 3)"},
		{"test2.py", "print(58 + 7 * 3)"},
		{"test3.py", "print(58 / 7 * 3)"},
	}

	// Create files in the temporary directory
	for _, file := range filesToCreate {
		filePath := filepath.Join(tempDir, file.Name)
		err := CreateFileWithContent(filePath, file.Content)
		if err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	// Create HTTP request
	req := httptest.NewRequest("GET", "/file", nil)

	// Record the response
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check the response status code
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	fileIDs := w.Body.String()
	t.Logf("File IDs: %s", fileIDs)
	for _, file := range filesToCreate {
		testFileName := file.Name
		// Check if the file ID is present in the response
		if !strings.Contains(fileIDs, testFileName) {
			t.Fatalf("Expected file ID %s to be present in the response", testFileName)
		}
	}
}

// TestFileIDGet tests the file retrieval by ID functionality
func TestFileIDGet(t *testing.T) {
	// Create a temporary directory for the file store
	tempDir := t.TempDir()

	// Initialize the file store
	router := gin.Default()
	f := &fileHandle{fs: filestore.NewFileLocalStore(tempDir)}
	router.GET("/file/:fid", f.fileIDGet)

	// Create a test file
	testFileName := "test.py"
	testFilePath := filepath.Join(tempDir, testFileName)
	err := CreateFileWithContent(testFilePath, "print(58 - 7 * 3)")
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Add the file to the file store
	fileID, err := f.fs.Add(testFileName, testFilePath)
	if err != nil {
		t.Fatalf("Failed to add file to storage: %v", err)
	}

	// Create HTTP request
	req := httptest.NewRequest("GET", "/file/"+fileID, nil)

	// Record the response
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check the response status code
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	bodyBytes, err := os.ReadFile(filepath.Join(tempDir, fileID))
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	bodyString := string(bodyBytes)

	expectedContent := "print(58 - 7 * 3)"
	if bodyString != expectedContent {
		t.Fatalf("Expected response body %s, got %s", expectedContent, bodyString)
	}
}

// TestFileIDDelete tests the file deletion functionality
func TestFileIDDelete(t *testing.T) {
	// Create a temporary directory for the file store
	tempDir := t.TempDir()

	// Initialize the file store
	router := gin.Default()
	f := &fileHandle{fs: filestore.NewFileLocalStore(tempDir)}
	router.DELETE("/file/:fid", f.fileIDDelete)

	// Create a test file
	testFileName := "test.py"
	testFilePath := filepath.Join(tempDir, testFileName)
	err := CreateFileWithContent(testFilePath, "print(58 - 7 * 3)")
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Add the file to the file store
	fileID, err := f.fs.Add(testFileName, testFilePath)
	if err != nil {
		t.Fatalf("Failed to add file to storage: %v", err)
	}

	// Create HTTP request
	req := httptest.NewRequest("DELETE", "/file/"+fileID, nil)

	// Record the response
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Check the response status code
	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check if the file is deleted from the storage
	if _, err := os.Stat(filepath.Join(tempDir, fileID)); !os.IsNotExist(err) {
		t.Fatalf("Expected file to be deleted, but it still exists")
	}
}
