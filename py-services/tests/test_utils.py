import unittest
from unittest.mock import patch, MagicMock

import pytest
import requests
from loguru import logger

from app.utils.utils import (
    setup_logger, parse_redis_url, retry, format_task_info,
    send_callback, get_task_key, get_document_tasks_key,
    count_words, count_chars
)


class TestLogger:
    """测试日志设置功能"""

    def test_setup_logger(self):
        """测试日志设置函数"""
        # 暂存原始的处理器数量
        original_handlers = len(logger._core.handlers)

        # 设置日志
        setup_logger("DEBUG")

        # 确认处理器被添加
        assert len(logger._core.handlers) > original_handlers

        # 清理（移除处理器）
        logger.remove()


class TestRedisUrlParser:
    """测试Redis URL解析功能"""

    def test_parse_redis_url_default(self):
        """测试默认Redis URL解析"""
        # 使用默认URL
        result = parse_redis_url()

        assert result["host"] == "localhost"
        assert result["port"] == 6379
        assert result["db"] == 0
        assert result["password"] is None
        assert result["decode_responses"] is True

    def test_parse_redis_url_with_auth(self):
        """测试带认证的Redis URL解析"""
        url = "redis://user:password@redis.example.com:7000/2"
        result = parse_redis_url(url)

        assert result["host"] == "redis.example.com"
        assert result["port"] == 7000
        assert result["db"] == 2
        assert result["password"] == "password"
        assert result["decode_responses"] is True

    def test_parse_redis_url_without_auth(self):
        """测试不带认证的Redis URL解析"""
        url = "redis://redis.example.com:7000/2"
        result = parse_redis_url(url)

        assert result["host"] == "redis.example.com"
        assert result["port"] == 7000
        assert result["db"] == 2
        assert result["password"] is None
        assert result["decode_responses"] is True

    def test_parse_redis_url_without_port(self):
        """测试不带端口的Redis URL解析"""
        url = "redis://redis.example.com/2"
        result = parse_redis_url(url)

        assert result["host"] == "redis.example.com"
        assert result["port"] == 6379  # 默认端口
        assert result["db"] == 2
        assert result["password"] is None
        assert result["decode_responses"] is True


class TestRetryDecorator:
    """测试重试装饰器功能"""

    def test_retry_success_first_attempt(self):
        """测试首次尝试成功的情况"""
        mock_func = MagicMock(return_value="success")
        decorated_func = retry(max_retries=3)(mock_func)

        result = decorated_func()

        assert result == "success"
        assert mock_func.call_count == 1

    def test_retry_success_after_retries(self):
        """测试多次尝试后成功的情况"""
        # 创建一个侧效应函数，前两次抛出异常，第三次成功
        side_effect = [ValueError("Error 1"), ValueError("Error 2"), "success"]
        mock_func = MagicMock(side_effect=side_effect)
        mock_func.__name__ = "mock_function"

        # 使用较短的延迟以加速测试
        decorated_func = retry(max_retries=3, delay=0.01, backoff=1)(mock_func)

        result = decorated_func()

        assert result == "success"
        assert mock_func.call_count == 3

    def test_retry_all_attempts_fail(self):
        """测试所有尝试都失败的情况"""
        mock_func = MagicMock(side_effect=ValueError("Persistent error"))
        mock_func.__name__ = "mock_function"

        # 使用较短的延迟以加速测试
        decorated_func = retry(max_retries=3, delay=0.01, backoff=1)(mock_func)

        with pytest.raises(ValueError, match="Persistent error"):
            decorated_func()

        assert mock_func.call_count == 3

    def test_retry_specific_exceptions(self):
        """测试仅重试特定异常的情况"""
        # 创建一个函数，抛出TypeError (未在retry列表中)
        mock_func = MagicMock(side_effect=TypeError("Wrong type"))

        # 仅处理ValueError，不处理TypeError
        decorated_func = retry(max_retries=3, delay=0.01, exceptions=(ValueError,))(mock_func)

        with pytest.raises(TypeError, match="Wrong type"):
            decorated_func()

        # 函数应该只被调用一次，因为错误类型不在重试列表中
        assert mock_func.call_count == 1


class TestTaskInfo:
    """测试任务信息格式化功能"""

    def test_format_task_info(self):
        """测试格式化任务信息"""
        # 创建模拟任务对象
        task = MagicMock()
        task.id = "task-123"
        task.type = "document_parse"
        task.document_id = "doc-456"
        task.status = "processing"
        task.error = ""

        # 格式化任务信息
        result = format_task_info(task)

        # 验证结果
        assert result["task_id"] == "task-123"
        assert result["type"] == "document_parse"
        assert result["document_id"] == "doc-456"
        assert result["status"] == "processing"
        assert "error" not in result

    def test_format_task_info_with_error(self):
        """测试格式化带有错误信息的任务"""
        # 创建带有错误的模拟任务对象
        task = MagicMock()
        task.id = "task-123"
        task.type = "document_parse"
        task.document_id = "doc-456"
        task.status = "failed"
        task.error = "Something went wrong"

        # 格式化任务信息
        result = format_task_info(task)

        # 验证结果
        assert result["task_id"] == "task-123"
        assert result["type"] == "document_parse"
        assert result["document_id"] == "doc-456"
        assert result["status"] == "failed"
        assert result["error"] == "Something went wrong"


class TestCallback:
    """测试回调功能"""

    @patch('requests.post')
    def test_send_callback_success(self, mock_post):
        """测试成功发送回调"""
        # 设置模拟响应
        mock_response = MagicMock()
        mock_response.raise_for_status = MagicMock()
        mock_post.return_value = mock_response

        # 调用发送回调函数
        result = send_callback("http://example.com/callback", {"status": "completed"})

        # 验证结果
        assert result is True
        mock_post.assert_called_once_with(
            url="http://example.com/callback",
            json={"status": "completed"},
            headers={"Content-Type": "application/json"},
            timeout=10
        )

    @patch('requests.post')
    def test_send_callback_failure(self, mock_post):
        """测试发送回调失败的情况"""
        # 设置模拟异常
        mock_post.side_effect = requests.exceptions.RequestException("Connection error")

        # 调用发送回调函数
        result = send_callback("http://example.com/callback", {"status": "completed"})

        # 验证结果
        assert result is False
        mock_post.assert_called_once()


class TestKeyGeneration:
    """测试键生成功能"""

    def test_get_task_key(self):
        """测试获取任务键"""
        task_id = "task-123"
        key = get_task_key(task_id)
        assert key == "task:task-123"

    def test_get_document_tasks_key(self):
        """测试获取文档任务集合键"""
        document_id = "doc-456"
        key = get_document_tasks_key(document_id)
        assert key == "document_tasks:doc-456"


class TestTextCounting:
    """测试文本计数功能"""

    def test_count_words(self):
        """测试单词计数"""
        text = "Hello world. This is a test."
        word_count = count_words(text)
        assert word_count == 6

    def test_count_words_empty(self):
        """测试空文本的单词计数"""
        text = ""
        word_count = count_words(text)
        assert word_count == 0

    def test_count_chars(self):
        """测试字符计数"""
        text = "Hello world. This is a test."
        char_count = count_chars(text)
        # 应该排除空格，但包括标点符号
        # H(1) + e(1) + l(2) + o(1) + w(1) + o(1) + r(1) + l(1) + d(1) + .(1) +
        # T(1) + h(1) + i(2) + s(2) + a(1) + t(1) + e(1) + s(1) + t(1) + .(1) = 23
        assert char_count == 23

    def test_count_chars_empty(self):
        """测试空文本的字符计数"""
        text = ""
        char_count = count_chars(text)
        assert char_count == 0

    def test_count_chars_whitespace_only(self):
        """测试仅包含空白字符的文本计数"""
        text = "   \n\t   "
        char_count = count_chars(text)
        assert char_count == 0


if __name__ == "__main__":
    unittest.main()