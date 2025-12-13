package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"my-bot-go/internal/config"
	"my-bot-go/internal/database"
	"my-bot-go/internal/telegram"

	"github.com/go-resty/resty/v2"
)

// CosineImage 对应 pic.cosine.ren API 返回的单个图片结构
type CosineImage struct {
	ID        int      `json:"id"`
	PID       string   `json:"pid"`       // Pixiv ID
	Title     string   `json:"title"`
	Author    string   `json:"author"`
	RawURL    string   `json:"rawurl"`    // 原图链接
	ThumbURL  string   `json:"thumburl"`  // 缩略图
	Extension string   `json:"extension"`
	Filename  string   `json:"filename"`
	Tags      []string `json:"tags"`
	Width     int      `json:"width"`     // 接口里包含了宽高
	Height    int      `json:"height"`
}

// CosineTagConfig 自定义配置（你可以在 config 包里加这些字段，或者直接在这里硬编码）
type CosineTagConfig struct {
	TargetTags []string // 要爬的标签列表，例如 []string{"原神", "崩坏星穹铁道"}
	LimitPerTag int      // 每个标签爬多少张
}

func StartCosineTag(ctx context.Context, cfg *config.Config, db *database.D1Client, botHandler *telegram.BotHandler) {
	// ================= 自定义配置区域 =================
	tagConfig := CosineTagConfig{
		TargetTags: []string{"原神", "崩坏星穹铁道", "甜妹"}, // 🎯 在这里修改你想爬的 Tag
		LimitPerTag: 50,                               // 🎯 每个 Tag 检查前 50 张（去重后可能少于50）
	}
	// ===============================================

	client := resty.New()
	// 基础超时设置
	client.SetTimeout(30 * time.Second)

	// 1. 索引请求 Header (模拟访问 cosine 站)
	indexHeaders := map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		"Referer":    "https://pic.cosine.ren/",
	}

	// 2. 下载请求 Header (针对 Pixiv 防盗链)
	pixivHeaders := map[string]string{
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		"Referer":    "https://www.pixiv.net/",
	}

	log.Println("🚀 Starting Cosine Tag Crawler...")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			for _, tag := range tagConfig.TargetTags {
				log.Printf("🏷️  Scanning Tag: %s", tag)
				
				processedCount := 0
				start := 0
				limit := 32 // API 固定每页 32

				for processedCount < tagConfig.LimitPerTag {
					// 构造 API URL
					// 注意：tag 需要 URL 编码，Resty 的 QueryParam 会自动处理
					apiURL := "https://pic.cosine.ren/api/tag"

					// 发送请求获取索引
					resp, err := client.R().
						SetHeaders(indexHeaders).
						SetQueryParams(map[string]string{
							"tag":   tag,
							"start": fmt.Sprintf("%d", start),
							"limit": fmt.Sprintf("%d", limit),
						}).
						Get(apiURL)

					if err != nil || resp.StatusCode() != 200 {
						log.Printf("❌ API Request Failed for tag %s: %v", tag, err)
						break
					}

					// 解析 JSON
					var images []CosineImage
					if err := json.Unmarshal(resp.Body(), &images); err != nil {
						log.Printf("❌ JSON Unmarshal Failed: %v", err)
						break
					}

					if len(images) == 0 {
						log.Println("🏁 No more images for this tag.")
						break
					}

					log.Printf("📄 Fetched %d images from page (start=%d)", len(images), start)

					// 遍历当前页图片
					for _, img := range images {
						if processedCount >= tagConfig.LimitPerTag {
							break
						}

						// 构造唯一的 PID 用于去重
						// 注意：cosine 里的 PID 通常就是 Pixiv ID
						// 如果是多图，API 返回的是单独的记录吗？根据之前抓包，好像是 p0, p1 分开的记录
						// 这里假设 filename 包含了 p0, p1 信息，或者直接用 filename 去重更稳
						
						// 提取 PID 变体，例如 "12345_p0"
						// 使用 filename 去掉后缀作为 ID 更安全，因为它唯一对应一张图
						uniqueID := strings.TrimSuffix(img.PID, "." + img.Extension)
                        // 如果 PID 只是数字，我们可以手动构造 pixiv_xxxx_p0 格式以兼容你原来的系统
                        // 根据 API 返回，filename 是 "12345_p0.jpg"，pid 是 "12345"
                        // 建议：直接用 filename 去后缀作为 key，例如 "133280809_p0"
                        
                        // 修正：根据你之前提供的JSON，filename 如 "133280809_p0.jpg"
                        dbKey := strings.TrimSuffix(img.Filename, "." + img.Extension) 
                        // 为了兼容你原来的 pixiv.go 逻辑 (pixiv_12345_p0)，我们可能需要调整格式
                        // 如果原来的 key 是 "pixiv_12345_p0"，那我们需要转换一下
                        if !strings.HasPrefix(dbKey, "pixiv_") {
                            // 尝试转换 "12345_p0" -> "pixiv_12345_p0"
                             dbKey = "pixiv_" + dbKey
                        }

						if db.History[dbKey] {
							// log.Printf("⏭️  Skipping existing: %s", dbKey)
							continue
						}

						// 确定下载 URL (优先 rawurl)
						downloadURL := img.RawURL
						if downloadURL == "" {
							downloadURL = img.ThumbURL
						}

						log.Printf("⬇️  Downloading: %s (%s)", img.Title, dbKey)

						// 切换 Header
						dlHeaders := indexHeaders
						if strings.Contains(downloadURL, "pximg.net") {
							dlHeaders = pixivHeaders
						}

						// 下载图片
						imgResp, err := client.R().
							SetHeaders(dlHeaders).
							Get(downloadURL)

						if err != nil || imgResp.StatusCode() != 200 {
							log.Printf("⚠️  Download Failed: %s", downloadURL)
							continue
						}

						// 构造 Caption
						cleanTitle := cleanText(img.Title)
						tagsStr := strings.Join(img.Tags, " #")
						caption := fmt.Sprintf("Title: %s\nArtist: %s\nTags: #%s\nSource: %s",
							cleanTitle, img.Author, tagsStr, "pic.cosine.ren")

						// 发送给 Telegram (复用你原来的 BotHandler)
						// 假设 ProcessAndSend 接受的是 []byte
						err = botHandler.ProcessAndSend(ctx, imgResp.Body(), dbKey, strings.Join(img.Tags, " "), caption, "pixiv", img.Width, img.Height)
                        
                        if err == nil {
                            // 只有发送成功才记入历史
                            db.History[dbKey] = true
                            db.PushHistory() // 及时保存
                            processedCount++
                            // 礼貌延时
                            time.Sleep(3 * time.Second)
                        } else {
                            log.Printf("⚠️ TG Send Failed: %v", err)
                        }
					}
					
					// 翻页
					start += limit
					time.Sleep(1 * time.Second)
				}
			}

			log.Println("😴 Cosine Crawler Cycle Done. Sleeping 4 hours...")
			time.Sleep(4 * time.Hour)
		}
	}
}

// 辅助函数：清理标题里的非法字符（如果是文件名才需要，TG Caption 不需要太严格）
func cleanText(s string) string {
	return strings.TrimSpace(s)
}

