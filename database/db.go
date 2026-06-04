package database

import (
	"fmt"
	"log"

	"RedQueenSystem/config"
	"RedQueenSystem/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB 全局数据库 GORM 句柄
var DB *gorm.DB

// InitDB 根据配置初始化数据库连接，并在首次启动时自动创建 admin 管理员账号
func InitDB(cfg config.Config) (*gorm.DB, error) {
	if cfg.DBDriver != "postgres" {
		return nil, fmt.Errorf("不支持的数据库驱动 DB_DRIVER: %s (目前仅支持 postgres)", cfg.DBDriver)
	}

	dialector := postgres.Open(cfg.DBSource)

	// 开启详细的 GORM 日志输出，方便调试
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	log.Println("PostgreSQL 数据库连接成功")

	// 自动迁移并同步表结构 (新增 models.User, models.ModelConfig)
	err = db.AutoMigrate(
		&models.User{},
		&models.VoiceCommand{},
		&models.ModelConfig{},
		&models.MCPServer{},
	)
	if err != nil {
		return nil, fmt.Errorf("自动迁移表结构失败: %w", err)
	}
	log.Println("数据库表结构迁移与同步成功")

	// 首次启动自动创建超级管理员 admin
	err = seedAdminUser(db)
	if err != nil {
		log.Printf("自动创建管理员账号失败: %v", err)
	}

	// 首次启动初始化大模型配置参数
	err = seedModelConfig(db)
	if err != nil {
		log.Printf("初始化大模型配置参数失败: %v", err)
	}

	DB = db
	return db, nil
}

// seedAdminUser 初始化管理员账号
func seedAdminUser(db *gorm.DB) error {
	var count int64
	// 检查是否已存在 admin 用户
	err := db.Model(&models.User{}).Where("username = ?", "admin").Count(&count).Error
	if err != nil {
		return err
	}

	if count == 0 {
		log.Println("检测到系统首次运行，正在自动创建 admin 账户...")
		
		// 加密管理员密码: 2026@redqueen!!
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("2026@redqueen!!"), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("加密管理员密码失败: %w", err)
		}

		admin := models.User{
			Username: "admin",
			Password: string(hashedPassword),
			Role:     "admin",
		}

		if err := db.Create(&admin).Error; err != nil {
			return fmt.Errorf("创建 admin 账户失败: %w", err)
		}
		log.Println("==================================================")
		log.Println("[成功] 超级管理员账号创建成功!")
		log.Println("用户名: admin")
		log.Println("默认密码已按系统安全规范初始化")
		log.Println("==================================================")
	} else {
		log.Println("admin 账号已存在，无需重复创建")
	}

	return nil
}

// seedModelConfig 自动初始化默认大语言模型配置参数
func seedModelConfig(db *gorm.DB) error {
	var count int64
	err := db.Model(&models.ModelConfig{}).Count(&count).Error
	if err != nil {
		return err
	}

	defaultPrompt := "你是一个{{.SystemRole}}，性格{{.SystemPersonality}}。【严格约束】对于涉及控制实体硬件、查询传感器或执行设备状态的操作，你必须严格基于已调用的外部工具（MCP）返回的真实数据和状态进行回复，严禁凭空编造数值或状态。如果工具执行失败，请诚实报告。对于普通的日常闲聊、故事、问候或常识性对话，你应当直接且符合你性格地进行回答，无需调用工具。"
	defaultPersonality := "冷酷的，简短的，不带语气词的"

	if count == 0 {
		log.Println("检测到系统首次运行，正在自动初始化默认大模型配置...")
		
		cfg := models.ModelConfig{
			ActiveMode:          "deepseek", // 默认直接使用大模型模式
			ApiKey:              "",
			ApiURL:              "https://api.deepseek.com", // 标准 OpenAI 格式基准 Endpoint
			ModelName:           "deepseek-v4-pro", // 默认使用 deepseek-v4-pro
			SystemPrompt:        defaultPrompt,
			SystemRole:          "红皇后",
			SystemPersonality:   defaultPersonality,
		}

		if err := db.Create(&cfg).Error; err != nil {
			return fmt.Errorf("初始化默认大模型配置失败: %w", err)
		}
		log.Println("[成功] 默认大模型配置参数初始化成功!")
	} else {
		// 【自动升级与校准逻辑】仅在数据库字段为空时进行填充，不覆盖管理员在后台自定义修改的配置
		var cfg models.ModelConfig
		if db.First(&cfg).Error == nil {
			needsUpdate := false
			if cfg.ModelName == "" {
				cfg.ModelName = "deepseek-v4-pro"
				needsUpdate = true
			}
			if cfg.ApiURL == "" {
				cfg.ApiURL = "https://api.deepseek.com"
				needsUpdate = true
			}
			if cfg.SystemPrompt == "" {
				cfg.SystemPrompt = defaultPrompt
				needsUpdate = true
			}
			if cfg.SystemRole == "" {
				cfg.SystemRole = "红皇后"
				needsUpdate = true
			}
			if cfg.SystemPersonality == "" {
				cfg.SystemPersonality = defaultPersonality
				needsUpdate = true
			}
			if needsUpdate {
				db.Save(&cfg)
			}
		}
	}

	return nil
}
