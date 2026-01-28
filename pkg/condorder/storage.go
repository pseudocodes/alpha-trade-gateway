// Package condorder 条件单持久化存储
package condorder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"alpha-trade-gateway/pkg/logger"
)

// Storage 条件单存储
type Storage struct {
	basePath string
}

// NewStorage 创建存储实例
func NewStorage(basePath string) *Storage {
	return &Storage{basePath: basePath}
}

// SaveCurrent 保存当前条件单数据
func (s *Storage) SaveCurrent(userKey string, data *ConditionOrderData) error {
	filename := s.getCurrentFilePath(userKey)
	return s.saveToFile(filename, data)
}

// LoadCurrent 加载当前条件单数据
func (s *Storage) LoadCurrent(userKey string, data *ConditionOrderData) error {
	filename := s.getCurrentFilePath(userKey)
	return s.loadFromFile(filename, data)
}

// SaveHistory 保存历史条件单数据
func (s *Storage) SaveHistory(userKey string, data *ConditionOrderHisData) error {
	filename := s.getHistoryFilePath(userKey)
	return s.saveToFile(filename, data)
}

// LoadHistory 加载历史条件单数据
func (s *Storage) LoadHistory(userKey string, data *ConditionOrderHisData) error {
	filename := s.getHistoryFilePath(userKey)
	return s.loadFromFile(filename, data)
}

// getCurrentFilePath 获取当前条件单文件路径
func (s *Storage) getCurrentFilePath(userKey string) string {
	return filepath.Join(s.basePath, userKey+".co.json")
}

// getHistoryFilePath 获取历史条件单文件路径
func (s *Storage) getHistoryFilePath(userKey string) string {
	return filepath.Join(s.basePath, userKey+".coh.json")
}

// saveToFile 保存到文件
func (s *Storage) saveToFile(filename string, data interface{}) error {
	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory failed: %w", err)
	}

	// 序列化
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal data failed: %w", err)
	}

	// 写入临时文件
	tmpFile := filename + ".tmp"
	if err := os.WriteFile(tmpFile, bytes, 0644); err != nil {
		return fmt.Errorf("write temp file failed: %w", err)
	}

	// 原子替换
	if err := os.Rename(tmpFile, filename); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("rename file failed: %w", err)
	}

	logger.Debug("condition order data saved",
		zap.String("filename", filename),
		zap.Int("size", len(bytes)))

	return nil
}

// loadFromFile 从文件加载
func (s *Storage) loadFromFile(filename string, data interface{}) error {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotExist
		}
		return fmt.Errorf("read file failed: %w", err)
	}

	if err := json.Unmarshal(bytes, data); err != nil {
		return fmt.Errorf("unmarshal data failed: %w", err)
	}

	logger.Debug("condition order data loaded",
		zap.String("filename", filename),
		zap.Int("size", len(bytes)))

	return nil
}
