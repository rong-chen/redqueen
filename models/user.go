package models

import "gorm.io/gorm"

// User 记录系统管理员/操作员用户模型
type User struct {
	gorm.Model
	Username string `gorm:"size:100;uniqueIndex;not null" json:"username"` // 用户名
	Password string `gorm:"size:255;not null" json:"-"`                     // 密码哈希值 (防 JSON 泄露)
	Role             string `gorm:"size:50;default:'operator'" json:"role"`         // 角色: admin (管理员), operator (操作员)
	MasterVoiceprint string `gorm:"type:text" json:"-"`                             // 用户声纹基础特征 (JSON 序列化的 []float32，为防泄露不对外输出)
}
