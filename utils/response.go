package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 定义了统一的 API 响应结构
type Response struct {
	Code    int         `json:"code"`    // 状态码
	Message string      `json:"message"` // 提示信息
	Data    interface{} `json:"data"`    // 数据载荷
}

// Success 返回成功的响应 (HTTP 200)
func Success(c *gin.Context, data interface{}, message string) {
	if message == "" {
		message = "操作成功"
	}
	c.JSON(http.StatusOK, Response{
		Code:    200,
		Message: message,
		Data:    data,
	})
}

// Created 返回创建成功的响应 (HTTP 201)
func Created(c *gin.Context, data interface{}, message string) {
	if message == "" {
		message = "创建成功"
	}
	c.JSON(http.StatusCreated, Response{
		Code:    201,
		Message: message,
		Data:    data,
	})
}

// Error 返回错误的响应 (指定 HTTP 状态码)
func Error(c *gin.Context, httpStatus int, message string) {
	c.JSON(httpStatus, Response{
		Code:    httpStatus,
		Message: message,
		Data:    nil,
	})
}

// BadRequest 返回请求参数错误的响应 (HTTP 400)
func BadRequest(c *gin.Context, message string) {
	if message == "" {
		message = "请求参数错误"
	}
	Error(c, http.StatusBadRequest, message)
}

// ServerError 返回服务器内部错误的响应 (HTTP 500)
func ServerError(c *gin.Context, message string) {
	if message == "" {
		message = "服务器内部错误"
	}
	Error(c, http.StatusInternalServerError, message)
}
