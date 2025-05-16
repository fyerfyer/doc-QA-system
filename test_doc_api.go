package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"

    "github.com/fyerfyer/doc-QA-system/internal/services"
)

func main() {
    // Test Python API endpoint directly
    pythonServiceURL := "http://localhost:8000"
    processURL := fmt.Sprintf("%s/api/tasks/process", pythonServiceURL)
    
    // Case 1: Using DefaultAsyncOptions for metadata (should work)
    options := services.DefaultAsyncOptions()
    requestBody1 := map[string]interface{}{
        "document_id": "test-doc-id-1",
        "file_path":   "test_file.txt",
        "file_name":   "test_file.txt",
        "file_type":   "txt",
        "chunk_size":  1000,
        "overlap":     200,
        "split_type":  "paragraph",
        "model":       "default",
        "metadata":    options.Metadata, // Use DefaultAsyncOptions metadata
    }
    
    fmt.Println("Test Case 1: With DefaultAsyncOptions metadata")
    testProcessRequest(processURL, requestBody1)
    
    // Case 2: With empty metadata (should work)
    requestBody2 := map[string]interface{}{
        "document_id": "test-doc-id-2",
        "file_path":   "test_file.txt",
        "file_name":   "test_file.txt",
        "file_type":   "txt",
        "chunk_size":  1000,
        "overlap":     200,
        "split_type":  "paragraph",
        "model":       "default",
        "metadata":    map[string]string{}, // Empty map instead of nil
    }
    
    fmt.Println("\nTest Case 2: With empty metadata")
    testProcessRequest(processURL, requestBody2)
}

func testProcessRequest(url string, requestBody map[string]interface{}) {
    jsonData, err := json.Marshal(requestBody)
    if err != nil {
        fmt.Printf("Error marshaling JSON: %v\n", err)
        return
    }
    
    req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    if err != nil {
        fmt.Printf("Error creating request: %v\n", err)
        return
    }
    
    req.Header.Set("Content-Type", "application/json")
    
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        fmt.Printf("Error sending request: %v\n", err)
        return
    }
    defer resp.Body.Close()
    
    respBody, _ := io.ReadAll(resp.Body)
    fmt.Printf("Response Status: %d\nResponse Body: %s\n", resp.StatusCode, string(respBody))
}