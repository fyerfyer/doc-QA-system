package pyprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// DocumentParseResult 表示文档解析结果
type DocumentParseResult struct {
	DocumentID string         `json:"document_id"`
	Content    string         `json:"content"`
	Title      string         `json:"title"`
	Meta       map[string]any `json:"meta"`
	Pages      int            `json:"pages"`
	Words      int            `json:"words"`
	Chars      int            `json:"chars"`
}

// DocumentParseResponse 表示文档解析API的响应
type DocumentParseResponse struct {
	Success       bool                `json:"success"`
	DocumentID    string              `json:"document_id"`
	TaskID        string              `json:"task_id"`
	Result        DocumentParseResult `json:"result"`
	ProcessTimeMs int                 `json:"process_time_ms"`
}

// DocumentClient 是Python文档解析服务的客户端
type DocumentClient struct {
	client Client
}

// NewDocumentClient 创建一个新的文档解析客户端
func NewDocumentClient(client Client) *DocumentClient {
	return &DocumentClient{
		client: client,
	}
}

// ParseDocument 解析指定路径的文档
func (c *DocumentClient) ParseDocument(ctx context.Context, filePath string) (*DocumentParseResult, error) {
	// 生成唯一文档ID
	documentID := uuid.New().String()

	// 获取原始文件名及扩展名
	originalFilename := filepath.Base(filePath)

	// 构造表单数据
	formData := url.Values{}
	formData.Set("document_id", documentID)
	formData.Set("file_path", filePath)
	formData.Set("store_result", "true")
	formData.Set("original_filename", originalFilename) // 添加此行以发送原始文件名

	// 构建请求URL
	reqPath := "/python/documents/parse"

	// 创建自定义请求（因为需要发送表单数据而不是JSON）
	reqURL := fmt.Sprintf("%s%s", c.client.GetConfig().BaseURL, reqPath)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, errors.Wrap(err, "创建请求失败")
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// 执行请求
	var response DocumentParseResponse
	httpClient := &http.Client{Timeout: c.client.GetConfig().Timeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "发送请求失败")
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API调用失败(状态码: %d): %s", resp.StatusCode, string(body))
	}

	// 解析响应
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, errors.Wrap(err, "解析响应失败")
	}

	// 检查API响应是否成功
	if !response.Success {
		return nil, fmt.Errorf("文档解析失败: %s", response.Result.Content)
	}

	return &response.Result, nil
}

// ParseDocumentWithReader 从io.Reader解析文档
func (c *DocumentClient) ParseDocumentWithReader(ctx context.Context, reader io.Reader, fileName string) (*DocumentParseResult, error) {
	// 生成唯一文档ID
	documentID := uuid.New().String()

	// 创建multipart表单
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// 添加文档ID字段
	if err := writer.WriteField("document_id", documentID); err != nil {
		return nil, errors.Wrap(err, "添加document_id字段失败")
	}

	// 添加store_result字段
	if err := writer.WriteField("store_result", "true"); err != nil {
		return nil, errors.Wrap(err, "添加store_result字段失败")
	}

	// 添加文件内容
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, errors.Wrap(err, "创建文件表单字段失败")
	}

	// 从reader复制数据到请求体
	if _, err := io.Copy(part, reader); err != nil {
		return nil, errors.Wrap(err, "复制文件数据失败")
	}

	// 完成multipart表单
	if err := writer.Close(); err != nil {
		return nil, errors.Wrap(err, "关闭表单写入器失败")
	}

	// 构建请求URL
	reqPath := "/python/documents/parse"

	// 创建自定义请求
	reqURL := fmt.Sprintf("%s%s", c.client.GetConfig().BaseURL, reqPath)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, &requestBody)
	if err != nil {
		return nil, errors.Wrap(err, "创建请求失败")
	}

	// 设置请求头
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 执行请求
	var response DocumentParseResponse
	httpClient := &http.Client{Timeout: c.client.GetConfig().Timeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "发送请求失败")
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API调用失败(状态码: %d): %s", resp.StatusCode, string(body))
	}

	// 解析响应
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, errors.Wrap(err, "解析响应失败")
	}

	// 检查API响应是否成功
	if !response.Success {
		return nil, fmt.Errorf("文档解析失败: %s", response.Result.Content)
	}

	return &response.Result, nil
}

// ParseDocumentAsync 异步解析文档，返回任务ID
func (c *DocumentClient) ParseDocumentAsync(ctx context.Context, filePath string) (string, error) {
	_, err := c.ParseDocument(ctx, filePath)
	if err != nil {
		return "", err
	}
	// 假设API在响应中返回了任务ID
	return "", errors.New("暂未实现异步解析功能")
	// TODO: 实现真正的异步解析，目前先返回错误
}

// GetDocumentContent 获取已解析文档的内容
func (c *DocumentClient) GetDocumentContent(ctx context.Context, documentID string) (*DocumentParseResult, error) {
	// 构建请求路径
	reqPath := fmt.Sprintf("/python/documents/%s", documentID)

	// 发送GET请求
	var response struct {
		Success    bool                `json:"success"`
		DocumentID string              `json:"document_id"`
		Result     DocumentParseResult `json:"result"`
	}

	err := c.client.Get(ctx, reqPath, &response)
	if err != nil {
		return nil, errors.Wrap(err, "获取文档内容失败")
	}

	if !response.Success {
		return nil, fmt.Errorf("获取文档内容失败: 请求成功但API返回失败状态")
	}

	return &response.Result, nil
}
