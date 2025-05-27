import os
import tempfile
import pytest
from unittest.mock import MagicMock, patch

import numpy as np
from llama_index.core import Document as LlamaDocument
from llama_index.core.schema import TextNode
from llama_index.core.node_parser import SentenceSplitter

# 导入要测试的模块
from app.document_processing.parser import DocumentParser
from app.document_processing.chunker import DocumentChunker, ChunkOptions
from app.document_processing.factory import (
    create_parser, create_chunker, process_file, 
    detect_content_type, get_file_from_minio
)
from app.document_processing.adapters import (
    DocumentParserAdapter, TextChunkerAdapter
)
from app.document_processing.utils import (
    clean_text, extract_title_from_content, format_chunk_for_embedding, 
    merge_metadata
)
from app.utils.utils import logger

# 创建测试数据目录
TEST_DATA_DIR = os.path.join(os.path.dirname(__file__), "testdata")
os.makedirs(TEST_DATA_DIR, exist_ok=True)

def create_test_pdf():
    """创建测试PDF文件"""
    pdf_path = os.path.join(TEST_DATA_DIR, "sample.pdf")
    if not os.path.exists(pdf_path):
        # 这里我们只提供文本内容，用户需要创建实际的PDF
        pdf_content = """
PDF测试文件

这是一个用于测试的PDF文档。
它包含多个段落和一些格式。

第二段内容：
- 项目1
- 项目2
- 项目3

结束。
        """
        # 写入一个文本文件，提醒用户创建PDF
        with open(pdf_path + ".txt", "w", encoding="utf-8") as f:
            f.write(pdf_content)
        print(f"Please create a PDF file at {pdf_path} using the content provided in {pdf_path}.txt")
    return pdf_path


class TestDocumentParser:
    """测试文档解析器"""

    @pytest.fixture
    def setup_test_files(self):
        """创建测试文件"""
        with tempfile.TemporaryDirectory() as temp_dir:
            # 创建测试文本文件
            text_file = os.path.join(temp_dir, "test.txt")
            with open(text_file, "w", encoding="utf-8") as f:
                f.write("这是一个测试文档。\n它有多行内容。\n这是第三行。")

            # 创建测试Markdown文件
            md_file = os.path.join(temp_dir, "test.md")
            with open(md_file, "w", encoding="utf-8") as f:
                f.write("# 测试Markdown\n\n这是一个段落。\n\n- 列表项1\n- 列表项2")

            # 创建测试HTML文件
            html_file = os.path.join(temp_dir, "test.html")
            with open(html_file, "w", encoding="utf-8") as f:
                f.write("<html><head><title>测试HTML</title></head><body><h1>测试标题</h1><p>测试段落</p></body></html>")

            # 创建模拟PDF文件（不是真正的PDF）
            pdf_file = os.path.join(temp_dir, "test.pdf")
            with open(pdf_file, "wb") as f:
                f.write("%PDF-1.4\n模拟PDF内容".encode('utf-8'))

            yield {
                "text_file": text_file,
                "md_file": md_file,
                "html_file": html_file,
                "pdf_file": pdf_file,
                "temp_dir": temp_dir
            }

    @pytest.fixture
    def mock_reader(self):
        """创建模拟LlamaIndex阅读器"""
        mock_reader = MagicMock()
        mock_doc = MagicMock()
        mock_doc.get_content.return_value = "模拟文档内容"
        mock_doc.metadata = {"title": "测试文档", "page_count": 1}
        mock_reader.load_data.return_value = [mock_doc]
        return mock_reader

    def test_document_parser_initialization(self):
        """测试DocumentParser初始化"""
        parser = DocumentParser("test.txt", "text/plain", ".txt")
        assert parser.file_path == "test.txt", "File path should be set correctly"
        assert parser.mime_type == "text/plain", "MIME type should be set correctly"
        assert parser.file_extension == ".txt", "File extension should be set correctly"
        assert parser.content == "", "Content should be empty initially"
        assert parser.metadata == {}, "Metadata should be empty initially"

    def test_detect_mime_type(self):
        """测试MIME类型检测"""
        parser = DocumentParser()
        
        # 测试常见文件类型
        assert "text/plain" in parser._detect_mime_type("test.txt"), "Should detect text file"
        assert "application/pdf" in parser._detect_mime_type("test.pdf"), "Should detect PDF file"
        assert "text/markdown" in parser._detect_mime_type("test.md"), "Should detect Markdown file"
        assert "application/vnd.openxmlformats" in parser._detect_mime_type("test.docx"), "Should detect DOCX file"
        
        # 测试未知扩展名
        unknown_mime = parser._detect_mime_type("test.xyz")
        assert "application/octet-stream" in unknown_mime, "Should use generic MIME type for unknown extension"

    def test_parse_with_mock_reader(self, setup_test_files):
        """测试使用模拟阅读器解析文档"""
        with patch('app.document_processing.parser.DocumentParser._get_reader') as mock_get_reader:
            # 设置模拟阅读器
            mock_reader = MagicMock()
            mock_doc = MagicMock()
            mock_doc.get_content.return_value = "测试文档内容"
            mock_doc.metadata = {"title": "测试标题", "author": "测试作者"}
            mock_reader.load_data.return_value = [mock_doc]
            mock_get_reader.return_value = mock_reader
            
            # 解析文档
            parser = DocumentParser(file_path=setup_test_files["text_file"])
            content = parser.parse()
            
            # 验证结果
            assert content == "测试文档内容", "Content should match mock document content"
            assert parser.metadata["title"] == "测试标题", "Title metadata should be extracted"
            assert parser.metadata["author"] == "测试作者", "Author metadata should be extracted"
            assert "filename" in parser.metadata, "Filename should be added to metadata"
            assert "file_size" in parser.metadata, "File size should be added to metadata"

    def test_parse_text_file(self, setup_test_files):
        """测试解析实际文本文件"""
        with patch('app.document_processing.parser.FlatReader') as MockFlatReader:
            # 设置模拟阅读器返回我们测试文件的内容
            mock_reader = MagicMock()
            mock_doc = MagicMock(spec=LlamaDocument)
            mock_doc.get_content.return_value = "这是一个测试文档。\n它有多行内容。\n这是第三行。"
            mock_doc.metadata = {}
            mock_reader.load_data.return_value = [mock_doc]
            MockFlatReader.return_value = mock_reader
            
            # 解析文件
            parser = DocumentParser(setup_test_files["text_file"])
            content = parser.parse()
            
            # 验证内容
            assert "测试文档" in content, "Parsed content should contain expected text"
            assert "多行内容" in content, "Parsed content should contain expected text"
            assert isinstance(parser.metadata, dict), "Metadata should be a dictionary"
            
            # 验证阅读器使用了正确的路径
            mock_reader.load_data.assert_called_once()

    def test_extract_title(self):
        """测试标题提取"""
        parser = DocumentParser()
        
        # 测试从内容提取
        parser.content = "# 文档标题\n\n这是内容。"
        parser.metadata = {}
        title = parser.extract_title()
        assert title == "# 文档标题", "Should extract title from first line"
        
        # 测试从元数据提取
        parser.metadata = {"title": "元数据标题"}
        title = parser.extract_title()
        assert title == "元数据标题", "Should prioritize title from metadata"
        
        # 测试使用文件名作为回退
        parser.content = ""
        parser.metadata = {}
        title = parser.extract_title(filename="test_doc.pdf")
        assert title == "test_doc", "Should fall back to filename without extension"
        
        # 测试默认回退值
        title = parser.extract_title()
        assert title == "Untitled Document", "Should use default title if nothing else is available"

    def test_parse_content_direct(self):
        """测试直接解析文本内容"""
        parser = DocumentParser()
        content = "这是一些要直接解析的内容。"
        result = parser.parse_content(content, "test.txt")
        
        # 验证结果
        assert result == content, "Direct parsing should return original content"
        assert parser.content == content, "Parsed content should be stored"
        assert parser.metadata["filename"] == "test.txt", "Filename should be added to metadata"
        assert parser.metadata["extension"] == ".txt", "Extension should be added to metadata"
        assert "words" in parser.metadata, "Word count should be added to metadata"
        assert "chars" in parser.metadata, "Character count should be added to metadata"

    def test_parse_error_handling(self, setup_test_files):
        """测试解析错误处理"""
        with patch('app.document_processing.parser.DocumentParser._get_reader') as mock_get_reader:
            # 设置模拟阅读器抛出异常
            mock_reader = MagicMock()
            mock_reader.load_data.side_effect = Exception("模拟解析错误")
            mock_get_reader.return_value = mock_reader
            
            # 尝试解析
            parser = DocumentParser(file_path=setup_test_files["text_file"])
            
            # 应该抛出异常
            with pytest.raises(Exception) as excinfo:
                parser.parse()
            
            # 验证异常消息
            assert "模拟解析错误" in str(excinfo.value), "Should propagate raised exception"


class TestDocumentChunker:
    """测试文档分块器"""

    @pytest.fixture
    def sample_text(self):
        """测试分块的示例文本"""
        return """
        这是用于测试文档分块器的示例文档。
        它包含多个段落，应该被分割成不同的块。
        
        这是第二个段落，内容不同。
        分块器应该将其与第一个段落分开。
        
        这是第三个段落，包含更多文本。
        这也应该是一个独立的块。
        """

    @pytest.fixture
    def mock_splitter(self):
        """创建模拟LlamaIndex分块器"""
        mock_splitter = MagicMock()
        # 创建模拟节点
        mock_node1 = MagicMock()
        mock_node1.text = "块1内容"
        mock_node1.metadata = {"source": "test"}
        
        mock_node2 = MagicMock()
        mock_node2.text = "块2内容"
        mock_node2.metadata = {"source": "test"}
        
        mock_splitter.get_nodes_from_documents.return_value = [mock_node1, mock_node2]
        return mock_splitter

    def test_chunker_initialization(self):
        """测试DocumentChunker初始化"""
        # 测试默认选项
        chunker = DocumentChunker()
        assert chunker.options.chunk_size == 1000, "Default chunk size should be 1000"
        assert chunker.options.chunk_overlap == 200, "Default chunk overlap should be 200"
        assert chunker.options.split_type == "paragraph", "Default split type should be paragraph"
        
        # 测试自定义选项
        options = ChunkOptions(chunk_size=500, chunk_overlap=50, split_type="sentence")
        chunker = DocumentChunker(options)
        assert chunker.options.chunk_size == 500, "Custom chunk size should be set"
        assert chunker.options.chunk_overlap == 50, "Custom chunk overlap should be set"
        assert chunker.options.split_type == "sentence", "Custom split type should be set"

    def test_chunk_text_with_mock_splitter(self, sample_text):
        """测试使用模拟分块器分割文本"""
        with patch('app.document_processing.chunker.DocumentChunker._get_splitter') as mock_get_splitter:
            # 设置模拟分块器
            mock_splitter = MagicMock()
            # 创建模拟节点
            mock_node1 = MagicMock()
            mock_node1.text = "块1内容"
            mock_node1.metadata = {"source": "test"}
            
            mock_node2 = MagicMock()
            mock_node2.text = "块2内容"
            mock_node2.metadata = {"source": "test"}
            
            mock_splitter.get_nodes_from_documents.return_value = [mock_node1, mock_node2]
            mock_get_splitter.return_value = mock_splitter
            
            # 执行分块
            chunker = DocumentChunker()
            chunks = chunker.chunk_text(sample_text, {"doc_id": "test123"})
            
            # 验证结果
            assert len(chunks) == 2, "Should return 2 chunks"
            assert chunks[0]["text"] == "块1内容", "First chunk should contain expected content"
            assert chunks[0]["index"] == 0, "First chunk should have index 0"
            assert "doc_id" in chunks[0]["metadata"], "Metadata should be included in chunks"
            assert chunks[0]["metadata"]["doc_id"] == "test123", "Document ID should be in metadata"
            assert chunks[1]["text"] == "块2内容", "Second chunk should contain expected content"
            assert chunks[1]["index"] == 1, "Second chunk should have index 1"

        def test_semantic_splitter_with_embedder_error(self, sample_text):
            """测试当嵌入模型不可用时语义分块器的回退处理"""
            chunker = DocumentChunker(ChunkOptions(split_type="semantic"))
        
            # 正确的 patch 位置是在 _get_splitter 方法内部的导入语句
            with patch('app.document_processing.chunker.get_default_embedder', side_effect=ImportError("模拟嵌入器导入错误")):
                # 直接使用真实的 SentenceSplitter 来检验
                chunks = chunker.chunk_text(sample_text)
                # 验证使用了段落分割器的结果
                assert len(chunks) > 0
                assert "paragraph_separator" in str(chunker.options)
    
    def test_estimate_chunks(self, sample_text):
        """测试估算文本块数"""
        chunker = DocumentChunker(ChunkOptions(chunk_size=100, chunk_overlap=20))
        estimate = chunker.estimate_chunks(sample_text)
        
        # 验证估算结果
        assert "estimated_chunks" in estimate, "Should include estimated chunk count"
        assert estimate["estimated_chunks"] > 0, "Estimated chunks should be positive"
        assert estimate["chars"] == len(sample_text), "Character count should match text length"
        assert "words" in estimate, "Should include word count"
        assert estimate["chunk_size"] == 100, "Should include chunk size"
        assert estimate["chunk_overlap"] == 20, "Should include chunk overlap"
        
        # 测试空文本
        empty_estimate = chunker.estimate_chunks("")
        assert empty_estimate["estimated_chunks"] == 0, "Empty text should estimate 0 chunks"
        assert empty_estimate["chars"] == 0, "Empty text should have 0 characters"

    def test_chunker_with_real_text(self):
        """测试使用真实文本（集成测试）"""
        text = """这是一个测试分块的长文本。
        它应该被分成多个块，因为它有足够的内容。

        这是第二段，应该成为单独的块。
        我们需要确保分块器能够正确识别段落边界。

        第三段内容也应该独立。
        这样我们才能确保分块器工作正常。"""
        
        # 使用真实分块器
        options = ChunkOptions(chunk_size=50, chunk_overlap=10, split_type="paragraph")
        chunker = DocumentChunker(options)
        
        # 替换真实的分块器调用
        with patch.object(SentenceSplitter, 'get_nodes_from_documents') as mock_split:
            # 模拟节点结果
            node1 = TextNode(text="这是一个测试分块的长文本。它应该被分成多个块，因为它有足够的内容。")
            node2 = TextNode(text="这是第二段，应该成为单独的块。我们需要确保分块器能够正确识别段落边界。")
            node3 = TextNode(text="第三段内容也应该独立。这样我们才能确保分块器工作正常。")
            mock_split.return_value = [node1, node2, node3]
            
            # 执行分块
            chunks = chunker.chunk_text(text)
            
            # 验证结果
            assert len(chunks) == 3, "Should create 3 chunks from 3 paragraphs"
            for i, chunk in enumerate(chunks):
                assert "text" in chunk, "Each chunk should contain text"
                assert chunk["index"] == i, "Chunk index should match position"
                assert "metadata" in chunk, "Each chunk should have metadata"


class TestFactoryFunctions:
    """测试工厂函数"""

    @pytest.fixture
    def setup_test_files(self):
        """创建测试文件"""
        with tempfile.TemporaryDirectory() as temp_dir:
            # 创建测试文本文件
            text_file = os.path.join(temp_dir, "test.txt")
            with open(text_file, "w", encoding="utf-8") as f:
                f.write("这是测试文档。")

            # 创建模拟PDF文件
            pdf_file = os.path.join(temp_dir, "test.pdf")
            with open(pdf_file, "wb") as f:
                f.write("%PDF-1.4\n模拟PDF内容".encode('utf-8'))

            yield {
                "text_file": text_file,
                "pdf_file": pdf_file,
                "temp_dir": temp_dir
            }

    def test_detect_content_type(self, setup_test_files):
        """测试内容类型检测"""
        # 测试已知类型
        assert detect_content_type(setup_test_files["text_file"]) == "text/plain", "Should detect text/plain for .txt"
        assert detect_content_type(setup_test_files["pdf_file"]) == "application/pdf", "Should detect application/pdf for .pdf"
        
        # 测试mimetype模块失败时通过扩展名检测
        with patch('mimetypes.guess_type', return_value=(None, None)):
            assert "application/pdf" in detect_content_type(setup_test_files["pdf_file"]), "Should detect PDF by extension"

    def test_create_parser(self, setup_test_files):
        """测试创建解析器工厂函数"""
        # 测试文本文件
        parser = create_parser(setup_test_files["text_file"])
        assert isinstance(parser, DocumentParser), "Should return DocumentParser instance"
        assert parser.file_path == setup_test_files["text_file"], "Parser should have correct file path"
        assert "text/plain" in parser.mime_type, "Parser should have correct MIME type"
        
        # 测试指定MIME类型
        parser = create_parser(setup_test_files["text_file"], "application/custom")
        assert parser.mime_type == "application/custom", "Parser should use provided MIME type"

    def test_create_chunker(self):
        """测试创建分块器工厂函数"""
        chunker = create_chunker()
        assert isinstance(chunker, DocumentChunker), "Should return DocumentChunker instance"
        assert chunker.options.chunk_size == 1000, "Chunker should use default chunk size"
        assert chunker.options.chunk_overlap == 200, "Chunker should use default chunk overlap"
        assert chunker.options.split_type == "paragraph", "Chunker should use default split type"
        
        # 测试自定义选项
        chunker = create_chunker(500, 50, "sentence")
        assert chunker.options.chunk_size == 500, "Chunker should use custom chunk size"
        assert chunker.options.chunk_overlap == 50, "Chunker should use custom chunk overlap"
        assert chunker.options.split_type == "sentence", "Chunker should use custom split type"

    def test_get_file_from_minio(self):
        """测试从MinIO获取文件"""
        # 测试MinIO客户端可用时
        with patch('app.document_processing.factory.minio_client') as mock_minio:
            mock_minio.download_file.return_value = True
            
            # 使用临时文件模拟下载
            with patch('tempfile.NamedTemporaryFile') as mock_temp:
                mock_temp.return_value.__enter__.return_value.name = "/tmp/test.pdf"
                path, is_temp = get_file_from_minio("test/file.pdf")
                assert is_temp is True, "Should indicate temp file was created"
            
            # MinIO下载失败
            mock_minio.download_file.return_value = False
            path, is_temp = get_file_from_minio("test/file.pdf")
            assert path == "test/file.pdf", "Should return original path on failure"
            assert is_temp is False, "Should indicate no temp file was created"
        
        # MinIO客户端不可用时
        with patch('app.document_processing.factory.minio_client', None):
            path, is_temp = get_file_from_minio("test/file.pdf")
            assert path == "test/file.pdf", "Should return original path when no client"
            assert is_temp is False, "Should indicate no temp file was created"

    def test_process_file(self, setup_test_files):
        """测试完整的process_file函数"""
        # 模拟解析器和分块器
        with patch('app.document_processing.factory.create_parser') as mock_create_parser:
            with patch('app.document_processing.factory.create_chunker') as mock_create_chunker:
                # 设置模拟解析器
                mock_parser = MagicMock()
                mock_parser.parse.return_value = "测试文档内容"
                mock_parser.get_metadata.return_value = {"title": "测试"}
                mock_create_parser.return_value = mock_parser
                
                # 设置模拟分块器
                mock_chunker = MagicMock()
                mock_chunker.chunk_text.return_value = [
                    {"text": "块1", "index": 0},
                    {"text": "块2", "index": 1}
                ]
                mock_create_chunker.return_value = mock_chunker
                
                # 测试process_file
                content, chunks, metadata = process_file(
                    setup_test_files["text_file"], 
                    chunk_size=500, 
                    chunk_overlap=50, 
                    split_type="sentence",
                    metadata={"custom": "metadata"}
                )
                
                # 验证结果
                assert content == "测试文档内容", "Should return parsed content"
                assert len(chunks) == 2, "Should return chunked content"
                assert chunks[0]["text"] == "块1", "First chunk should have correct content"
                assert chunks[1]["text"] == "块2", "Second chunk should have correct content"
                assert metadata["title"] == "测试", "Should include parser metadata"
                assert metadata["custom"] == "metadata", "Should include custom metadata"
                
                # 验证调用参数
                mock_create_chunker.assert_called_once_with(500, 50, "sentence")
                mock_chunker.chunk_text.assert_called_once()


class TestAdapterClasses:
    """测试适配器类"""

    @pytest.fixture
    def setup_test_files(self):
        """创建测试文件"""
        with tempfile.TemporaryDirectory() as temp_dir:
            # 创建测试文本文件
            text_file = os.path.join(temp_dir, "test.txt")
            with open(text_file, "w", encoding="utf-8") as f:
                f.write("这是测试文档。")

            yield {
                "text_file": text_file,
                "temp_dir": temp_dir
            }

    def test_document_parser_adapter(self, setup_test_files):
        """测试文档解析适配器向后兼容性"""
        # 模拟LlamaIndex读取器
        with patch('llama_index.readers.file.FlatReader') as MockFlatReader:
            # 设置模拟阅读器
            mock_reader = MagicMock()
            mock_doc = MagicMock()
            mock_doc.get_content.return_value = "适配器测试内容"
            mock_doc.metadata = {"title": "测试", "filename": "适配器测试.txt"}
            mock_reader.load_data.return_value = [mock_doc]
            MockFlatReader.return_value = mock_reader
            
            # 测试适配器
            adapter = DocumentParserAdapter(setup_test_files["text_file"])
            content = adapter.parse()
            
            # 验证内容和元数据
            assert content == "适配器测试内容", "Adapter should return content from reader"
            assert adapter.metadata["title"] == "测试", "Adapter should extract metadata"
            
            # 测试get_metadata
            metadata = adapter.get_metadata()
            assert metadata["title"] == "测试", "get_metadata should return parsed metadata"
            assert "filename" in metadata, "get_metadata should include filename"
            
            # 测试extract_title使用元数据标题
            title = adapter.extract_title("一些内容", "test.txt")
            assert title == "测试", "Should use metadata title"
            
            # 测试无元数据标题时使用内容首行
            adapter.metadata = {}
            title = adapter.extract_title("第一行\n第二行", "test.txt")
            assert title == "第一行", "Should use first line when no metadata title"
            
            # 测试文件名回退
            title = adapter.extract_title("", "test.txt")
            assert title == "test", "Should fall back to filename without extension"

    def test_text_chunker_adapter(self):
        """测试文本分块适配器向后兼容性"""
        # 模拟LlamaIndex分块器
        with patch('app.document_processing.adapters.SentenceSplitter') as MockSplitter:
            # 设置模拟分块器
            mock_splitter = MagicMock()
            # 创建模拟节点
            mock_node1 = MagicMock()
            mock_node1.text = "适配器块1"
            mock_node1.metadata = {"source": "test"}
            
            mock_node2 = MagicMock()
            mock_node2.text = "适配器块2"
            mock_node2.metadata = {"source": "test"}
            
            mock_splitter.get_nodes_from_documents.return_value = [mock_node1, mock_node2]
            MockSplitter.return_value = mock_splitter
            
            # 测试适配器
            adapter = TextChunkerAdapter()
            chunks = adapter.split_text(
                "测试内容", 
                chunk_size=500, 
                chunk_overlap=50, 
                split_type="paragraph",
                metadata={"doc_id": "test123"}
            )
            
            # 验证结果
            assert len(chunks) == 2, "Should return 2 chunks"
            assert chunks[0]["text"] == "适配器块1", "First chunk should have correct content"
            assert chunks[0]["index"] == 0, "First chunk should have correct index"
            assert chunks[0]["metadata"]["source"] == "test", "Chunk metadata should be preserved"
            assert chunks[1]["text"] == "适配器块2", "Second chunk should have correct content"


class TestUtilityFunctions:
    """测试工具函数"""

    def test_clean_text(self):
        """测试文本清理函数"""
        # 测试多余空行和空格的清理
        dirty_text = "这是一行文本。  \n\n\n\n这是第二行。   \n   第三行。"
        clean = clean_text(dirty_text)
        assert clean == "这是一行文本。\n\n这是第二行。\n第三行。", "Should clean excessive whitespace"
        
        # 测试控制字符清理
        control_text = "这是文本\x01\x02带有控制字符\x1F。"
        clean = clean_text(control_text)
        assert "\x01" not in clean, "Should remove control characters"
        assert "这是文本带有控制字符。" == clean, "Should clean control characters while preserving content"
        
        # 测试空输入
        assert clean_text("") == "", "Should handle empty input"
        assert clean_text(None) == "", "Should handle None input"

    def test_extract_title_from_content(self):
        """测试从内容提取标题"""
        # 测试从内容首行提取
        content = "这是标题\n这是内容第一段。\n这是内容第二段。"
        title = extract_title_from_content(content)
        assert title == "这是标题", "Should extract first line as title"
        
        # 测试使用文件名作为回退
        title = extract_title_from_content("", "document.pdf")
        assert title == "document", "Should use filename without extension"
        
        # 测试默认标题
        title = extract_title_from_content("")
        assert title == "Untitled Document", "Should use default title for empty content"

    def test_format_chunk_for_embedding(self):
        """测试格式化分块用于嵌入"""
        # 测试基本格式化
        chunk = {"text": "这是块内容", "index": 1}
        formatted = format_chunk_for_embedding(chunk)
        assert formatted["text"] == "这是块内容", "Text should be preserved"
        assert formatted["index"] == 1, "Index should be preserved"
        assert "metadata" in formatted, "Should add metadata field"
        assert "chars" in formatted["metadata"], "Should add character count"
        assert "words" in formatted["metadata"], "Should add word count"
        
        # 测试缺少字段的处理
        incomplete_chunk = {"text": "只有文本"}
        formatted = format_chunk_for_embedding(incomplete_chunk)
        assert formatted["index"] == -1, "Should add default index for missing index"
        assert "metadata" in formatted, "Should add metadata field"
        
        # 测试没有文本的处理
        no_text_chunk = {"index": 2}
        formatted = format_chunk_for_embedding(no_text_chunk)
        assert formatted["text"] == "", "Should add empty string for missing text"

    def test_merge_metadata(self):
        """测试合并多个元数据字典"""
        # 测试基本合并
        meta1 = {"title": "标题", "author": "作者"}
        meta2 = {"pages": 10, "language": "zh"}
        merged = merge_metadata([meta1, meta2])
        assert merged["title"] == "标题", "Should include first dict values"
        assert merged["pages"] == 10, "Should include second dict values"
        assert len(merged) == 4, "Should include all keys"
        
        # 测试冲突处理
        meta1 = {"title": "标题1", "pages": 5}
        meta2 = {"title": "标题2", "pages": 10}
        merged = merge_metadata([meta1, meta2])
        assert merged["title"] == "标题1", "Should keep first value for conflicts"
        assert merged["pages"] == 5, "Should keep first value for conflicts"
        
        # 测试字符串包含关系
        meta1 = {"title": "标题"}
        meta2 = {"title": "完整标题"}
        merged = merge_metadata([meta1, meta2])
        assert merged["title"] == "完整标题" or isinstance(merged["title"], list), "Should use longer string or make list"
        
        # 测试列表合并
        meta1 = {"tags": ["标签1", "标签2"]}
        meta2 = {"tags": ["标签2", "标签3"]}
        merged = merge_metadata([meta1, meta2])
        assert set(merged["tags"]) == {"标签1", "标签2", "标签3"}, "Should merge lists with unique values"


class TestIntegration:
    """集成测试"""

    def test_parse_and_chunk_with_factory(self):
        """测试使用工厂函数的解析和分块流程"""
        # 创建测试文件
        with tempfile.TemporaryDirectory() as temp_dir:
            test_file = os.path.join(temp_dir, "integration_test.txt")
            with open(test_file, "w", encoding="utf-8") as f:
                f.write("这是一个集成测试文档。\n\n它包含多个段落。\n\n这是第三段内容。")
                
            # 模拟process_file函数
            with patch('app.document_processing.factory.create_parser') as mock_create_parser:
                with patch('app.document_processing.factory.create_chunker') as mock_create_chunker:
                    # 设置模拟解析器
                    mock_parser = MagicMock()
                    mock_parser.parse.return_value = "这是一个集成测试文档。\n\n它包含多个段落。\n\n这是第三段内容。"
                    mock_parser.get_metadata.return_value = {"title": "集成测试"}
                    mock_create_parser.return_value = mock_parser
                    
                    # 设置模拟分块器
                    mock_chunker = MagicMock()
                    mock_chunker.chunk_text.return_value = [
                        {"text": "这是一个集成测试文档。", "index": 0, "metadata": {}},
                        {"text": "它包含多个段落。", "index": 1, "metadata": {}},
                        {"text": "这是第三段内容。", "index": 2, "metadata": {}}
                    ]
                    mock_create_chunker.return_value = mock_chunker
                    
                    # 执行集成流程
                    content, chunks, metadata = process_file(
                        test_file, 
                        chunk_size=200, 
                        chunk_overlap=20, 
                        split_type="paragraph",
                        metadata={"source": "integration_test"}
                    )
                    
                    # 验证结果
                    assert len(chunks) == 3, "Should generate 3 chunks"
                    assert metadata["title"] == "集成测试", "Should preserve metadata from parser"
                    assert metadata["source"] == "integration_test", "Should include custom metadata"
                    assert mock_create_parser.called, "Should call create_parser"
                    assert mock_create_chunker.called, "Should call create_chunker"


@pytest.mark.skipif(not os.path.exists(os.path.join(TEST_DATA_DIR, "sample.pdf")), 
                     reason="Sample PDF not created")
class TestPDFProcessing:
    """PDF处理测试(需要sample.pdf存在)"""
    
    def test_pdf_parsing(self):
        """测试PDF解析"""
        pdf_path = os.path.join(TEST_DATA_DIR, "sample.pdf")
        if not os.path.exists(pdf_path):
            create_test_pdf()
            pytest.skip("PDF file not created, skipping test")
        
        # 模拟PDF阅读器
        with patch('app.document_processing.parser.PyMuPDFReader') as MockPDFReader:
            # 设置模拟阅读器
            mock_reader = MagicMock()
            mock_doc = MagicMock()
            mock_doc.get_content.return_value = "PDF测试文件\n\n这是一个用于测试的PDF文档。\n它包含多个段落和一些格式。"
            mock_doc.metadata = {"title": "PDF测试", "author": "测试作者", "page_count": 1}
            mock_reader.load_data.return_value = [mock_doc]
            MockPDFReader.return_value = mock_reader
            
            # 解析PDF
            parser = DocumentParser(pdf_path)
            content = parser.parse()
            
            # 验证结果
            assert "PDF测试文件" in content, "Should extract PDF content"
            assert parser.metadata["title"] == "PDF测试", "Should extract PDF title"
            assert parser.metadata["author"] == "测试作者", "Should extract PDF author"
            assert parser.metadata["page_count"] == 1, "Should extract PDF page count"
            
    def test_pdf_chunking(self):
        """测试PDF内容分块"""
        pdf_path = os.path.join(TEST_DATA_DIR, "sample.pdf")
        if not os.path.exists(pdf_path):
            create_test_pdf()
            pytest.skip("PDF file not created, skipping test")
        
        # 模拟过程
        with patch('app.document_processing.factory.create_parser') as mock_create_parser:
            with patch('app.document_processing.factory.create_chunker') as mock_create_chunker:
                # 设置模拟PDF解析
                mock_parser = MagicMock()
                pdf_content = """PDF测试文件

这是一个用于测试的PDF文档。
它包含多个段落和一些格式。

第二段内容：
- 项目1
- 项目2
- 项目3

结束。"""
                mock_parser.parse.return_value = pdf_content
                mock_parser.get_metadata.return_value = {
                    "title": "PDF测试文件",
                    "page_count": 1
                }
                mock_create_parser.return_value = mock_parser
                
                # 设置模拟分块结果
                mock_chunker = MagicMock()
                mock_chunker.chunk_text.return_value = [
                    {"text": "PDF测试文件", "index": 0, "metadata": {}},
                    {"text": "这是一个用于测试的PDF文档。它包含多个段落和一些格式。", "index": 1, "metadata": {}},
                    {"text": "第二段内容：\n- 项目1\n- 项目2\n- 项目3", "index": 2, "metadata": {}},
                    {"text": "结束。", "index": 3, "metadata": {}}
                ]
                mock_create_chunker.return_value = mock_chunker
                
                # 执行处理
                content, chunks, metadata = process_file(
                    pdf_path, 
                    chunk_size=200, 
                    chunk_overlap=10, 
                    split_type="paragraph"
                )
                
                # 验证结果
                assert content == pdf_content, "Should return PDF content"
                assert len(chunks) == 4, "Should create 4 chunks from PDF"
                assert chunks[0]["text"] == "PDF测试文件", "First chunk should be title"
                assert metadata["title"] == "PDF测试文件", "Should extract PDF metadata"


# 运行测试时创建示例PDF提示
if __name__ == "__main__":
    create_test_pdf()
    pytest.main(["-xvs", __file__])