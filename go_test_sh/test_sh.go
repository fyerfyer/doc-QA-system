package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func main() {
	pythonServiceURL := "http://py-api:8000"

	// 准备测试请求体
	requestBody := map[string]interface{}{
		"document_id": "test-doc-id",
		"file_path":   "/app/data/test.md",
		"file_name":   "test.md",
		"file_type":   "md",
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		fmt.Printf("Error marshaling request: %v\n", err)
		return
	}

	// 发送HTTP请求
	req, err := http.NewRequest("POST", pythonServiceURL+"/api/tasks/process", bytes.NewBuffer(jsonBody))
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

	// 输出响应
	var respData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		fmt.Printf("Error decoding response: %v\n", err)
		return
	}

	fmt.Printf("Response: %+v\n", respData)
}
