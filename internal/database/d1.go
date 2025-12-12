package database

import (
	"encoding/json"
	"fmt"
	"log"
	"my-bot-go/internal/config"
	"time"

	"github.com/go-resty/resty/v2"
)

type D1Client struct {
	client  *resty.Client
	cfg     *config.Config
	History map[string]bool
}

// D1QueryResponse 用于解析 Cloudflare D1 API 的 JSON 返回
type D1QueryResponse struct {
	Result []struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	} `json:"result"`
	Success bool `json:"success"`
}

func NewD1Client(cfg *config.Config) *D1Client {
	return &D1Client{
		client:  resty.New(),
		cfg:     cfg,
		History: make(map[string]bool),
	}
}

// SyncHistory 直接从 D1 数据库拉取所有已存在的 ID 到内存
func (d *D1Client) SyncHistory() {
	log.Println("📥 Loading history directly from D1 Database...")

	// 构造 D1 查询 URL
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s/query",
		d.cfg.CF_AccountID, d.cfg.D1_DatabaseID)
	
	// SQL: 只查询 ID 列，减少数据传输量
	body := map[string]interface{}{
		"sql":    "SELECT id FROM images",
		"params": []interface{}{},
	}

	resp, err := d.client.R().
		SetHeader("Authorization", "Bearer "+d.cfg.CF_APIToken).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(url)

	if err != nil {
		log.Printf("⚠️ Sync history failed (Network): %v", err)
		return
	}

	// 解析响应
	var d1Resp D1QueryResponse
	if err := json.Unmarshal(resp.Body(), &d1Resp); err != nil {
		log.Printf("⚠️ Sync history failed (JSON Parse): %v", err)
		return
	}

	if !d1Resp.Success || len(d1Resp.Result) == 0 {
		log.Println("⚠️ Sync history failed: D1 API returned success=false or empty result")
		return
	}

	// 将 ID 存入内存 Map
	count := 0
	for _, row := range d1Resp.Result[0].Results {
		if row.ID != "" {
			d.History[row.ID] = true
			count++
		}
	}

	log.Printf("✅ Synced %d items from D1 Database", count)
}

// PushHistory 已废弃，因为 SaveImage 已经实时写入数据库了，不需要再推送到 Worker
func (d *D1Client) PushHistory() {
	// 空函数，保留为了兼容已有调用，但不做任何事
}

// SaveImage 将图片信息写入 D1 并更新内存缓存
func (d *D1Client) SaveImage(postID, fileID, caption, tags, source string, width, height int) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s/query", 
		d.cfg.CF_AccountID, d.cfg.D1_DatabaseID)
	
	finalTags := fmt.Sprintf("%s %s", tags, source)
	
	sql := "INSERT OR IGNORE INTO images (id, file_name, caption, tags, created_at, width, height) VALUES (?, ?, ?, ?, ?, ?, ?)"
	params := []interface{}{postID, fileID, caption, finalTags, time.Now().Unix(), width, height}
	
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
	
	// 写入成功后，立即在内存中标记为“已处理”
	d.History[postID] = true
	log.Printf("💾 Saved to D1: %s", postID)
	
	return nil
}
