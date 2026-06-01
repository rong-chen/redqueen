package middleware

import (
	"net/http"
	"strings"

	"RedQueenSystem/utils"

	"github.com/gin-gonic/gin"
)

// AuthRequired 用于拦截保护的 API，校验 Authorization 标头中的 Token
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			utils.Error(c, http.StatusUnauthorized, "请求未携带会话 Token 凭证")
			c.Abort()
			return
		}

		// 解析 Bearer 格式的 Token
		parts := strings.SplitN(authHeader, " ", 2)
		var token string
		if len(parts) == 2 && parts[0] == "Bearer" {
			token = parts[1]
		} else {
			token = parts[0]
		}

		// 检查 Token 格式有效性: 必须以 'rq_' 开头且长度匹配 hex(16) -> 32字符
		if !strings.HasPrefix(token, "rq_") || len(token) != 35 {
			utils.Error(c, http.StatusUnauthorized, "会话 Token 凭证无效或已过期")
			c.Abort()
			return
		}

		// 将 token 写入上下文，便于后期审计
		c.Set("session_token", token)
		c.Next()
	}
}
