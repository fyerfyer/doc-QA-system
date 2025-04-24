package document

import (
	"fmt"
	"io/ioutil"
	"os"
)

// PlainTextParser 纯文本解析器
type PlainTextParser struct{}

// NewPlainTextParser 创建一个新的纯文本解析器
func NewPlainTextParser() Parser {
	return &PlainTextParser{}
}

// Parse 解析纯文本文件
func (p *PlainTextParser) Parse(filePath string) (string, error) {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open text file: %v", err)
	}
	defer file.Close()

	// 读取文件内容
	content, err := ioutil.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read text file: %v", err)
	}

	return string(content), nil
}
