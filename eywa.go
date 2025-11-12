package eywa

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
)

// Task status constants
const (
	SUCCESS    = "SUCCESS"
	ERROR      = "ERROR"
	PROCESSING = "PROCESSING"
	EXCEPTION  = "EXCEPTION"
)

// Log event types
const (
	INFO      = "INFO"
	WARN      = "WARN"
	DEBUG     = "DEBUG"
	TRACE     = "TRACE"
	LOG_ERROR = "ERROR"
	LOG_EXCEPTION = "EXCEPTION"
)

// Request represents a JSON-RPC request
type Request struct {
	JsonRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      string      `json:"id,omitempty"`
}

// Response represents a JSON-RPC response
type Response struct {
	JsonRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
	ID      string      `json:"id,omitempty"`
}

// LogParams represents parameters for logging
type LogParams struct {
	Time        *time.Time  `json:"time,omitempty"`
	Event       string      `json:"event"`
	Message     string      `json:"message"`
	Data        interface{} `json:"data,omitempty"`
	Coordinates interface{} `json:"coordinates,omitempty"`
	Duration    *int        `json:"duration,omitempty"`
}

// ReportData represents structured report data with cards and tables
type ReportData struct {
	Card   string                 `json:"card,omitempty"`
	Tables map[string]TableData   `json:"tables,omitempty"`
}

// TableData represents a table with headers and rows
type TableData struct {
	Headers []string      `json:"headers"`
	Rows    [][]interface{} `json:"rows"`
}

// ReportOptions represents options for creating reports
type ReportOptions struct {
	Data  *ReportData `json:"data,omitempty"`
	Image string      `json:"image,omitempty"`
}

// ReportParams represents parameters for reporting (internal use)
type ReportParams struct {
	Message   string                 `json:"message"`
	Data      interface{}            `json:"data,omitempty"`
	Image     string                 `json:"image,omitempty"`
	HasCard   bool                   `json:"has_card"`
	HasTable  bool                   `json:"has_table"`
	HasImage  bool                   `json:"has_image"`
	Task      map[string]interface{} `json:"task"`
}

// TaskParams represents task-related parameters
type TaskParams struct {
	Status string `json:"status"`
}

// GraphQLParams represents GraphQL query parameters
type GraphQLParams struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// Global maps and mutex for concurrency
var (
	rpcCallbacks = make(map[string]chan Response)
	handlers     = make(map[string]func(Request))
	mu           sync.Mutex
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// RegisterHandler registers a handler for a specific method
func RegisterHandler(method string, handler func(Request)) {
	mu.Lock()
	defer mu.Unlock()
	handlers[method] = handler
}

// SendRequest sends a JSON-RPC request and returns a channel for the response
func SendRequest(data map[string]interface{}) chan Response {
	id := generateID()
	data["jsonrpc"] = "2.0"
	data["id"] = id

	// Create a channel for the response and store it
	responseChan := make(chan Response, 1)
	mu.Lock()
	rpcCallbacks[id] = responseChan
	mu.Unlock()

	// Write request to stdout
	sendJSON(data)
	return responseChan
}

// SendNotification sends a JSON-RPC notification (no response expected)
func SendNotification(data map[string]interface{}) {
	data["jsonrpc"] = "2.0"
	sendJSON(data)
}

// OpenPipe starts listening for incoming JSON-RPC messages on stdin
func OpenPipe() {
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size for large JSON responses
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	
	for scanner.Scan() {
		var data map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &data); err != nil {
			log.Printf("Received invalid JSON: %v", err)
			continue
		}
		handleData(data)
	}
	
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading stdin: %v", err)
	}
}

// Log sends a log message with full control over parameters
func Log(event, message string, data interface{}, duration *int, coordinates interface{}, logTime *time.Time) {
	params := LogParams{
		Event:       event,
		Message:     message,
		Data:        data,
		Duration:    duration,
		Coordinates: coordinates,
	}
	
	if logTime != nil {
		params.Time = logTime
	} else {
		now := time.Now()
		params.Time = &now
	}
	
	SendNotification(map[string]interface{}{
		"method": "task.log",
		"params": params,
	})
}

// Info logs an info message
func Info(message string, data interface{}) {
	Log(INFO, message, data, nil, nil, nil)
}

// Error logs an error message
func Error(message string, data interface{}) {
	Log(LOG_ERROR, message, data, nil, nil, nil)
}

// Warn logs a warning message
func Warn(message string, data interface{}) {
	Log(WARN, message, data, nil, nil, nil)
}

// Debug logs a debug message
func Debug(message string, data interface{}) {
	Log(DEBUG, message, data, nil, nil, nil)
}

// Trace logs a trace message
func Trace(message string, data interface{}) {
	Log(TRACE, message, data, nil, nil, nil)
}

// Exception logs an exception message
func Exception(message string, data interface{}) {
	Log(LOG_EXCEPTION, message, data, nil, nil, nil)
}

// Report creates a structured task report following EYWA schema exactly
// Matches the corrected Node.js implementation
func Report(message string, options *ReportOptions) error {
	// Get current task UUID
	taskData, err := GetTask()
	if err != nil {
		return fmt.Errorf("cannot create report: no active task found: %v", err)
	}
	
	// Extract UUID from task data
	var currentTaskUUID string
	if taskMap, ok := taskData.(map[string]interface{}); ok {
		if euuid, exists := taskMap["euuid"]; exists {
			currentTaskUUID = fmt.Sprintf("%v", euuid)
		} else if id, exists := taskMap["id"]; exists {
			currentTaskUUID = fmt.Sprintf("%v", id)
		} else {
			return fmt.Errorf("task UUID not found in task data")
		}
	} else {
		return fmt.Errorf("invalid task data format")
	}
	
	// Build report data structure
	reportData := ReportParams{
		Message: message,
		Task: map[string]interface{}{
			"euuid": currentTaskUUID,
		},
	}
	
	// Process data and set flags
	if options != nil && options.Data != nil {
		reportData.Data = options.Data
		reportData.HasCard = len(options.Data.Card) > 0
		reportData.HasTable = len(options.Data.Tables) > 0
	} else {
		reportData.HasCard = false
		reportData.HasTable = false
	}
	
	// Process image and validate
	if options != nil && len(options.Image) > 0 {
		if !isValidBase64(options.Image) {
			return fmt.Errorf("invalid base64 image data")
		}
		reportData.Image = options.Image
		reportData.HasImage = true
	} else {
		reportData.HasImage = false
	}
	
	// Validate table structure if present
	if options != nil && options.Data != nil && options.Data.Tables != nil {
		if err := validateTables(options.Data.Tables); err != nil {
			return err
		}
	}
	
	// Note: metadata is not supported by EYWA Task Report schema
	// The Task Report entity only supports: message, data, image, has_* flags
	
	// Send report via JSON-RPC
	SendNotification(map[string]interface{}{
		"method": "task.report",
		"params": reportData,
	})
	
	return nil
}

// ReportSimple is a convenience function for simple text reports
func ReportSimple(message string) error {
	return Report(message, nil)
}

// ReportWithCard creates a report with markdown card content
func ReportWithCard(message, card string) error {
	return Report(message, &ReportOptions{
		Data: &ReportData{
			Card: card,
		},
	})
}

// UpdateTask updates the current task status
func UpdateTask(status string) {
	SendNotification(map[string]interface{}{
		"method": "task.update",
		"params": TaskParams{
			Status: status,
		},
	})
}

// GetTask retrieves the current task information
func GetTask() (interface{}, error) {
	responseChan := SendRequest(map[string]interface{}{
		"method": "task.get",
	})
	
	response := <-responseChan
	
	if response.Error != nil {
		return nil, fmt.Errorf("task.get error: %v", response.Error)
	}
	
	return response.Result, nil
}

// ReturnTask returns control to EYWA without closing the task
func ReturnTask() {
	SendNotification(map[string]interface{}{
		"method": "task.return",
	})
	os.Exit(0)
}

// CloseTask closes the current task with a status
func CloseTask(status string) {
	SendNotification(map[string]interface{}{
		"method": "task.close",
		"params": TaskParams{
			Status: status,
		},
	})
	
	if status == SUCCESS {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}

// GraphQL executes a GraphQL query
func GraphQL(query string, variables map[string]interface{}) (map[string]interface{}, error) {
	responseChan := SendRequest(map[string]interface{}{
		"method": "eywa.datasets.graphql",
		"params": GraphQLParams{
			Query:     query,
			Variables: variables,
		},
	})
	
	response := <-responseChan
	
	if response.Error != nil {
		return nil, fmt.Errorf("GraphQL error: %v", response.Error)
	}
	
	// Convert result to map
	if result, ok := response.Result.(map[string]interface{}); ok {
		return result, nil
	}
	
	return nil, fmt.Errorf("unexpected GraphQL response format")
}

// Helper functions (internal)

func generateID() string {
	return fmt.Sprintf("%d", rand.Int63())
}

func handleData(data map[string]interface{}) {
	if _, ok := data["method"].(string); ok {
		handleRequest(data)
	} else if _, ok := data["id"]; ok {
		handleResponse(data)
	} else {
		log.Println("Received invalid JSON-RPC:", data)
	}
}

func handleRequest(data map[string]interface{}) {
	method, _ := data["method"].(string)
	
	request := Request{
		JsonRPC: "2.0",
		Method:  method,
		Params:  data["params"],
	}
	
	if id, ok := data["id"]; ok {
		request.ID = fmt.Sprintf("%v", id)
	}
	
	mu.Lock()
	handler, exists := handlers[method]
	mu.Unlock()
	
	if exists {
		handler(request)
	} else {
		log.Printf("Method %s doesn't have a registered handler", method)
	}
}

func handleResponse(data map[string]interface{}) {
	id := fmt.Sprintf("%v", data["id"])
	
	response := Response{
		JsonRPC: "2.0",
		Result:  data["result"],
		Error:   data["error"],
		ID:      id,
	}

	mu.Lock()
	if callback, exists := rpcCallbacks[id]; exists {
		delete(rpcCallbacks, id)
		mu.Unlock()
		callback <- response
		close(callback)
	} else {
		mu.Unlock()
		log.Printf("RPC callback not registered for request with id = %s", id)
	}
}

func sendJSON(data interface{}) {
	encoded, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to encode JSON: %v", err)
		return
	}
	fmt.Println(string(encoded))
}

// Validation helper functions (following Node.js implementation)

// isValidBase64 validates base64 string (matches Node.js implementation)
func isValidBase64(str string) bool {
	if len(strings.TrimSpace(str)) == 0 {
		return false
	}
	
	// Try to decode the base64 string
	decoded, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return false
	}
	
	// Re-encode and compare (matches Node.js btoa/atob pattern)
	reencoded := base64.StdEncoding.EncodeToString(decoded)
	return reencoded == str
}

// validateTables validates table structure (matches Node.js implementation)
func validateTables(tables map[string]TableData) error {
	if tables == nil {
		return fmt.Errorf("tables must be a map with named table entries")
	}
	
	for tableName, tableData := range tables {
		if tableData.Headers == nil {
			return fmt.Errorf("table '%s' must have a 'headers' array", tableName)
		}
		
		if tableData.Rows == nil {
			return fmt.Errorf("table '%s' must have a 'rows' array", tableName)
		}
		
		// Validate each row has same number of columns as headers
		headerCount := len(tableData.Headers)
		for i, row := range tableData.Rows {
			if row == nil {
				return fmt.Errorf("table '%s' row %d cannot be nil", tableName, i)
			}
			if len(row) != headerCount {
				return fmt.Errorf("table '%s' row %d has %d columns but headers specify %d",
					tableName, i, len(row), headerCount)
			}
		}
	}
	
	return nil
}