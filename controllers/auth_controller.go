package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"RedQueenSystem/database"
	"RedQueenSystem/models"
	"RedQueenSystem/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// AuthController 负责用户认证与会话管理
type AuthController struct{}

// NewAuthController 实例化认证控制器
func NewAuthController() *AuthController {
	return &AuthController{}
}

// LoginInput 登录请求的 JSON 参数
type LoginInput struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录成功的响应数据
type LoginResponse struct {
	Token string      `json:"token"` // 会话凭证
	User  models.User `json:"user"`  // 用户信息
}

// Login 处理 POST /api/auth/login - 校验密码并分发 Token
func (ctrl *AuthController) Login(c *gin.Context) {
	var input LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		utils.BadRequest(c, "用户名或密码格式错误: "+err.Error())
		return
	}

	var user models.User
	// 1. 在数据库中寻找该用户
	err := database.DB.Where("username = ?", input.Username).First(&user).Error
	if err != nil {
		utils.Error(c, http.StatusUnauthorized, "用户名或密码不正确")
		return
	}

	// 2. 比对哈希后的密码
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password))
	if err != nil {
		utils.Error(c, http.StatusUnauthorized, "用户名或密码不正确")
		return
	}

	// 3. 生成唯一的会话 Token (用于鉴权中间件校验)
	token, err := generateSessionToken()
	if err != nil {
		utils.ServerError(c, "Token 生成失败: "+err.Error())
		return
	}

	// 返回登录成功
	utils.Success(c, LoginResponse{
		Token: token,
		User:  user,
	}, "登录成功")
}

// generateSessionToken 随机生成 32 字节的会话密钥 Hex 字符串
func generateSessionToken() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	// 在网关级简单使用固定前缀 + 随机 Token，供验证中间件核对
	return "rq_" + hex.EncodeToString(bytes), nil
}
