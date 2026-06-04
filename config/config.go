package config

import (
	"os"
	"strconv"
)

// Config 存储系统全局配置参数
type Config struct {
	Port           string // 服务器监听端口
	DBDriver       string // 数据库驱动类型: "postgres"
	DBSource       string // PostgreSQL 连接 DSN
	AllowOrigins   string // 允许的跨域来源
	WakeWord       string // 语音唤醒词，默认 "皇后"
	SessionTimeout int    // 语音会话激活态超时秒数，默认 15
}

// LoadConfig 从环境变量加载配置，如无环境变量则使用默认参数
func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "9091"
	}

	dbDriver := os.Getenv("DB_DRIVER")
	if dbDriver == "" {
		dbDriver = "postgres"
	}

	dbSource := os.Getenv("DB_SOURCE")
	if dbSource == "" {
		if dbDriver == "postgres" {
			// 默认使用 PostgreSQL 本地连接
			dbSource = "host=127.0.0.1 user=postgres password=postgres dbname=redqueen port=5432 sslmode=disable TimeZone=Asia/Shanghai"
		} else {
			dbSource = "host=127.0.0.1 user=postgres password=postgres dbname=redqueen_db port=5432 sslmode=disable TimeZone=Asia/Shanghai"
		}
	}

	allowOrigins := os.Getenv("ALLOW_ORIGINS")
	if allowOrigins == "" {
		allowOrigins = "*"
	}

	wakeWord := os.Getenv("WAKE_WORD")
	if wakeWord == "" {
		wakeWord = "皇后"
	}

	sessionTimeout := 15 // 默认 15 秒
	if timeoutStr := os.Getenv("SESSION_TIMEOUT"); timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil && t > 0 {
			sessionTimeout = t
		}
	}

	return Config{
		Port:           port,
		DBDriver:       dbDriver,
		DBSource:       dbSource,
		AllowOrigins:   allowOrigins,
		WakeWord:       wakeWord,
		SessionTimeout: sessionTimeout,
	}
}
