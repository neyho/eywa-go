package main

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/neyho/eywa-go"
)

func main() {
	fmt.Println("🧪 Starting Go Task Reporting Demo...")
	
	// Start EYWA communication
	go eywa.OpenPipe()
	time.Sleep(100 * time.Millisecond)
	
	// Get current task info for debugging
	taskData, err := eywa.GetTask()
	if err != nil {
		log.Fatalf("Failed to get task: %v", err)
	}
	
	if taskMap, ok := taskData.(map[string]interface{}); ok {
		if euuid, exists := taskMap["euuid"]; exists {
			fmt.Printf("Running task: %v\n", euuid)
		}
	}

	// Test 1: Simple card report
	fmt.Println("📝 Test 1: Simple card report")
	err = eywa.ReportWithCard("Daily Summary", `
# Success! 🎉
Processed **1,000 records** with 0 errors.

## Key Metrics
- **Processing Time:** 2.3 seconds
- **Success Rate:** 100%
- **Memory Usage:** Optimized
`)
	if err != nil {
		fmt.Printf("❌ Test 1 failed: %v\n", err)
		eywa.Error(fmt.Sprintf("Test 1 failed: %v", err), nil)
		eywa.CloseTask(eywa.ERROR)
		return
	}

	// Test 2: Table report
	fmt.Println("📊 Test 2: Multi-table report")
	err = eywa.Report("Performance Analysis", &eywa.ReportOptions{
		Data: &eywa.ReportData{
			Card: fmt.Sprintf(`
# Go Performance Report

**Runtime:** %s  
**OS:** %s  
**Architecture:** %s  
**CPUs:** %d

## Summary
System performance exceeded targets by **15%%.** 
All tests passing successfully.
`, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU()),
			Tables: map[string]eywa.TableData{
				"System Health": {
					Headers: []string{"Service", "Uptime", "Response Time", "Status"},
					Rows: [][]interface{}{
						{"API Gateway", "99.9%", "85ms", "Healthy"},
						{"Database", "100%", "12ms", "Healthy"},
						{"Cache Layer", "99.8%", "3ms", "Healthy"},
					},
				},
				"Go Runtime": {
					Headers: []string{"Property", "Value"},
					Rows: [][]interface{}{
						{"Go Version", runtime.Version()},
						{"OS", runtime.GOOS},
						{"Architecture", runtime.GOARCH},
						{"CPUs", fmt.Sprintf("%d", runtime.NumCPU())},
					},
				},
			},
		},
	})
	if err != nil {
		fmt.Printf("❌ Test 2 failed: %v\n", err)
		eywa.Error(fmt.Sprintf("Test 2 failed: %v", err), nil)
		eywa.CloseTask(eywa.ERROR)
		return
	}

	// Test 3: Image report
	fmt.Println("🖼️  Test 3: Image report")
	sampleImage := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	err = eywa.Report("Visual Analysis", &eywa.ReportOptions{
		Data: &eywa.ReportData{
			Card: `
# Monthly Trends 📈

Chart shows **significant growth** in Q4 2024.

**Key Insights:**
- Revenue up 23% YoY  
- User engagement improved 15%
- Mobile traffic now 65% of total
`,
		},
		Image: sampleImage,
	})
	if err != nil {
		fmt.Printf("❌ Test 3 failed: %v\n", err)
		eywa.Error(fmt.Sprintf("Test 3 failed: %v", err), nil)
		eywa.CloseTask(eywa.ERROR)
		return
	}

	// Test 4: Simple text report
	fmt.Println("✅ Test 4: Simple report")
	err = eywa.ReportSimple("All Go task reporting tests completed successfully!")
	if err != nil {
		fmt.Printf("❌ Test 4 failed: %v\n", err)
		eywa.Error(fmt.Sprintf("Test 4 failed: %v", err), nil)
		eywa.CloseTask(eywa.ERROR)
		return
	}

	// Success
	fmt.Println("✅ All Go task reports generated successfully!")
	eywa.Info("Go task reporting demo completed successfully", map[string]interface{}{
		"reports_created": 4,
		"timestamp":      time.Now(),
		"go_version":     runtime.Version(),
	})
	
	eywa.CloseTask(eywa.SUCCESS)
	fmt.Println("🎉 Go task reporting demo completed!")
}
