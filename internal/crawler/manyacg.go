package crawler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // 注册 jpeg 解码器，用于分析图片宽高
	_ "image/png"  // 注册 png 解码器，用于分析图片宽高
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
		ID       int    `json:"id"`
		Title    string `json:"title"`
		Artist   struct {
			Name string `json:"name"`
		} `json:"artist"`
		Pictures []struct {
			Regular string `json:"regular"` // 图片地址
		} `json:"pictures"`
		Tags []string `json:"tags"`
		R18  bool     `json:"r18"`
	} `json:"data"`
}

func StartManyACG(ctx context.Context, cfg *config.Config, db *database.D1Client, botHandler *telegram.BotHandler) {
	client := resty.New()
	client.SetTimeout(60 * time.Second)
	client.SetRetryCount(3)
	// 伪装 User-Agent，防止被拦截
	client.SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Println("🎲 Checking ManyACG (Random)...")

			url := "https://manyacg.top/api/v1/artwork/random"

			// 发起请求
			resp, err := client.R().Get(url)
			if err != nil {
				log.Printf("ManyACG API Error: %v", err)
				time.Sleep(3 * time.Minute)
				continue
			}

			var result ManyACGResponse
			if err := json.Unmarshal(resp.Body(), &result); err != nil {
				log.Printf("ManyACG JSON Error: %v", err)
				time.Sleep(1 * time.Minute)
				continue
			}

			// 遍历结果（通常随机图接口一次返回 1 张，但也可能是列表）
			for _, item := range result.Data {
				pid := fmt.Sprintf("manyacg_%d", item.ID)

				// 1. 去重检查
				if db.History[pid] {
					log.Printf("⏭️ ManyACG %d 已存在，跳过", item.ID)
					continue
				}

				if len(item.Pictures) == 0 {
					continue
				}
				imgURL := item.Pictures[0].Regular

				log.Printf("⬇️ Downloading ManyACG: %d", item.ID)

				// 2. 下载图片
				imgResp, err := client.R().Get(imgURL)
				if err != nil {
					log.Printf("Failed to download image: %v", err)
					continue
				}

				// 3. 自动计算图片宽高 (程序自动分析，不需要人工输入)
				width, height := 0, 0
				// bytes.NewReader 将下载的图片数据转为 Reader 供 image 库分析
				if cfg, _, err := image.DecodeConfig(bytes.NewReader(imgResp.Body())); err == nil {
					width = cfg.Width
					height = cfg.Height
				} else {
					log.Printf("⚠️ 无法解析图片宽高 (ID: %d): %v", item.ID, err)
				}

				// 4. 构造文案
				tags := item.Tags
				if item.R18 {
					tags = append(tags, "R-18")
				}
				// 替换空格，确保 tags 格式正确 (如 "Tag A" -> "TagA" 或保持原样，视需求而定，这里保留原样加 #)
				tagsStr := strings.Join(tags, " ")
				// 将 tags 里的空格转为 #，形成 Telegram 标签格式
				formattedTags := strings.ReplaceAll(tagsStr, " ", " #")

				caption := fmt.Sprintf("MtcACG: %s\nArtist: %s\nTags: #%s",
					item.Title,
					item.Artist.Name,
					formattedTags,
				)

				// 5. 发送并保存
				// 此时 width 和 height 已经是程序计算出的真实值了
				botHandler.ProcessAndSend(ctx, imgResp.Body(), pid, tagsStr, caption, "manyacg", width, height)

				db.PushHistory()

				time.Sleep(3 * time.Second) // 避免发送过快
			}

			log.Println("😴 ManyACG Done. Sleeping 5m...")
			time.Sleep(5 * time.Minute) // 随机图无需频繁请求
		}
	}
}
