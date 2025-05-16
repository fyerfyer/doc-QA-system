package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "os"
    "path/filepath"
    "time"
)

func main() {
    // 1. Test Go API health endpoint
    fmt.Println("\n=== Testing Go API Health ===")
    resp, err := http.Get("http://localhost:8080/api/health")
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    fmt.Printf("Status: %d\nResponse: %s\n", resp.StatusCode, string(body))

    // 2. Test document upload with proper multipart form
    fmt.Println("\n=== Testing Document Upload ===")
    
    // Create a temporary file for testing
    tempFilePath := createTempFile("test.txt", "This is a test document content.")
    defer os.Remove(tempFilePath)
    
    // Upload the file using multipart form
    fileID, err := uploadFile(tempFilePath)
    if err != nil {
        fmt.Printf("Error uploading file: %v\n", err)
        return
    }
    fmt.Printf("Document ID: %s\n", fileID)

    // Wait for processing
    time.Sleep(2 * time.Second)
    
    fmt.Println("\n=== Checking Document Status ===")
    statusURL := fmt.Sprintf("http://localhost:8080/api/documents/%s/status", fileID)
    statusResp, err := http.Get(statusURL)
    if err != nil {
        fmt.Printf("Error checking status: %v\n", err)
        return
    }
    defer statusResp.Body.Close()
    
    statusBody, _ := io.ReadAll(statusResp.Body)
    fmt.Printf("Status: %d\nResponse: %s\n", statusResp.StatusCode, string(statusBody))
}

// Create temporary test file
func createTempFile(filename, content string) string {
    tempDir, err := os.MkdirTemp("", "docqa-test-")
    if err != nil {
        panic(fmt.Sprintf("Failed to create temp dir: %v", err))
    }
    
    filePath := filepath.Join(tempDir, filename)
    if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
        panic(fmt.Sprintf("Failed to write temp file: %v", err))
    }
    
    return filePath
}

// Upload file using multipart form
func uploadFile(filePath string) (string, error) {
    // Create multipart form body
    var requestBody bytes.Buffer
    multipartWriter := multipart.NewWriter(&requestBody)
    
    // Add file field
    file, err := os.Open(filePath)
    if err != nil {
        return "", fmt.Errorf("error opening file: %v", err)
    }
    defer file.Close()
    
    fileWriter, err := multipartWriter.CreateFormFile("file", filepath.Base(filePath))
    if err != nil {
        return "", fmt.Errorf("error creating form file: %v", err)
    }
    
    if _, err = io.Copy(fileWriter, file); err != nil {
        return "", fmt.Errorf("error copying file content: %v", err)
    }
    
    // Add tags field
    if err = multipartWriter.WriteField("tags", "test,upload"); err != nil {
        return "", fmt.Errorf("error adding tags field: %v", err)
    }
    
    // Close the multipart writer to set the boundary
    if err = multipartWriter.Close(); err != nil {
        return "", fmt.Errorf("error closing multipart writer: %v", err)
    }
    
    // Create and send request
    req, err := http.NewRequest("POST", "http://localhost:8080/api/documents", &requestBody)
    if err != nil {
        return "", fmt.Errorf("error creating request: %v", err)
    }
    
    req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
    
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("error sending request: %v", err)
    }
    defer resp.Body.Close()
    
    respBody, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error reading response: %v", err)
    }
    
    fmt.Printf("Status: %d\nResponse: %s\n", resp.StatusCode, string(respBody))
    
    // Extract document ID from response
    if resp.StatusCode == http.StatusOK {
        var result map[string]interface{}
        if err := json.Unmarshal(respBody, &result); err != nil {
            return "", fmt.Errorf("error parsing JSON response: %v", err)
        }
        
        if data, ok := result["data"].(map[string]interface{}); ok {
            if fileID, ok := data["file_id"].(string); ok {
                return fileID, nil
            }
        }
        return "", fmt.Errorf("could not extract document ID from response")
    }
    
    return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(respBody))
}