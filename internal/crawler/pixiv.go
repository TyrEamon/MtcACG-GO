package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"my-bot-go/internal/config"
	"my-bot-go/internal/database"
	"my-bot-go/internal/telegram"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// 定义更严谨的结构体，方便解析 pages 接口
type PixivPage struct {
	Urls struct {
		Original string `json:"original"`
		Small    string `json:"small"`
	} `json:"urls"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type PixivPagesResp struct {
	Body []PixivPage `json:"body"`
}

type PixivDetailResp struct {
	Body struct {
		IllustId   string `json:"illustId"`
		IllustTitle string `json:"illustTitle"`
		UserName   string `json:"userName"`
		IllustType int    `json:"illustType"` // 2=动图
		Tags       struct {
			Tags []struct {
				Tag string `json:"tag"`
			} `json:"tags"`
		} `json:"tags"`
	} `json:"body"`
}

func StartPixiv(ctx context.Context, cfg *config.Config, db *database.D1Client, botHandler *telegram.BotHandler) {
	client := resty.New()
	client.SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	client.SetHeader("Referer", "https://www.pixiv.net/")
	client.SetHeader("Cookie", "PHPSESSID="+cfg.PixivPHPSESSID)
	// 建议把超时设长一点
	client.SetTimeout(60 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Println("🍪 Checking Pixiv (Cookie Mode)...")

			for _, uid := range cfg.PixivArtistIDs {
				// 1. 获取画师所有作品列表
				resp, err := client.R().Get(fmt.Sprintf("https://www.pixiv.net/ajax/user/%s/profile/all", uid))
				if err != nil || resp.StatusCode() != 200 {
					log.Printf("⚠️ Pixiv User %s Error: %v", uid, err)
					continue
				}

				var profile struct {
					Body struct {
						Illusts map[string]interface{} `json:"illusts"`
					} `json:"body"`
				}
				json.Unmarshal(resp.Body(), &profile)

				// 提取 ID 并倒序排列 (最新的在前)
				var ids []int
				for k := range profile.Body.Illusts {
					if id, err := strconv.Atoi(k); err == nil {
						ids = append(ids, id)
					}
				}
				sort.Sort(sort.Reverse(sort.IntSlice(ids)))

				// ✅ 修正逻辑：只取 slice 的前 N 个，不再依赖 count 计数器
				// 这样无论是否已下载，都只检查最新的 PixivLimit 张，防止无限回溯旧图
				targetIDs := ids
				if len(ids) > cfg.PixivLimit {
					targetIDs = ids[:cfg.PixivLimit]
				}

				for _, id := range targetIDs {
					// 基础去重 (只要发过第一张，就算这个ID处理过了)
					mainPid := fmt.Sprintf("pixiv_%d_p0", id)
					if db.History[mainPid] {
						continue
					}

					log.Printf("🔍 Processing Pixiv ID: %d", id)

					// 2. 获取详情 (主要为了拿标题、Tags、动图判断)
					detailResp, err := client.R().Get(fmt.Sprintf("https://www.pixiv.net/ajax/illust/%d", id))
					if err != nil { continue }

					var detail PixivDetailResp
					if err := json.Unmarshal(detailResp.Body(), &detail); err != nil {
						continue
					}
					
					// 如果是动图 (IllustType == 2)，暂时跳过
					if detail.Body.IllustType == 2 {
						log.Printf("⚠️ Skip Ugoira (GIF): %d", id)
						db.History[mainPid] = true
						continue 
					}

					// Tags 拼接
					var tagStrs []string
					for _, t := range detail.Body.Tags.Tags {
						tagStrs = append(tagStrs, t.Tag)
					}
					tagsStr := strings.Join(tagStrs, " ")
					
					// 3. 获取 Pages (多图)
					pagesResp, err := client.R().Get(fmt.Sprintf("https://www.pixiv.net/ajax/illust/%d/pages?lang=zh", id))
					if err != nil { continue }

					var pages PixivPagesResp
					json.Unmarshal(pagesResp.Body(), &pages)

					if len(pages.Body) == 0 {
						continue
					}

					// 4. 开始处理每一张图
					maxPages := 5 
					
					for i, page := range pages.Body {
						if i >= maxPages { break }

						subPid := fmt.Sprintf("pixiv_%d_p%d", id, i)
						
						if db.History[subPid] {
							continue
						}

						log.Printf("⬇️ Downloading Pixiv: %s (P%d)", detail.Body.IllustTitle, i)
						
						imgResp, err := client.R().Get(page.Urls.Original)
						if err != nil || imgResp.StatusCode() != 200 {
							log.Printf("❌ Download failed: %v", err)
							continue
						}

						caption := fmt.Sprintf("Pixiv: %s [P%d/%d]\nArtist: %s\nTags: #%s", 
							detail.Body.IllustTitle, i+1, len(pages.Body), 
							detail.Body.UserName, 
							strings.ReplaceAll(tagsStr, " ", " #"))

						// ✅ 关键回退：强制传 0, 0 作为宽高
						// 既然你的 Bot 以前能跑，说明 ProcessAndSend 在收到 0 时或者不传时，Telegram 能够自动处理
						// 只要不把 Pixiv 返回的奇怪数值（可能导致 400 错误）传过去就行
						botHandler.ProcessAndSend(ctx, imgResp.Body(), subPid, tagsStr, caption, "pixiv", 0, 0)
						
						time.Sleep(3 * time.Second) 
					}
					
					db.PushHistory()
				}
			}

			log.Println("😴 Pixiv Done. Sleeping 10m...")
			time.Sleep(10 * time.Minute)
		}
	}
}
