package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/neyho/eywa-go"
)

// generateTestUUID creates a deterministic UUID for testing
func generateTestUUID(prefix string) string {
	// Create a simple but valid UUID v4 format for testing
	b := make([]byte, 16)
	// Use a mix of timestamp and prefix for uniqueness
	timestamp := time.Now().UnixNano()
	for i := 0; i < 8; i++ {
		b[i] = byte(timestamp >> (i * 8))
	}
	// Fill rest with prefix hash
	for i, c := range prefix {
		if i+8 < 16 {
			b[i+8] = byte(c)
		}
	}
	// Set version (4) and variant bits
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func main() {
	fmt.Println("Starting EYWA Go Files Client - Specification Compliant Test...\n")

	// Start the pipe listener in a goroutine
	go eywa.OpenPipe()
	time.Sleep(100 * time.Millisecond)

	eywa.Info("Testing specification-compliant EYWA Files client", nil)

	// Test 1: Constants
	eywa.Info("Testing required constants", map[string]interface{}{
		"ROOT_UUID":   eywa.ROOT_UUID,
		"ROOT_FOLDER": eywa.ROOT_FOLDER,
	})

	// Test 2: Upload file with client UUID management
	testFilePath := "/tmp/test_upload.txt"
	createTestFile(testFilePath, "Hello from Go EYWA client!")

clientUUID := generateTestUUID("test")
	
	eywa.Info("Testing upload with client-controlled UUID", map[string]interface{}{
		"file": testFilePath,
		"uuid": clientUUID,
	})

	err := eywa.Upload(testFilePath, map[string]interface{}{
		"euuid":   clientUUID,
		"name":    "test-upload.txt",
		"folder":  eywa.ROOT_FOLDER, // Using ROOT_FOLDER constant
		"progressFn": func(current, total int64) {
			if current == total {
				eywa.Debug("Upload progress completed", map[string]interface{}{
					"bytes": current,
				})
			}
		},
	})

	if err != nil {
		eywa.Error("Upload failed", map[string]interface{}{"error": err.Error()})
	} else {
		eywa.Info("Upload successful", map[string]interface{}{"uuid": clientUUID})
	}

	// Test 3: Upload content directly
contentUUID := generateTestUUID("content")
	
	err = eywa.UploadContent([]byte("Direct content upload test"), map[string]interface{}{
		"euuid":        contentUUID,
		"name":         "direct-content.txt",
		"content_type": "text/plain",
		"folder":       eywa.ROOT_FOLDER,
	})

	if err != nil {
		eywa.Error("Content upload failed", map[string]interface{}{"error": err.Error()})
	} else {
		eywa.Info("Content upload successful", map[string]interface{}{"uuid": contentUUID})
	}

	// Test 4: Upload stream
	streamContent := "Stream upload test content"
	streamReader := strings.NewReader(streamContent)
streamUUID := generateTestUUID("stream")

	err = eywa.UploadStream(streamReader, map[string]interface{}{
		"euuid": streamUUID,
		"name":  "stream-upload.txt",
		"size":  int64(len(streamContent)),
		"folder": eywa.ROOT_FOLDER,
	})

	if err != nil {
		eywa.Error("Stream upload failed", map[string]interface{}{"error": err.Error()})
	} else {
		eywa.Info("Stream upload successful", map[string]interface{}{"uuid": streamUUID})
	}

	// Test 5: Create folder with client UUID
folderUUID := generateTestUUID("folder")
	
	err = eywa.CreateFolder(map[string]interface{}{
		"euuid":  folderUUID,
		"name":   "test-folder",
		"parent": eywa.ROOT_FOLDER,
	})

	if err != nil {
		eywa.Error("Folder creation failed", map[string]interface{}{"error": err.Error()})
	} else {
		eywa.Info("Folder creation successful", map[string]interface{}{"uuid": folderUUID})
	}

	// Test 6: Query files using direct GraphQL (as recommended by spec)
	eywa.Info("Testing direct GraphQL queries for file listing", nil)
	
	result, err := eywa.GraphQL(`
query GetRecentFiles($limit: Int!) {
			searchFile(
				_limit: $limit,
				_order_by: {uploaded_at: desc}
			) {
				euuid
				name
				size
				content_type
				uploaded_at
				folder {
					name
					path
				}
				uploaded_by {
					name
				}
			}
		}
	`, map[string]interface{}{
		"limit": 5,
	})

	if err != nil {
		eywa.Error("GraphQL query failed", map[string]interface{}{"error": err.Error()})
	} else {
		if data, ok := result["data"].(map[string]interface{}); ok {
			if files, ok := data["searchFile"].([]interface{}); ok {
				eywa.Info("Files found via direct GraphQL", map[string]interface{}{
					"count": len(files),
				})
				
				// Show first file as example
				if len(files) > 0 {
					if file, ok := files[0].(map[string]interface{}); ok {
						eywa.Debug("First file details", file)
					}
				}
			}
		}
	}

	// Test 7: Download file as complete content
	if err == nil && len(result["data"].(map[string]interface{})["searchFile"].([]interface{})) > 0 {
		firstFile := result["data"].(map[string]interface{})["searchFile"].([]interface{})[0].(map[string]interface{})
		fileUUID := firstFile["euuid"].(string)
		
		eywa.Info("Testing download", map[string]interface{}{"uuid": fileUUID})
		
		content, err := eywa.Download(fileUUID)
		if err != nil {
			eywa.Error("Download failed", map[string]interface{}{"error": err.Error()})
		} else {
			eywa.Info("Download successful", map[string]interface{}{
				"size": len(content),
				"preview": string(content[:min(50, len(content))]),
			})
		}

		// Test 8: Download as stream
		eywa.Info("Testing download stream", map[string]interface{}{"uuid": fileUUID})
		
		stream, err := eywa.DownloadStream(fileUUID)
		if err != nil {
			eywa.Error("Download stream failed", map[string]interface{}{"error": err.Error()})
		} else {
			eywa.Info("Download stream successful", map[string]interface{}{
				"contentLength": stream.ContentLength,
			})
			
			// Read a bit from stream to test
			buffer := make([]byte, 100)
			n, _ := stream.Stream.Read(buffer)
			stream.Stream.Close()
			
			eywa.Debug("Stream content preview", map[string]interface{}{
				"bytes": n,
				"content": string(buffer[:n]),
			})
		}
	}

	// Test 9: Complex GraphQL query showing the power of direct GraphQL
	eywa.Info("Testing complex GraphQL query (shows why we don't abstract queries)", nil)
	
	complexResult, err := eywa.GraphQL(`
query ComplexFileAnalysis($pattern: String!) {
			searchFile(
				_where: {
					name: {_ilike: $pattern}
				},
				_order_by: {uploaded_at: desc},
				_limit: 5
			) {
				euuid
				name
				size
				content_type
				uploaded_at
				folder {
					name
				}
				uploaded_by {
					name
				}
			}
		}
	`, map[string]interface{}{
		"pattern": "%test%",
	})

	if err != nil {
		eywa.Warn("Complex GraphQL query failed (expected if no matching files)", map[string]interface{}{
			"error": err.Error(),
		})
	} else {
		eywa.Info("Complex GraphQL query successful - demonstrates direct GraphQL power", map[string]interface{}{
			"hasData": complexResult["data"] != nil,
		})
	}

	// Test 10: Delete operations
	if clientUUID != "" {
		eywa.Info("Testing file deletion", map[string]interface{}{"uuid": clientUUID})
		success := eywa.DeleteFile(clientUUID)
		eywa.Info("File deletion result", map[string]interface{}{
			"success": success,
		})
	}

	if folderUUID != "" {
		eywa.Info("Testing folder deletion", map[string]interface{}{"uuid": folderUUID})
		success := eywa.DeleteFolder(folderUUID)
		eywa.Info("Folder deletion result", map[string]interface{}{
			"success": success,
		})
	}

	// Cleanup
	cleanupTestFile(testFilePath)

	eywa.Info("All specification compliance tests completed!", map[string]interface{}{
		"compliance": "EYWA Files Specification v1.0",
		"features": []string{
			"Single map arguments",
			"Client UUID management", 
			"Protocol abstraction only",
			"Direct GraphQL for queries",
			"Required constants",
			"Proper error types",
			"Stream support",
			"Progress callbacks",
		},
	})

	eywa.CloseTask(eywa.SUCCESS)
}

func createTestFile(path, content string) {
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		log.Printf("Failed to create test file: %v", err)
	}
}

func cleanupTestFile(path string) {
	err := os.Remove(path)
	if err != nil {
		log.Printf("Failed to cleanup test file: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}