// Package objectstore S3 兼容对象存储（备份远端归档目标），配置存于面板 Setting。
package objectstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gorm.io/gorm"

	"cnb.cool/hanyuxin/panel/internal/core"
)

// S3Config 对象存储配置。
type S3Config struct {
	Enabled   bool   `json:"enabled"`
	Endpoint  string `json:"endpoint"` // 如 play.min.io 或 cos.ap-guangzhou.myqcloud.com
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Bucket    string `json:"bucket"`
	UseSSL    bool   `json:"use_ssl"`
	Prefix    string `json:"prefix"` // 对象键前缀，默认 backups/
}

func Get(db *gorm.DB) S3Config {
	var c S3Config
	_ = json.Unmarshal([]byte(core.GetSetting(db, core.KS3Cfg)), &c)
	if c.Prefix == "" {
		c.Prefix = "backups/"
	}
	return c
}

func Save(db *gorm.DB, c S3Config) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return core.SetSetting(db, core.KS3Cfg, string(data))
}

func client(c S3Config) (*minio.Client, error) {
	return minio.New(c.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""),
		Secure: c.UseSSL,
	})
}

// Upload 上传本地文件到对象存储，返回对象键。
func Upload(ctx context.Context, c S3Config, localPath, objectName string) (string, error) {
	mc, err := client(c)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	object := c.Prefix + objectName
	contentType := "application/octet-stream"
	_, err = mc.FPutObject(ctx, c.Bucket, object, localPath, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", err
	}
	return object, nil
}

// Check 校验配置连通性（桶是否存在/可访问）。
func Check(c S3Config) error {
	mc, err := client(c)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ok, err := mc.BucketExists(ctx, c.Bucket)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("桶 %s 不存在", c.Bucket)
	}
	return nil
}

// ---- 路由 ----

type Module struct{ db *gorm.DB }

func New(db *gorm.DB) *Module { return &Module{db: db} }

func (m *Module) Register(api gin.IRouter) {
	g := api.Group("/s3")
	g.GET("/config", func(c *gin.Context) {
		cfg := Get(m.db)
		if cfg.SecretKey != "" {
			cfg.SecretKey = "******"
		}
		core.OK(c, cfg)
	})
	g.POST("/config", func(c *gin.Context) {
		var req S3Config
		if err := c.ShouldBindJSON(&req); err != nil {
			core.BindErr(c, err)
			return
		}
		if req.SecretKey == "******" {
			req.SecretKey = Get(m.db).SecretKey
		}
		if err := Save(m.db, req); err != nil {
			core.Err(c, 500, err.Error())
			return
		}
		core.OK(c, nil)
	})
	g.POST("/test", func(c *gin.Context) {
		cfg := Get(m.db)
		if err := Check(cfg); err != nil {
			core.Err(c, 500, "连接失败: "+err.Error())
			return
		}
		core.OK(c, nil)
	})
}
