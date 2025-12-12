package database

import (
	"fmt"
	"log"
	"my-bot-go/internal/config"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type D1Client struct {
	client  *resty.Client
	cfg     *config.Config
	History map[string]bool // 本地缓存的已发送ID
}

func NewD1Client(cfg *config.Config) *D1Client {
	return &D1Client{
		client:  resty.New(),
		cfg:     cfg,
		History: make(map[string]bool),
	}
}

// SyncHistory 从 Worker 获取历史记录
func (d *D1Client) SyncHistory() {
	if d.cfg.WorkerURL == "" {
		return
	}
	resp, err := d.client.R().Get(d.cfg.WorkerURL + "/api/get_history")
	if err != nil {
		log.Printf("⚠️ Sync history failed: %v", err)
		return
	}
	
	ids := strings.Split(string(resp.Body()), ",")
	for _, id := range ids {
		if strings.TrimSpace(id) != "" {
			d.History[id] = true
		}
	}
	log.Printf("🧠 Synced %d items from history", len(d.History))
}

// PushHistory 上传历史记录到 Worker
func (d *D1Client) PushHistory() {
	if d.cfg.WorkerURL == "" {
		return
	}
	var idList []string
	for id := range d.History {
		idList = append(idList, id)
	}
	data := strings.Join(idList, ",")
	
	_, err := d.client.R().
		SetBody(data).
		Post(d.cfg.WorkerURL + "/api/update_history")
		
	if err != nil {
		log.Printf("⚠️ Push history failed: %v", err)
	} else {
		log.Println("☁️ History updated to cloud")
	}
}

// SaveImage 写入 D1 数据库
func (d *D1Client) SaveImage(postID, fileID, caption, tags, source string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s/query", 
		d.cfg.CF_AccountID, d.cfg.D1_DatabaseID)
	
	finalTags := fmt.Sprintf("%s %s", tags, source)
	sql := "INSERT OR IGNORE INTO images (id, file_name, caption, tags, created_at) VALUES (?, ?, ?, ?, ?)"
	params := []interface{}{postID, fileID, caption, finalTags, time.Now().Unix()}
	
	body := map[string]interface{}{
		"sql":    sql,
		"params": params,
	}

	resp, err := d.client.R().
		SetHeader("Authorization", "Bearer "+d.cfg.CF_APIToken).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(url)

	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("D1 Error: %s", resp.String())
	}
	
	// 更新本地缓存
	d.History[postID] = true
	return nil
}
