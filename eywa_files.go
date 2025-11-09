// EYWA File Management Extensions for Go
//
// This package provides protocol abstraction for EYWA file operations.
// Focuses on upload/download complexity while letting users write their own GraphQL queries.
//
// Key principles:
// - Single map arguments that mirror GraphQL schema
// - Client controls UUID generation and management
// - No query abstraction - users write direct GraphQL
// - Protocol focus - abstract S3 complexity, not query complexity

package eywa

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

// Constants as required by specification
const ROOT_UUID = "87ce50d8-5dfa-4008-a265-053e727ab793"

var ROOT_FOLDER = map[string]interface{}{
	"euuid": ROOT_UUID,
}

// Exception/Error Types as required by specification
type FileUploadError struct {
	Message string
	Type    string
	Code    *int
}

func (e *FileUploadError) Error() string {
	return fmt.Sprintf("File upload error: %s", e.Message)
}

func NewFileUploadError(message string) *FileUploadError {
	return &FileUploadError{
		Message: message,
		Type:    "upload-error",
	}
}

type FileDownloadError struct {
	Message string
	Type    string
	Code    *int
}

func (e *FileDownloadError) Error() string {
	return fmt.Sprintf("File download error: %s", e.Message)
}

func NewFileDownloadError(message string) *FileDownloadError {
	return &FileDownloadError{
		Message: message,
		Type:    "download-error",
	}
}

// DownloadStreamResult represents a download stream with content length
type DownloadStreamResult struct {
	Stream        io.ReadCloser
	ContentLength int64
}

// Progress callback type
type ProgressFn func(current, total int64)

// generateUUID creates a new UUID v4 for client-side UUID management
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	// Set version (4) and variant bits according to RFC 4122
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Upload uploads a file from local filesystem to EYWA using the 3-step protocol.
//
// Parameters:
//   - filepath: string - Local file path
//   - fileData: map[string]interface{} - File metadata matching GraphQL schema:
//     {
//       euuid?: string - Client-generated UUID (optional, auto-generated if not provided)
//       name?: string - Override filename (defaults to filepath basename)
//       folder?: map[string]interface{} - Target folder: {euuid: string} OR {path: string}
//       content_type?: string - Override MIME type (auto-detected if not provided)
//       size?: int64 - File size (auto-detected)
//       progressFn?: ProgressFn - Progress callback
//     }
//
// Returns: error (null on success)
func Upload(filePath string, fileData map[string]interface{}) error {
	if fileData == nil {
		fileData = make(map[string]interface{})
	}

	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("File not found: %s", filePath))
	}

	if fileInfo.IsDir() {
		return NewFileUploadError(fmt.Sprintf("Path is a directory, not a file: %s", filePath))
	}

	// Prepare file metadata, defaulting missing values
	size := fileInfo.Size()
	name := getStringFromData(fileData, "name", filepath.Base(filePath))
	contentType := getStringFromData(fileData, "content_type", detectMimeType(filePath))
	euuid := getStringFromData(fileData, "euuid", generateUUID())
	
	var progressFn ProgressFn
	if fn, ok := fileData["progressFn"].(ProgressFn); ok {
		progressFn = fn
	}

	Info(fmt.Sprintf("Starting upload: %s (%d bytes)", name, size), nil)

	// Step 1: Request upload URL
	uploadMutation := `
		mutation RequestUploadURL($file: FileInput!) {
			requestUploadURL(file: $file)
		}
	`

	variables := map[string]interface{}{
		"file": map[string]interface{}{
			"euuid":        euuid,
			"name":         name,
			"content_type": contentType,
			"size":         size,
		},
	}

	// Add folder if specified
	if folder, ok := fileData["folder"].(map[string]interface{}); ok {
		variables["file"].(map[string]interface{})["folder"] = folder
	}

	result, err := GraphQL(uploadMutation, variables)
	if err != nil {
		Error("Upload failed", map[string]interface{}{"error": err.Error()})
		return NewFileUploadError(fmt.Sprintf("Upload failed: %s", err.Error()))
	}

	uploadURL, ok := result["data"].(map[string]interface{})["requestUploadURL"].(string)
	if !ok {
		return NewFileUploadError("Failed to get upload URL from response")
	}

	Debug(fmt.Sprintf("Upload URL received: %s...", uploadURL[:minInt(50, len(uploadURL))]), nil)

	// Step 2: Upload file to S3
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("Failed to read file: %s", err.Error()))
	}

	if progressFn != nil {
		progressFn(0, size)
	}

	err = httpPutRequest(uploadURL, fileBytes, map[string]string{
		"Content-Type": contentType,
	})
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("S3 upload failed: %s", err.Error()))
	}

	if progressFn != nil {
		progressFn(size, size)
	}

	Debug("File uploaded to S3 successfully", nil)

	// Step 3: Confirm upload
	confirmMutation := `
		mutation ConfirmFileUpload($url: String!) {
			confirmFileUpload(url: $url)
		}
	`

	confirmResult, err := GraphQL(confirmMutation, map[string]interface{}{
		"url": uploadURL,
	})
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("Upload confirmation failed: %s", err.Error()))
	}

	confirmed, ok := confirmResult["data"].(map[string]interface{})["confirmFileUpload"].(bool)
	if !ok || !confirmed {
		return NewFileUploadError("Upload confirmation failed")
	}

	Debug("Upload confirmed", nil)
	Info(fmt.Sprintf("Upload completed: %s -> %s", name, euuid), nil)
	return nil
}

// UploadStream uploads from a stream to EYWA.
//
// Parameters:
//   - inputStream: io.Reader - Readable stream of file data
//   - fileData: map[string]interface{} - File metadata (size required):
//     {
//       euuid?: string - Client-generated UUID (optional)
//       name: string - Filename (required)
//       size: int64 - Content size (required)
//       folder?: map[string]interface{} - Target folder
//       content_type?: string - MIME type (defaults to "application/octet-stream")
//       progressFn?: ProgressFn - Progress callback
//     }
//
// Returns: error (null on success)
func UploadStream(inputStream io.Reader, fileData map[string]interface{}) error {
	if fileData == nil {
		return NewFileUploadError("fileData is required")
	}

	name, ok := fileData["name"].(string)
	if !ok || name == "" {
		return NewFileUploadError("name is required for stream upload")
	}

	var size int64
	if s, ok := fileData["size"].(int64); ok {
		size = s
	} else if s, ok := fileData["size"].(int); ok {
		size = int64(s)
	} else {
		return NewFileUploadError("size is required for stream upload")
	}

	euuid := getStringFromData(fileData, "euuid", generateUUID())
	contentType := getStringFromData(fileData, "content_type", "application/octet-stream")
	
	var progressFn ProgressFn
	if fn, ok := fileData["progressFn"].(ProgressFn); ok {
		progressFn = fn
	}

	Info(fmt.Sprintf("Starting stream upload: %s (%d bytes)", name, size), nil)

	// Read all content from stream
	content, err := io.ReadAll(inputStream)
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("Failed to read from stream: %s", err.Error()))
	}

	if int64(len(content)) != size {
		return NewFileUploadError(fmt.Sprintf("Content size mismatch: expected %d, got %d", size, len(content)))
	}

	// Step 1: Request upload URL
	uploadMutation := `
		mutation RequestUploadURL($file: FileInput!) {
			requestUploadURL(file: $file)
		}
	`

	variables := map[string]interface{}{
		"file": map[string]interface{}{
			"euuid":        euuid,
			"name":         name,
			"content_type": contentType,
			"size":         size,
		},
	}

	// Add folder if specified
	if folder, ok := fileData["folder"].(map[string]interface{}); ok {
		variables["file"].(map[string]interface{})["folder"] = folder
	}

	result, err := GraphQL(uploadMutation, variables)
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("Upload failed: %s", err.Error()))
	}

	uploadURL, ok := result["data"].(map[string]interface{})["requestUploadURL"].(string)
	if !ok {
		return NewFileUploadError("Failed to get upload URL from response")
	}

	// Step 2: Upload to S3
	if progressFn != nil {
		progressFn(0, size)
	}

	err = httpPutRequest(uploadURL, content, map[string]string{
		"Content-Type": contentType,
	})
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("S3 upload failed: %s", err.Error()))
	}

	if progressFn != nil {
		progressFn(size, size)
	}

	// Step 3: Confirm upload
	confirmMutation := `
		mutation ConfirmFileUpload($url: String!) {
			confirmFileUpload(url: $url)
		}
	`

	confirmResult, err := GraphQL(confirmMutation, map[string]interface{}{
		"url": uploadURL,
	})
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("Upload confirmation failed: %s", err.Error()))
	}

	confirmed, ok := confirmResult["data"].(map[string]interface{})["confirmFileUpload"].(bool)
	if !ok || !confirmed {
		return NewFileUploadError("Upload confirmation failed")
	}

	Info(fmt.Sprintf("Stream upload completed: %s -> %s", name, euuid), nil)
	return nil
}

// UploadContent uploads string or binary content directly.
//
// Parameters:
//   - content: []byte - Content to upload
//   - fileData: map[string]interface{} - File metadata:
//     {
//       euuid?: string - Client-generated UUID (optional)
//       name: string - Filename (required)
//       folder?: map[string]interface{} - Target folder
//       content_type?: string - MIME type (defaults to "text/plain")
//       progressFn?: ProgressFn - Progress callback
//     }
//
// Returns: error (null on success)
func UploadContent(content []byte, fileData map[string]interface{}) error {
	if fileData == nil {
		return NewFileUploadError("fileData is required")
	}

	name, ok := fileData["name"].(string)
	if !ok || name == "" {
		return NewFileUploadError("name is required for content upload")
	}

	size := int64(len(content))
	euuid := getStringFromData(fileData, "euuid", generateUUID())
	contentType := getStringFromData(fileData, "content_type", "text/plain")
	
	var progressFn ProgressFn
	if fn, ok := fileData["progressFn"].(ProgressFn); ok {
		progressFn = fn
	}

	Info(fmt.Sprintf("Starting content upload: %s (%d bytes)", name, size), nil)

	// Step 1: Request upload URL
	uploadMutation := `
		mutation RequestUploadURL($file: FileInput!) {
			requestUploadURL(file: $file)
		}
	`

	variables := map[string]interface{}{
		"file": map[string]interface{}{
			"euuid":        euuid,
			"name":         name,
			"content_type": contentType,
			"size":         size,
		},
	}

	// Add folder if specified
	if folder, ok := fileData["folder"].(map[string]interface{}); ok {
		variables["file"].(map[string]interface{})["folder"] = folder
	}

	result, err := GraphQL(uploadMutation, variables)
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("Content upload failed: %s", err.Error()))
	}

	uploadURL, ok := result["data"].(map[string]interface{})["requestUploadURL"].(string)
	if !ok {
		return NewFileUploadError("Failed to get upload URL from response")
	}

	// Step 2: Upload to S3
	if progressFn != nil {
		progressFn(0, size)
	}

	err = httpPutRequest(uploadURL, content, map[string]string{
		"Content-Type": contentType,
	})
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("S3 upload failed: %s", err.Error()))
	}

	if progressFn != nil {
		progressFn(size, size)
	}

	// Step 3: Confirm upload
	confirmMutation := `
		mutation ConfirmFileUpload($url: String!) {
			confirmFileUpload(url: $url)
		}
	`

	confirmResult, err := GraphQL(confirmMutation, map[string]interface{}{
		"url": uploadURL,
	})
	if err != nil {
		return NewFileUploadError(fmt.Sprintf("Upload confirmation failed: %s", err.Error()))
	}

	confirmed, ok := confirmResult["data"].(map[string]interface{})["confirmFileUpload"].(bool)
	if !ok || !confirmed {
		return NewFileUploadError("Upload confirmation failed")
	}

	Info(fmt.Sprintf("Content upload completed: %s -> %s", name, euuid), nil)
	return nil
}

// DownloadStream downloads file as a stream.
//
// Parameters:
//   - fileUuid: string - UUID of file to download
//
// Returns: 
//   - *DownloadStreamResult - Stream with content length
//   - error - Error if download fails
func DownloadStream(fileUuid string) (*DownloadStreamResult, error) {
	Info(fmt.Sprintf("Starting stream download: %s", fileUuid), nil)

	// Step 1: Request download URL
	downloadQuery := `
		query RequestDownloadURL($file: FileInput!) {
			requestDownloadURL(file: $file)
		}
	`

	result, err := GraphQL(downloadQuery, map[string]interface{}{
		"file": map[string]interface{}{
			"euuid": fileUuid,
		},
	})
	if err != nil {
		Error("Download failed", map[string]interface{}{"error": err.Error()})
		return nil, NewFileDownloadError(fmt.Sprintf("Download failed: %s", err.Error()))
	}

	downloadURL, ok := result["data"].(map[string]interface{})["requestDownloadURL"].(string)
	if !ok {
		return nil, NewFileDownloadError("Failed to get download URL from response")
	}

	Debug(fmt.Sprintf("Download URL received: %s...", downloadURL[:minInt(50, len(downloadURL))]), nil)

	// Step 2: Create HTTP request for streaming
	resp, err := http.Get(downloadURL)
	if err != nil {
		return nil, NewFileDownloadError(fmt.Sprintf("Download failed: %s", err.Error()))
	}

	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, NewFileDownloadError(fmt.Sprintf("Download failed with status: %d", resp.StatusCode))
	}

	return &DownloadStreamResult{
		Stream:        resp.Body,
		ContentLength: resp.ContentLength,
	}, nil
}

// Download downloads file as complete buffer/data.
//
// Parameters:
//   - fileUuid: string - UUID of file to download
//
// Returns: []byte - Complete file content
func Download(fileUuid string) ([]byte, error) {
	stream, err := DownloadStream(fileUuid)
	if err != nil {
		return nil, err
	}
	defer stream.Stream.Close()

	content, err := io.ReadAll(stream.Stream)
	if err != nil {
		return nil, NewFileDownloadError(fmt.Sprintf("Failed to read download content: %s", err.Error()))
	}

	Info(fmt.Sprintf("Download completed: %s (%d bytes)", fileUuid, len(content)), nil)
	return content, nil
}

// CreateFolder creates a new folder.
//
// Parameters:
//   - folderData: map[string]interface{} - Folder definition:
//     {
//       euuid?: string - Client-generated UUID (optional)
//       name: string - Folder name (required)
//       parent?: map[string]interface{} - Parent folder: {euuid: string} (use ROOT_UUID for root level)
//     }
//
// Returns: error (null on success)
func CreateFolder(folderData map[string]interface{}) error {
	if folderData == nil {
		return fmt.Errorf("folderData is required")
	}

	name, ok := folderData["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("name is required for folder creation")
	}

	// Prepare folder data, defaulting missing values
	euuid := getStringFromData(folderData, "euuid", generateUUID())

	mutation := `
		mutation CreateFolder($folder: FolderInput!) {
			stackFolder(data: $folder) {
				euuid name path modified_on
				parent { euuid name }
			}
		}
	`

	variables := map[string]interface{}{
		"folder": map[string]interface{}{
			"euuid": euuid,
			"name":  name,
		},
	}

	// Add parent folder if specified
	if parent, ok := folderData["parent"].(map[string]interface{}); ok {
		variables["folder"].(map[string]interface{})["parent"] = parent
	}

	result, err := GraphQL(mutation, variables)
	if err != nil {
		return fmt.Errorf("folder creation failed: %s", err.Error())
	}

	if result["data"] == nil {
		return fmt.Errorf("folder creation failed: no data returned")
	}

	Info(fmt.Sprintf("Folder created: %s -> %s", name, euuid), nil)
	return nil
}

// DeleteFile deletes a file from EYWA.
//
// Parameters:
//   - fileUuid: string - UUID of file to delete
//
// Returns: bool - true if deleted successfully
func DeleteFile(fileUuid string) bool {
	mutation := `
		mutation DeleteFile($uuid: UUID!) {
			deleteFile(euuid: $uuid)
		}
	`

	result, err := GraphQL(mutation, map[string]interface{}{
		"uuid": fileUuid,
	})
	if err != nil {
		Error("Failed to delete file", map[string]interface{}{"error": err.Error()})
		return false
	}

	success, ok := result["data"].(map[string]interface{})["deleteFile"].(bool)
	if !ok {
		Warn("Unexpected response format for file deletion", nil)
		return false
	}

	if success {
		Info(fmt.Sprintf("File deleted: %s", fileUuid), nil)
	} else {
		Warn(fmt.Sprintf("File deletion failed: %s", fileUuid), nil)
	}

	return success
}

// DeleteFolder deletes an empty folder.
//
// Parameters:
//   - folderUuid: string - UUID of folder to delete
//
// Returns: bool - true if deleted
//
// Requirements:
//   - Folder must be empty (no files or subfolders)
func DeleteFolder(folderUuid string) bool {
	mutation := `
		mutation DeleteFolder($uuid: UUID!) {
			deleteFolder(euuid: $uuid)
		}
	`

	result, err := GraphQL(mutation, map[string]interface{}{
		"uuid": folderUuid,
	})
	if err != nil {
		Error("Failed to delete folder", map[string]interface{}{"error": err.Error()})
		return false
	}

	success, ok := result["data"].(map[string]interface{})["deleteFolder"].(bool)
	if !ok {
		Warn("Unexpected response format for folder deletion", nil)
		return false
	}

	if success {
		Info(fmt.Sprintf("Folder deleted: %s", folderUuid), nil)
	} else {
		Warn(fmt.Sprintf("Folder deletion failed: %s", folderUuid), nil)
	}

	return success
}

// Helper functions

func getStringFromData(data map[string]interface{}, key, defaultValue string) string {
	if val, ok := data[key].(string); ok && val != "" {
		return val
	}
	return defaultValue
}

func detectMimeType(filePath string) string {
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func httpPutRequest(url string, data []byte, headers map[string]string) error {
	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return err
	}

	// Set Content-Length explicitly (required for S3)
	req.ContentLength = int64(len(data))
	
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}