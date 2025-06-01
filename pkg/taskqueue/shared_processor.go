package taskqueue

import (
	"sync"

	"github.com/sirupsen/logrus"
)

var (
	sharedProcessor     *CallbackProcessor
	sharedProcessorOnce sync.Once
)

// GetSharedCallbackProcessor 返回一个单例的 CallbackProcessor 实例
func GetSharedCallbackProcessor(queue Queue, logger *logrus.Logger) *CallbackProcessor {
	sharedProcessorOnce.Do(func() {
		sharedProcessor = NewCallbackProcessor(queue, logger)
	})
	return sharedProcessor
}
