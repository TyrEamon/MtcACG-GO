package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"my-bot-go/internal/config"
	"my-bot-go/internal/database"
	"my-bot-go/internal/telegram"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// ManyACGResponse 对应 https://manyacg.top/api/v1/artwork/random 的返回结构
type ManyACGResponse struct {
	Data []struct {
		ID       string `json:"id"` // JSON返回的是字符串ID
		Title    string `json:"title"`
		Artist   struct {
			Name string `json:"name"`
		} `json:"artist"`
		Pictures []struct {
			Regular string `json:"regular"`
			Width   int    `json:"width"`  // ✅ 直接读取 API 提供的宽高
			Height  int    `json:"height"` // ✅ 直接读取 API 提供的宽高
		} `json:"pictures"`
		Tags []string `json:"tags"`
		R18  bool     `json:"r18"`
	} `json:"data"`
}

func StartManyACG(ctx context.Context, cfg *config.Config, db *database.D1Client, botHandler *telegram.BotHandler) {
	client := resty.New()
	client.SetTimeout(60 * time.Second)
	client.SetRetryCount(3)
	client.SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Println("🎲 Starting Batch ManyACG (10 Pics)...")

			// ✅ 批量抽 10 次
			for i := 0; i < 10; i++ {
				url := "https://manyacg.top/api/v1/artwork/random"

				resp, err := client.R().Get(url)
				if err != nil {
					log.Printf("ManyACG API Error: %v", err)
					continue
				}

				var result ManyACGResponse
				if err := json.Unmarshal(resp.Body(), &result); err != nil {
					log.Printf("ManyACG JSON Error: %v", err)
					continue
				}

				for _, item := range result.Data {
					// 构造去重 ID，因为 ID 是字符串，直接用
					pid := fmt.Sprintf("manyacg_%s", item.ID)

					if db.History[pid] {
						// log.Printf("⏭️ ManyACG %s 已存在，跳过", item.ID)
						continue
					}

					if len(item.Pictures) == 0 {
						continue
					}
					
					pic := item.Pictures[0] // 拿第一张图
					imgURL := pic.Regular
					
					// ✅ 直接从 JSON 获取宽高
					width := pic.Width
					height := pic.Height

					log.Printf("⬇️ Downloading ManyACG: %s (%dx%d)", item.Title, width, height)

					// 下载图片 (仅为了发送，不需要再分析了)
					imgResp, err := client.R().Get(imgURL)
					if err != nil {
						log.Printf("Failed to download image: %v", err)
						continue
					}

					// 构造文案
					tags := item.Tags
					if item.R18 {
						tags = append(tags, "R-18")
					}
					tagsStr := strings.Join(tags, " ")
					caption := fmt.Sprintf("MtcACG: %s\nArtist: %s\nTags: #%s",
						item.Title,
						item.Artist.Name,
						strings.ReplaceAll(tagsStr, " ", " #"),
					)

					botHandler.ProcessAndSend(ctx, imgResp.Body(), pid, tagsStr, caption, "manyacg", width, height)

					db.PushHistory()
					time.Sleep(3 * time.Second)
				}
				
				// 每次 API 请求间隔 1 秒
				time.Sleep(1 * time.Second)
			}

			log.Println("😴 ManyACG Batch Done. Sleeping 5m...")
			time.Sleep(5 * time.Minute)
		}
	}
}
