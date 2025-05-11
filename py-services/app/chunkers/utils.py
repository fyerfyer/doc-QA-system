import logging
import re
from pathlib import Path
from typing import List, Tuple

import unicodedata

# 导入第三方库
try:
    # 用于语言检测
    import langdetect

    HAS_LANGDETECT = True
except ImportError:
    HAS_LANGDETECT = False

try:
    # 用于中文文本处理
    import jieba

    HAS_JIEBA = True
except ImportError:
    HAS_JIEBA = False

try:
    # 用于日语文本处理
    import fugashi

    HAS_FUGASHI = True
except ImportError:
    HAS_FUGASHI = False

try:
    # 用于更高级的句子分割
    import nltk
    from nltk.tokenize import sent_tokenize

    # 如果需要下载资源，可以放在此处
    # nltk.download('punkt')
    HAS_NLTK = True
except ImportError:
    HAS_NLTK = False

# 初始化日志
logger = logging.getLogger(__name__)

# 句子终止符号 (多语言)
SENTENCE_ENDINGS = {'.', '!', '?', '。', '！', '？', '…', '︒', '︓', '︔', '︕', '︖', '︙'}

# 非中断标点符号集合
NON_BREAKING_PUNCTUATION = {',', ';', ':', '，', '；', '：'}

# 开闭引号、括号对
PAIRED_MARKERS = {
    '"': '"',
    "'": "'",
    '"""': '"""',
    "「": "」",
    "『": "』",
    "(": ")",
    "[": "]",
    "{": "}",
    "（": "）",
    "【": "】",
    "《": "》",
    "<": ">"
}

# 控制每个块最大的token数
MAX_TOKENS_PER_CHUNK = 1024

def detect_language(text: str) -> str:
    """
    检测文本的语言

    参数:
        text: 要检测的文本

    返回:
        str: 语言代码 (如 'en', 'zh', 'ja')
    """
    if not text or len(text) < 10:
        return 'en'  # 文本过短时默认为英文

    try:
        if HAS_LANGDETECT:
            return langdetect.detect(text)
        else:
            # 简单的启发式语言检测
            # 检查中文字符的比例
            chinese_chars = sum(1 for c in text if '\u4e00' <= c <= '\u9fff')
            # 检查日文字符的比例
            japanese_chars = sum(1 for c in text if '\u3040' <= c <= '\u30ff' or '\u31f0' <= c <= '\u31ff')
            # 检查韩文字符的比例
            korean_chars = sum(1 for c in text if '\u3130' <= c <= '\u318f' or '\uac00' <= c <= '\ud7af')

            total_chars = len(text)
            if chinese_chars / total_chars > 0.1:
                return 'zh'
            elif japanese_chars / total_chars > 0.1:
                return 'ja'
            elif korean_chars / total_chars > 0.1:
                return 'ko'
            else:
                return 'en'
    except Exception as e:
        logger.warning(f"Language detection failed: {str(e)}. Defaulting to 'en'")
        return 'en'


def normalize_text(text: str) -> str:
    """
    规范化文本 - 统一换行符、删除多余空白等

    参数:
        text: 要规范化的文本

    返回:
        str: 规范化后的文本
    """
    if not text:
        return ""

    # 统一换行符
    text = text.replace('\r\n', '\n').replace('\r', '\n')

    # 替换连续的空行为最多两个换行符
    text = re.sub(r'\n{3,}', '\n\n', text)

    # 规范化Unicode，将组合字符转换为单个字符
    text = unicodedata.normalize('NFC', text)

    # 移除零宽字符
    text = re.sub(r'[\u200b\u200c\u200d\u2060\ufeff]', '', text)

    # 替换连续的空格为单个空格
    text = re.sub(r' {2,}', ' ', text)

    # 移除控制字符
    text = re.sub(r'[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]', '', text)

    return text.strip()


def count_tokens(text: str, lang: str = None) -> int:
    """
    估计文本中的token数量 (根据不同语言的规则)

    参数:
        text: 要计算token数的文本
        lang: 语言代码，如果为None则自动检测

    返回:
        int: 估计的token数
    """
    if not text:
        return 0

    # 如果未指定语言，则检测语言
    if lang is None:
        lang = detect_language(text)

    # 对于中文、日文和韩文，每个字符可能是一个token
    if lang in ['zh', 'ja', 'ko']:
        # 特殊处理中文
        if lang == 'zh' and HAS_JIEBA:
            words = list(jieba.cut(text))
            return len(words)
        # 特殊处理日文
        elif lang == 'ja' and HAS_FUGASHI:
            tagger = fugashi.Tagger()
            words = [word.surface for word in tagger(text)]
            return len(words)
        else:
            # 简单估计: 每个字符算作0.7个token (考虑到字符间的关联性)
            char_count = len(text)
            return max(1, int(char_count * 0.7))
    else:
        # 对于拉丁字母语言，按空格分词
        words = text.split()
        return len(words)


def is_sentence_boundary(text: str, pos: int) -> bool:
    """
    判断指定位置是否为句子边界

    参数:
        text: 文本
        pos: 位置索引

    返回:
        bool: 是否为句子边界
    """
    if pos <= 0 or pos >= len(text):
        return False

    # 检查当前字符是否为句子终止符
    if text[pos] in SENTENCE_ENDINGS or text[pos - 1] in SENTENCE_ENDINGS:
        # 检查是否在引号内
        open_quotes = []
        for i in range(pos):
            if text[i] in PAIRED_MARKERS:
                open_quotes.append(text[i])
            elif text[i] in PAIRED_MARKERS.values() and open_quotes:
                if open_quotes[-1] == list(PAIRED_MARKERS.keys())[list(PAIRED_MARKERS.values()).index(text[i])]:
                    open_quotes.pop()

        # 如果有未闭合的引号，则不是句子边界
        if open_quotes:
            return False

        # 检查后续字符是否为空白或标点符号
        next_pos = pos + 1
        if next_pos < len(text) and text[next_pos].isalnum():
            return False

        return True

    return False


def split_sentences(text: str, lang: str = None) -> List[str]:
    """
    将文本分割成句子

    参数:
        text: 要分割的文本
        lang: 语言代码，如果为None则自动检测

    返回:
        List[str]: 句子列表
    """
    if not text:
        return []

    # 规范化文本
    text = normalize_text(text)

    # 如果未指定语言，则检测语言
    if lang is None:
        lang = detect_language(text)

    # 使用NLTK的句子分割器(如果可用)
    if HAS_NLTK:
        try:
            if lang in ['zh', 'ja', 'ko']:
                # 对于中文、日文和韩文，使用自定义规则
                return custom_sentence_split(text, lang)
            else:
                # 对于其他语言，使用NLTK
                return sent_tokenize(text, language=map_language_to_nltk(lang))
        except Exception as e:
            logger.warning(f"NLTK sentence tokenization failed: {str(e)}. Falling back to custom method.")

    # 回退到自定义的句子分割逻辑
    return custom_sentence_split(text, lang)


def custom_sentence_split(text: str, lang: str) -> List[str]:
    """
    自定义句子分割逻辑

    参数:
        text: 要分割的文本
        lang: 语言代码

    返回:
        List[str]: 句子列表
    """
    # 处理不同的语言
    if lang == 'zh':
        # 中文分句，根据标点符号分割
        pattern = r'([。！？…]+)([^。！？…]*)'
    elif lang == 'ja':
        # 日文分句
        pattern = r'([。！？…︒︓︔︕︖]+)([^。！？…︒︓︔︕︖]*)'
    elif lang == 'ko':
        # 韩文分句
        pattern = r'([.!?…]+)([^.!?…]*)'
    else:
        # 默认的分句规则
        pattern = r'([.!?\n]+)([^.!?\n]*)'

    # 查找句子边界
    sentences = []
    current = ''
    parts = re.findall(pattern, text)

    if not parts and text:
        sentences.append(text)
        return sentences

    for ending, following in parts:
        if current:
            sentences.append(current + ending)
        else:
            sentences.append(ending)
        current = following

    if current:
        sentences.append(current)

    # 处理可能的空白句子
    return [s.strip() for s in sentences if s.strip()]


def map_language_to_nltk(lang: str) -> str:
    """
    将语言代码映射到NLTK支持的语言代码

    参数:
        lang: 语言代码

    返回:
        str: NLTK支持的语言代码
    """
    # NLTK语言映射
    mapping = {
        'en': 'english',
        'fr': 'french',
        'de': 'german',
        'it': 'italian',
        'es': 'spanish',
        'pt': 'portuguese',
        'ru': 'russian',
        'nl': 'dutch',
        'pl': 'polish',
        'sk': 'slovak',
        'sl': 'slovene',
        'sv': 'swedish',
        'no': 'norwegian',
        'da': 'danish',
        'fi': 'finnish',
        'et': 'estonian',
        'cs': 'czech'
    }
    return mapping.get(lang, 'english')


def calculate_overlap(chunk_size: int, overlap: int, text_length: int) -> Tuple[int, int]:
    """
    计算分块参数，确保合理的块大小和重叠

    参数:
        chunk_size: 需求的块大小
        overlap: 需求的重叠大小
        text_length: 文本总长度

    返回:
        Tuple[int, int]: 调整后的(块大小, 重叠大小)
    """
    # 如果文本较短，直接返回
    if text_length <= chunk_size:
        return text_length, 0

    # 确保重叠不超过块大小的一半
    adjusted_overlap = min(overlap, chunk_size // 2)

    # 确保重叠至少为块大小的10%
    min_overlap = max(1, chunk_size // 10)
    adjusted_overlap = max(adjusted_overlap, min_overlap)

    return chunk_size, adjusted_overlap


def find_best_split_point(text: str, target_pos: int, window: int = 100) -> int:
    """
    在目标位置附近寻找最佳的分割点(如段落结束、句子结束等)

    参数:
        text: 文本
        target_pos: 目标分割位置
        window: 查找窗口大小

    返回:
        int: 最佳分割位置
    """
    text_len = len(text)

    # 确保范围有效
    if target_pos <= 0:
        return 0
    if target_pos >= text_len:
        return text_len

    # 确定搜索窗口
    start = max(0, target_pos - window)
    end = min(text_len, target_pos + window)

    # 首选：段落边界
    for i in range(target_pos, end):
        if i + 1 < text_len and text[i] == '\n' and text[i + 1] == '\n':
            return i + 2  # 返回段落开始位置

    for i in range(target_pos, start, -1):
        if i + 1 < text_len and text[i] == '\n' and text[i + 1] == '\n':
            return i + 2  # 返回段落开始位置

    # 次选：句子边界
    for i in range(target_pos, end):
        if is_sentence_boundary(text, i):
            return i + 1  # 返回句子结尾后的位置

    for i in range(target_pos, start, -1):
        if is_sentence_boundary(text, i):
            return i + 1

    # 第三选择：单个换行符
    for i in range(target_pos, end):
        if text[i] == '\n':
            return i + 1

    for i in range(target_pos, start, -1):
        if text[i] == '\n':
            return i + 1

    # 第四选择：空格
    for i in range(target_pos, end):
        if text[i].isspace():
            return i + 1

    for i in range(target_pos, start, -1):
        if text[i].isspace():
            return i + 1

    # 如果没有找到合适的分割点，就直接使用目标位置
    return target_pos


def get_chunk_title(chunk_text: str, filename: str, chunk_index: int) -> str:
    """
    为文本块生成标题

    参数:
        chunk_text: 块文本
        filename: 文件名
        chunk_index: 块索引

    返回:
        str: 生成的标题
    """
    # 提取第一行作为候选标题
    lines = chunk_text.split('\n')
    first_line = lines[0].strip() if lines else ""

    # 如果第一行是有效的标题（不太长且不是分隔线），则使用它
    if first_line and len(first_line) < 100 and not all(c in '-=*#' for c in first_line):
        return first_line

    # 否则基于文件名和块索引生成标题
    base_filename = Path(filename).stem
    return f"{base_filename} (Part {chunk_index + 1})"


def estimate_chunk_quality(chunk: str) -> float:
    """
    估计块的质量分数(0-1)，可用于过滤低质量块

    参数:
        chunk: 文本块

    返回:
        float: 质量分数(0-1)
    """
    if not chunk:
        return 0.0

    # 计算一些基本指标
    char_count = len(chunk)
    if char_count < 10:
        return 0.0

    # 有意义字符比例
    meaningful_chars = sum(1 for c in chunk if c.isalnum() or c.isspace())
    meaningful_ratio = meaningful_chars / char_count

    # 行数
    lines = chunk.count('\n') + 1

    # 平均行长度
    avg_line_length = char_count / lines if lines > 0 else 0

    # 计算质量分数
    quality = (
            0.5 * meaningful_ratio +  # 有意义字符占比
            0.3 * min(1.0, avg_line_length / 40) +  # 平均行长度
            0.2 * min(1.0, lines / 3)  # 行数
    )

    return min(1.0, max(0.0, quality))
