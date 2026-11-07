package core

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type resp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

// OK 统一成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, resp{Code: 0, Msg: "ok", Data: data})
}

// Err 业务错误（HTTP 200 + code != 0）。
func Err(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, resp{Code: code, Msg: msg})
}

// ErrWith 中断请求的业务错误。
func ErrWith(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(http.StatusOK, resp{Code: code, Msg: msg})
}

// Fail HTTP 状态码错误。
func Fail(c *gin.Context, httpStatus, code int, msg string) {
	c.JSON(httpStatus, resp{Code: code, Msg: msg})
}

// Unauthorized 未登录（中断请求）。
func Unauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, resp{Code: 401, Msg: "未登录或会话已过期"})
}

// BindErr 参数绑定错误。
func BindErr(c *gin.Context, err error) {
	Err(c, 400, "参数错误: "+err.Error())
}
