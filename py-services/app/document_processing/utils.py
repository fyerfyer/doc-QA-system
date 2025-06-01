import os
import re
import json
import mimetypes
import tempfile
from typing import Dict, List, Any, Optional, Union, Tuple
from datetime import datetime

from app.utils.utils import logger, count_words, count_chars


def clean_text(text: str) -> str:
    """
    清理文本，删除多余的空白字符和特殊字符
    
    参数:
        text: 要清理的文本
        
    返回:
        str: 清理后的文本
    """
    if not text:
        return ""
    
    # 替换连续的空行为单个空行
    text = re.sub(r'\n{3,}', '\n\n', text)
    
    # 删除不可见控制字符(保留换行和制表符)
    text = re.sub(r'[\x00-\x09\x0B\x0C\x0E-\x1F\x7F]', '', text)
    
    # 整理空白字符
    text = re.sub(r' {2,}', ' ', text)
    
    # 修剪每行开头和结尾的空白
    lines = [line.strip() for line in text.splitlines()]
    text = '\n'.join(lines)
    
    return text.strip()


def extract_title_from_content(content: str, filename: Optional[str] = None) -> str:
    """
    从文档内容中提取标题
    
    参数:
        content: 文档内容
        filename: 文件名(如果无法提取标题则使用)
        
    返回:
        str: 文档标题
    """
    # 如果内容为空，使用文件名或默认标题
    if not content:
        if filename:
            return os.path.splitext(os.path.basename(filename))[0]
        else:
            return "Untitled Document"
    
    # 尝试从内容前几行中提取标题
    lines = content.strip().split('\n')
    for i, line in enumerate(lines[:5]):  # 只检查前5行
        line = line.strip()
        # 如果行不为空并且长度在合理范围内，可能是标题
        if line and 3 <= len(line) <= 100:
            # 排除纯数字、日期等不太可能是标题的行
            if not re.match(r'^[\d\W]+$', line):
                return line
    
    # 如果无法从内容中提取，使用文件名或默认标题
    if filename:
        return os.path.splitext(os.path.basename(filename))[0]
    
    # 最后的后备选项
    return "Untitled Document"


def extract_metadata_from_pdf(file_path: str) -> Dict[str, Any]:
    """
    从PDF文件提取元数据
    
    参数:
        file_path: PDF文件路径
        
    返回:
        Dict[str, Any]: 元数据字典
    """
    try:
        import fitz  # PyMuPDF
        
        metadata = {}
        with fitz.open(file_path) as doc:
            # 提取基本信息
            metadata = {
                'title': doc.metadata.get('title', ''),
                'author': doc.metadata.get('author', ''),
                'subject': doc.metadata.get('subject', ''),
                'keywords': doc.metadata.get('keywords', ''),
                'creator': doc.metadata.get('creator', ''),
                'producer': doc.metadata.get('producer', ''),
                'creation_date': doc.metadata.get('creationDate', ''),
                'mod_date': doc.metadata.get('modDate', ''),
                'page_count': doc.page_count
            }
            
            # 添加文档统计信息
            total_words = 0
            total_chars = 0
            for page_num in range(doc.page_count):
                page = doc[page_num]
                text = page.get_text()
                total_words += count_words(text)
                total_chars += count_chars(text)
                
            metadata['words'] = total_words
            metadata['chars'] = total_chars
            
        return metadata
    
    except ImportError:
        logger.warning("PyMuPDF (fitz) not installed. Cannot extract detailed PDF metadata.")
        return {}
    except Exception as e:
        logger.error(f"Error extracting PDF metadata: {str(e)}")
        return {}


def download_file_to_temp(url: str, file_name: Optional[str] = None) -> Tuple[str, bool]:
    """
    下载文件到临时目录
    
    参数:
        url: 文件URL
        file_name: 文件名(可选，用于确定文件扩展名)
        
    返回:
        Tuple[str, bool]: (临时文件路径, 是否成功)
    """
    try:
        import requests
        
        # 确定文件扩展名
        if file_name:
            ext = os.path.splitext(file_name)[1]
        else:
            # 从URL尝试提取文件名
            url_path = url.split('?')[0]  # 移除查询参数
            ext = os.path.splitext(os.path.basename(url_path))[1]
            
        # 如果无法确定扩展名，使用通用扩展名
        if not ext:
            ext = '.bin'
        
        # 创建临时文件
        with tempfile.NamedTemporaryFile(delete=False, suffix=ext) as temp_file:
            temp_path = temp_file.name
        
        # 下载文件
        logger.info(f"Downloading file from URL: {url}")
        response = requests.get(url, stream=True, timeout=30)
        response.raise_for_status()
        
        # 写入文件
        with open(temp_path, 'wb') as f:
            for chunk in response.iter_content(chunk_size=8192):
                if chunk:
                    f.write(chunk)
        
        logger.info(f"File downloaded to: {temp_path}")
        return temp_path, True
    
    except Exception as e:
        logger.error(f"Error downloading file: {str(e)}")
        return "", False


def get_file_info(file_path: str) -> Dict[str, Any]:
    """
    获取文件基本信息
    
    参数:
        file_path: 文件路径
        
    返回:
        Dict[str, Any]: 文件信息字典
    """
    if not os.path.exists(file_path):
        return {
            'exists': False,
            'filename': os.path.basename(file_path),
            'extension': os.path.splitext(file_path)[1].lower()
        }
    
    file_stat = os.stat(file_path)
    
    # 获取MIME类型
    mime_type, _ = mimetypes.guess_type(file_path)
    
    return {
        'exists': True,
        'filename': os.path.basename(file_path),
        'extension': os.path.splitext(file_path)[1].lower(),
        'file_size': file_stat.st_size,
        'created_at': datetime.fromtimestamp(file_stat.st_ctime).isoformat(),
        'modified_at': datetime.fromtimestamp(file_stat.st_mtime).isoformat(),
        'mime_type': mime_type or 'application/octet-stream'
    }


def merge_metadata(metadata_list: List[Dict[str, Any]]) -> Dict[str, Any]:
    """
    合并多个元数据字典
    
    参数:
        metadata_list: 元数据字典列表
        
    返回:
        Dict[str, Any]: 合并后的元数据
    """
    if not metadata_list:
        return {}
    
    # 使用第一个字典作为基础
    result = metadata_list[0].copy()
    
    # 合并其余字典
    for metadata in metadata_list[1:]:
        for key, value in metadata.items():
            # 如果键不存在或值为空，则使用新值
            if key not in result or not result[key]:
                result[key] = value
            # 如果键已存在且两个值不同，尝试合并
            elif value and result[key] != value:
                # 列表类型，合并为集合后转回列表
                if isinstance(result[key], list) and isinstance(value, list):
                    result[key] = list(set(result[key] + value))
                # 字符串类型，如果一个是另一个的子字符串，使用较长的
                elif isinstance(result[key], str) and isinstance(value, str):
                    if result[key] in value:
                        result[key] = value
                    else:
                        # 保留第一个值，与测试预期一致
                        pass
                # 对于字典类型，递归合并
                elif isinstance(result[key], dict) and isinstance(value, dict):
                    result[key] = merge_metadata([result[key], value])
    
    return result


def format_chunk_for_embedding(chunk: Dict[str, Any]) -> Dict[str, Any]:
    """
    格式化文本块以便于嵌入处理
    
    参数:
        chunk: 文本块字典
        
    返回:
        Dict[str, Any]: 格式化后的文本块
    """
    # 创建一个新字典，以避免修改原始块
    formatted_chunk = chunk.copy()
    
    # 确保文本字段存在
    if 'text' not in formatted_chunk:
        logger.warning("Chunk missing 'text' field")
        formatted_chunk['text'] = ""
    
    # 确保索引字段存在
    if 'index' not in formatted_chunk:
        formatted_chunk['index'] = -1
    
    # 确保元数据字段存在
    if 'metadata' not in formatted_chunk:
        formatted_chunk['metadata'] = {}
    
    # 添加文本统计信息
    if 'text' in formatted_chunk and formatted_chunk['text']:
        text = formatted_chunk['text']
        
        if 'metadata' not in formatted_chunk:
            formatted_chunk['metadata'] = {}
            
        metadata = formatted_chunk['metadata']
        
        if 'chars' not in metadata:
            metadata['chars'] = count_chars(text)
            
        if 'words' not in metadata:
            metadata['words'] = count_words(text)
    
    return formatted_chunk