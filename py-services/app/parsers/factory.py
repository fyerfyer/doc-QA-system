import logging
from typing import Optional, Type
from pathlib import Path

from app.parsers.base import BaseParser
from app.parsers.pdf_parser import PDFParser
from app.parsers.markdown_parser import MarkdownParser
from app.parsers.text_parser import TextParser

# 初始化日志记录器
logger = logging.getLogger(__name__)

# 扩展名到解析器类的映射
EXTENSION_TO_PARSER = {
    # PDF 文件
    'pdf': PDFParser,

    # Markdown 文件
    'md': MarkdownParser,
    'markdown': MarkdownParser,
    'mdown': MarkdownParser,
    'mkd': MarkdownParser,

    # 纯文本和代码文件 (使用TextParser处理)
    'txt': TextParser,
    'text': TextParser,
    'log': TextParser,
    'csv': TextParser,
    'tsv': TextParser,
    'json': TextParser,
    'xml': TextParser,
    'yaml': TextParser,
    'yml': TextParser,
    'html': TextParser,
    'htm': TextParser,
    'css': TextParser,
    'js': TextParser,
    'py': TextParser,
    'go': TextParser,
    'java': TextParser,
    'c': TextParser,
    'cpp': TextParser,
    'cs': TextParser,
    'rs': TextParser,
    'sh': TextParser,
    'bat': TextParser,
    'ps1': TextParser,

    # TODO: 添加更多文件类型的支持，如docx, pptx, xlsx等
}

# 内容类型到解析器类的映射
MIME_TO_PARSER = {
    'application/pdf': PDFParser,
    'text/markdown': MarkdownParser,
    'text/plain': TextParser,
    'text/html': TextParser,
    'text/css': TextParser,
    'text/javascript': TextParser,
    'application/json': TextParser,
    'application/xml': TextParser,

    # TODO: 添加更多MIME类型支持
}

def get_parser_for_extension(extension: str) -> Optional[Type[BaseParser]]:
    """
    根据文件扩展名获取对应的解析器类

    Args:
        extension: 文件扩展名（不含点号）

    Returns:
        Type[BaseParser]: 解析器类，如果不支持则返回None
    """
    return EXTENSION_TO_PARSER.get(extension.lower())

def get_parser_for_mime_type(mime_type: str) -> Optional[Type[BaseParser]]:
    """
    根据MIME类型获取对应的解析器类

    Args:
        mime_type: MIME类型字符串

    Returns:
        Type[BaseParser]: 解析器类，如果不支持则返回None
    """
    return MIME_TO_PARSER.get(mime_type.lower())

def get_extension(file_path: str) -> str:
    """
    从文件路径获取扩展名

    Args:
        file_path: 文件路径

    Returns:
        str: 文件扩展名（小写，不含点号）
    """
    return Path(file_path).suffix.lower().lstrip('.')

def create_parser(file_path: str = None, mime_type: str = None, fallback_extension: str = None) -> BaseParser:
    """
    根据文件路径或MIME类型创建适当的解析器
    
    Args:
        file_path: 文件路径（可选）
        mime_type: MIME类型（可选）
        fallback_extension: 备用扩展名（可选）
        
    Returns:
        BaseParser: 解析器实例
    """
    parser_class = None

    # 如果提供了文件路径，尝试根据扩展名获取解析器
    if file_path:
        extension = get_extension(file_path)
        if extension:
            parser_class = get_parser_for_extension(extension)
            if parser_class:
                logger.info(f"Selected parser {parser_class.__name__} based on file extension '{extension}'")
            else:
                logger.warning(f"No parser found for extension '{extension}'")
                
    # 使用备用扩展名（如果提供）
    if not parser_class and fallback_extension:
        parser_class = get_parser_for_extension(fallback_extension)
        if parser_class:
            logger.info(f"Selected parser {parser_class.__name__} based on fallback extension '{fallback_extension}'")

    # 如果通过扩展名无法确定解析器，但提供了MIME类型，则尝试根据MIME类型获取解析器
    if not parser_class and mime_type:
        parser_class = get_parser_for_mime_type(mime_type)
        if parser_class:
            logger.info(f"Selected parser {parser_class.__name__} based on MIME type '{mime_type}'")
        else:
            logger.warning(f"No parser found for MIME type '{mime_type}'")

    # 如果仍然无法确定解析器，使用TextParser作为默认解析器
    if not parser_class:
        logger.warning("Using TextParser as fallback parser for unidentified file type")
        from app.parsers.text_parser import TextParser
        return TextParser()

    # 创建并返回解析器实例
    return parser_class()

def detect_content_type(file_path: str) -> str:
    """
    根据文件扩展名检测内容类型

    Args:
        file_path: 文件路径

    Returns:
        str: MIME类型字符串
    """
    import mimetypes

    # 确保mimetypes已初始化
    mimetypes.init()

    # 获取文件MIME类型
    mime_type, _ = mimetypes.guess_type(file_path)

    # 如果无法确定MIME类型，默认为二进制流
    if not mime_type:
        # 使用扩展名映射到常见MIME类型
        ext = get_extension(file_path)
        if ext in ['md', 'markdown', 'mdown', 'mkd']:
            return 'text/markdown'
        elif ext in ['txt', 'text']:
            return 'text/plain'
        else:
            return 'application/octet-stream'

    return mime_type