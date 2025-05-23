package document

import (
	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

// PDFParser PDF文档解析器
type PDFParser struct{}

// NewPDFParser 创建一个新的PDF解析器
func NewPDFParser() Parser {
	return &PDFParser{}
}

// Parse 解析PDF文件并提取其文本内容
func (p *PDFParser) Parse(filePath string) (string, error) {
	// 直接使用文件路径，因为PDF解析需要文件系统访问
	// 创建临时目录用于存放提取的文本
	tmpDir, err := ioutil.TempDir("", "pdfcpu_extract_")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 使用默认配置
	conf := model.NewDefaultConfiguration()

	// 提取文本到临时目录
	if err := api.ExtractContentFile(filePath, tmpDir, nil, conf); err != nil {
		return "", fmt.Errorf("failed to extract text from PDF: %v", err)
	}

	// 读取所有提取出来的txt文件
	files, err := ioutil.ReadDir(tmpDir)
	if err != nil {
		return "", fmt.Errorf("failed to read extracted text dir: %v", err)
	}

	// 按文件名排序（页码顺序）
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	var allText strings.Builder
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".txt") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tmpDir, f.Name()))
		if err != nil {
			continue
		}
		if allText.Len() > 0 {
			allText.WriteString("\n\n")
		}
		allText.WriteString(string(data))
	}

	result := strings.TrimSpace(allText.String())
	if result == "" {
		return "", fmt.Errorf("no text content found in PDF")
	}
	return result, nil
}

// ParseReader 从Reader解析PDF内容
func (p *PDFParser) ParseReader(r io.Reader, filename string) (string, error) {
	// 创建临时文件
	tempFile, err := ioutil.TempFile("", "pdf-parser-*"+filepath.Ext(filename))
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// 将内容写入临时文件
	_, err = io.Copy(tempFile, r)
	if err != nil {
		return "", fmt.Errorf("failed to write temp file: %v", err)
	}
	tempFile.Close() // 关闭文件以释放句柄

	// 使用Parse方法处理临时文件
	return p.Parse(tempFile.Name())
}
