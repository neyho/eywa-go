# EYWA Files Client for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/neyho/eywa-go.svg)](https://pkg.go.dev/github.com/neyho/eywa-go)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**Specification-compliant EYWA files client** providing protocol abstraction for upload/download complexity while letting you write direct GraphQL queries for data operations.

## 🎯 Core Principles

- **GraphQL Schema Compliance** - All operations work with current EYWA GraphQL schema
- **Client UUID Management** - You control UUID generation and file replacement
- **Single Map Arguments** - Functions use single map arguments that mirror GraphQL schema
- **No Parameter Mangling** - Direct pass-through to GraphQL without client-side transformation
- **Protocol Focus** - Abstract protocol complexity, not query complexity
- **Let GraphQL Be GraphQL** - Write your own queries for complex data retrieval

## Installation

```bash
go get github.com/neyho/eywa-go
```

## Quick Start

```go
package main

import (
    "time"
    "github.com/neyho/eywa-go"
)

func main() {
    // Initialize EYWA connection
    go eywa.OpenPipe()
    time.Sleep(100 * time.Millisecond)
    
    // Upload a file with client-controlled UUID
    err := eywa.Upload("/path/to/file.txt", map[string]interface{}{
        "euuid": "my-controlled-uuid-123",
        "name":  "uploaded-file.txt", 
        "folder": eywa.ROOT_FOLDER,
    })
    
    if err != nil {
        eywa.Error("Upload failed", map[string]interface{}{"error": err.Error()})
        eywa.CloseTask(eywa.ERROR)
        return
    }
    
    // Query files using direct GraphQL (recommended approach)
    result, err := eywa.GraphQL(`
        query GetMyFiles($pattern: String!) {
            searchFile(_where: {name: {_ilike: $pattern}}) {
                euuid name size uploaded_at
                folder { name path }
            }
        }
    `, map[string]interface{}{
        "pattern": "%uploaded%",
    })
    
    // Download file content
    if len(result["data"].(map[string]interface{})["searchFile"].([]interface{})) > 0 {
        fileUUID := result["data"].(map[string]interface{})["searchFile"].([]interface{})[0].(map[string]interface{})["euuid"].(string)
        content, err := eywa.Download(fileUUID)
        if err == nil {
            eywa.Info("Downloaded file", map[string]interface{}{"size": len(content)})
        }
    }
    
    eywa.CloseTask(eywa.SUCCESS)
}
```

## 🏗️ API Reference

### Required Constants

```go
const ROOT_UUID = "87ce50d8-5dfa-4008-a265-053e727ab793"
var ROOT_FOLDER = map[string]interface{}{"euuid": ROOT_UUID}
```

### Exception Types

```go
type FileUploadError struct {
    Message string
    Type    string
    Code    *int
}

type FileDownloadError struct {
    Message string  
    Type    string
    Code    *int
}
```

## 📤 Upload Operations (Protocol Abstraction)

### Upload(filepath, fileData)

Upload a file from local filesystem using the 3-step S3 protocol.

```go
err := eywa.Upload("/path/to/file.pdf", map[string]interface{}{
    "euuid":        "client-generated-uuid",     // Optional: client controls UUID
    "name":         "report.pdf",               // Optional: override filename  
    "folder":       map[string]interface{}{"euuid": folderUuid}, // Optional: target folder
    "content_type": "application/pdf",          // Optional: override MIME type
    "progressFn":   func(current, total int64) {
        fmt.Printf("Progress: %d/%d bytes\n", current, total)
    },
})
```

**Key Features:**
- Client controls UUID generation for guaranteed file replacement
- Auto-detects file size, MIME type, and filename if not provided
- 3-step protocol: request URL → upload to S3 → confirm
- Progress tracking support

### UploadStream(inputStream, fileData)

Upload from a stream (size must be known).

```go
reader := strings.NewReader("Hello, World!")
err := eywa.UploadStream(reader, map[string]interface{}{
    "euuid": "stream-upload-uuid",
    "name":  "hello.txt", 
    "size":  int64(13),                        // Required for streams
    "folder": eywa.ROOT_FOLDER,
})
```

### UploadContent(content, fileData)

Upload bytes or string content directly.

```go
content := []byte("Direct content upload")
err := eywa.UploadContent(content, map[string]interface{}{
    "euuid":        "content-uuid",
    "name":         "content.txt",
    "content_type": "text/plain",              // Defaults to "text/plain"
    "folder":       eywa.ROOT_FOLDER,
})
```

## 📥 Download Operations (Protocol Abstraction)

### Download(fileUuid)

Download complete file as bytes.

```go
content, err := eywa.Download("file-uuid-here")
if err != nil {
    // Handle download error
}
// Use content ([]byte)
```

### DownloadStream(fileUuid)

Download as a stream for large files.

```go
stream, err := eywa.DownloadStream("large-file-uuid")
if err != nil {
    // Handle error
}
defer stream.Stream.Close()

// Read from stream.Stream (io.ReadCloser)
// stream.ContentLength gives total size
buffer := make([]byte, 8192)
for {
    n, err := stream.Stream.Read(buffer)
    if n > 0 {
        // Process chunk
    }
    if err == io.EOF {
        break
    }
}
```

## 📋 Simple CRUD Operations

### CreateFolder(folderData)

Create a new folder.

```go
err := eywa.CreateFolder(map[string]interface{}{
    "euuid":  "folder-uuid",                   // Optional: client controls UUID
    "name":   "My Documents",                  // Required: folder name
    "parent": eywa.ROOT_FOLDER,                // Optional: parent folder (use ROOT_FOLDER for root)
})
```

### DeleteFile(fileUuid) / DeleteFolder(folderUuid) 

Delete files or empty folders.

```go
success := eywa.DeleteFile("file-uuid")       // Returns true if successful
success := eywa.DeleteFolder("folder-uuid")   // Folder must be empty
```

## ❌ What's NOT Included (Use Direct GraphQL Instead)

This client **intentionally omits** query functions. Write direct GraphQL for better control:

```go
// ❌ DON'T use helper functions like ListFiles(), SearchFiles(), GetFileInfo()
// ✅ DO write direct GraphQL queries:

// Simple file listing
files, err := eywa.GraphQL(`
    query GetRecentFiles($limit: Int!) {
        searchFile(_limit: $limit, _order_by: {uploaded_at: desc}) {
            euuid name size content_type uploaded_at
            folder { name path }
            uploaded_by { name }
        }
    }
`, map[string]interface{}{"limit": 10})

// Complex filtering and relationships
result, err := eywa.GraphQL(`
    query FindLargeImageFiles {
        searchFile(_where: {
            _and: [
                {content_type: {_like: "image%"}},
                {size: {_gt: 1048576}},              # > 1MB
                {status: {_eq: "UPLOADED"}},
                {uploaded_at: {_gt: "2024-01-01"}}
            ]
        }, _order_by: {size: desc}) {
            euuid name size content_type
            folder(_where: {name: {_eq: "photos"}}) {
                name path
                parent { name }
            }
            uploaded_by { name email }
        }
        
        # Get statistics in same query  
        stats: searchFile_aggregate(_where: {content_type: {_like: "image%"}}) {
            aggregate {
                count
                sum { size }
                avg { size }
            }
        }
    }
`)
```

**Why this is better:**
- **No abstraction leakage** - You see exactly what GraphQL executes
- **Full GraphQL power** - Use any GraphQL features (counts, relations, custom ordering)
- **No translation bugs** - No client-side parameter conversion errors
- **Future-proof** - New GraphQL schema features work immediately
- **Less code** - No complex filtering logic to maintain

## 🔧 Complete Example

```go
package main

import (
    "fmt"
    "log"
    "strings"
    "time"
    
    "github.com/neyho/eywa-go"
)

func main() {
    // Initialize
    go eywa.OpenPipe()
    time.Sleep(100 * time.Millisecond)
    
    // Upload with client UUID control
    clientUUID := fmt.Sprintf("report-%d", time.Now().Unix())
    
    err := eywa.Upload("./monthly-report.pdf", map[string]interface{}{
        "euuid":   clientUUID,                   // Client controls UUID
        "name":    "January-2024-Report.pdf",   
        "folder":  map[string]interface{}{"euuid": "reports-folder-uuid"},
        "progressFn": func(current, total int64) {
            fmt.Printf("Uploading: %.1f%%\n", float64(current)/float64(total)*100)
        },
    })
    
    if err != nil {
        eywa.Error("Upload failed", map[string]interface{}{"error": err.Error()})
        eywa.CloseTask(eywa.ERROR)
        return
    }
    
    // Query uploaded reports using direct GraphQL
    reports, err := eywa.GraphQL(`
        query GetMonthlyReports($folderUuid: UUID!) {
            searchFile(_where: {
                _and: [
                    {folder: {euuid: {_eq: $folderUuid}}},
                    {name: {_ilike: "%-Report.pdf"}},
                    {status: {_eq: "UPLOADED"}}
                ]
            }, _order_by: {uploaded_at: desc}) {
                euuid
                name
                size
                uploaded_at
                uploaded_by { name }
            }
        }
    `, map[string]interface{}{
        "folderUuid": "reports-folder-uuid",
    })
    
    if err != nil {
        eywa.Error("Query failed", map[string]interface{}{"error": err.Error()})
        return
    }
    
    // Process results
    if data, ok := reports["data"].(map[string]interface{}); ok {
        if files, ok := data["searchFile"].([]interface{}); ok {
            eywa.Info("Found reports", map[string]interface{}{"count": len(files)})
            
            for _, file := range files {
                if f, ok := file.(map[string]interface{}); ok {
                    // Download each report for processing
                    content, err := eywa.Download(f["euuid"].(string))
                    if err != nil {
                        continue
                    }
                    
                    // Process PDF content...
                    eywa.Info("Processed report", map[string]interface{}{
                        "name": f["name"],
                        "size": len(content),
                    })
                }
            }
        }
    }
    
    // Create organized folder structure
    archiveFolder := fmt.Sprintf("archive-%d", time.Now().Year())
    err = eywa.CreateFolder(map[string]interface{}{
        "euuid":  fmt.Sprintf("archive-%d-uuid", time.Now().Year()),
        "name":   archiveFolder,
        "parent": map[string]interface{}{"euuid": "reports-folder-uuid"},
    })
    
    if err != nil {
        eywa.Warn("Archive folder creation failed", map[string]interface{}{"error": err.Error()})
    }
    
    eywa.CloseTask(eywa.SUCCESS)
}
```

## 🛡️ Error Handling

```go
// Upload errors
err := eywa.Upload("/nonexistent/file.txt", map[string]interface{}{
    "name": "test.txt",
})
if uploadErr, ok := err.(*eywa.FileUploadError); ok {
    fmt.Printf("Upload error: %s (type: %s)\n", uploadErr.Message, uploadErr.Type)
}

// Download errors  
content, err := eywa.Download("invalid-uuid")
if downloadErr, ok := err.(*eywa.FileDownloadError); ok {
    fmt.Printf("Download error: %s\n", downloadErr.Message)
}
```

## 🧪 Testing

Run the specification compliance test:

```bash
cd examples
eywa run -c 'go run files_compliance_test.go'
```

This test verifies:
- ✅ All required constants exist
- ✅ Single map argument APIs
- ✅ Client UUID management 
- ✅ Protocol abstraction (3-step upload)
- ✅ Stream support and progress callbacks
- ✅ Direct GraphQL usage for queries
- ✅ Proper error types
- ✅ CRUD operations work correctly

## 🔄 Migration from Old API

If you have existing code using the old API, here's how to migrate:

```go
// OLD (non-compliant):
fileInfo, err := eywa.UploadFile("/path/file.txt", &eywa.UploadFileOptions{
    Name:       "custom.txt",
    FolderUUID: "folder-id",
})
files, err := eywa.ListFiles(&eywa.ListFilesOptions{Limit: &limit})

// NEW (specification-compliant):
err := eywa.Upload("/path/file.txt", map[string]interface{}{
    "name":   "custom.txt", 
    "folder": map[string]interface{}{"euuid": "folder-id"},
})

// Use direct GraphQL instead of ListFiles:
result, err := eywa.GraphQL(`
    query GetFiles($limit: Int!) {
        searchFile(_limit: $limit) { euuid name size }
    }
`, map[string]interface{}{"limit": 10})
```

## 📋 Requirements

- Go 1.19+
- EYWA server connection via `eywa run` command
- No external dependencies (uses only standard library)

## 🎯 Success Criteria

This implementation satisfies the EYWA Files specification by:

1. ✅ **Handling upload/download protocols** - Abstracts 3-step S3 complexity
2. ✅ **Providing simple CRUD mutations** - Basic create/delete operations  
3. ✅ **Letting GraphQL be GraphQL** - No query abstraction, direct user control
4. ✅ **Supporting all required functions** - 8 core functions implemented
5. ✅ **Proper error handling** - Typed exceptions with meaningful messages
6. ✅ **Progress tracking** - For upload/download operations
7. ✅ **Client UUID management** - Users control file UUIDs for replacement
8. ✅ **Single map arguments** - Mirror GraphQL schema directly

## 📖 Philosophy

This client is a **thin, reliable protocol layer** - not a query abstraction layer. It handles what's complex (S3 upload protocol) and lets you control what should be flexible (data queries via GraphQL).

**The goal: Make file upload/download simple while keeping data queries powerful.**

## 📜 License

MIT

## 🤝 Contributing

This implementation follows the [EYWA Files Client Functional Specification](./FILES_SPEC.md). When contributing, ensure all changes maintain specification compliance.

## 📞 Support

For issues and questions, please visit the [EYWA repository](https://github.com/neyho/eywa).
