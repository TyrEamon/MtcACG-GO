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

type YandePost struct {
	ID        int    `json:"id"`
	ParentID  int    `json:"parent_id"`
	SampleURL string `json:"sample_url"`
	FileURL   string `json:"file_url"`
	FileSize  int    `json:"file_size"`
	Tags      string `json:"tags"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

func StartYande(ctx context.Context, cfg *config.Config, db *database.D1Client, botHandler *telegram.BotHandler) {
	client := resty.New()
	client.SetTimeout(90 * time.Second)
	client.SetRetryCount(3)
	client.SetRetryWaitTime(4 * time.Second)
	// 伪装
	client.SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	tagGroups := strings.Split(cfg.YandeTags, ",")

	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Println("🔄 Starting Yande Loop...")

			//  遍历每一组任务
			for _, tags := range tagGroups {
				currentTags := strings.TrimSpace(tags)
				if currentTags == "" {
					continue
				}

				log.Printf("🔍 Checking Yande Tags: [%s] ...", currentTags)

				// 构造 URL，使用当前这组标签
				url := fmt.Sprintf("https://yande.re/post.json?limit=%d&tags=%s", cfg.YandeLimit, currentTags)

				resp, err := client.R().Get(url)
				if err != nil {
					log.Printf("Yande API Error (%s): %v", currentTags, err)
					time.Sleep(10 * time.Second)
					continue
				}

				var posts []YandePost
				if err := json.Unmarshal(resp.Body(), &posts); err != nil {
					log.Printf("Yande JSON Error (%s): %v", currentTags, err)
					time.Sleep(10 * time.Second)
					continue
				}

				if len(posts) == 0 {
					log.Printf("⚠️ No posts found for tags: %s", currentTags)
					continue
				}

				processedInLoop := make(map[int]bool)
				for _, post := range posts {
					if processedInLoop[post.ID] {
						continue
					}

                pid := fmt.Sprintf("yande_%d", post.ID)

                // 1. 先按原始 ID 查 (防止单图逻辑变动)
                if db.CheckExists(pid) {
                       continue
                    }

                targetCheckID := post.ID
                if post.ParentID != 0 {
                   targetCheckID = post.ParentID
                    }
                // 构造 _p0 格式的 ID
                pidP0 := fmt.Sprintf("yande_%d_p0", targetCheckID)

                if db.CheckExists(pidP0) {
                // 把原始 ID 也补进内存
                   db.History[pid] = true 
					log.Printf("♻️ Skip Family Group (Parent: %d) - Already in DB", targetCheckID)
                   continue
                   }



					targetID := post.ID
					if post.ParentID != 0 {
						targetID = post.ParentID
					}

					// 确保包含父图
					familyPosts := fetchFamilyWithParent(client, targetID)
					if len(familyPosts) == 0 {
						// 兜底
						familyPosts = []YandePost{post}
					}

					// 处理单图或套图
					if len(familyPosts) == 1 {
						p := familyPosts[0]
						processSingleImage(ctx, client, p, db, botHandler)
						processedInLoop[p.ID] = true
						db.History[fmt.Sprintf("yande_%d", p.ID)] = true
					} else {
						// 传入 targetID (父ID) 用于生成统一格式的 ID
						processMediaGroup(ctx, client, familyPosts, targetID, db, botHandler)
						for _, p := range familyPosts {
							processedInLoop[p.ID] = true
							db.History[fmt.Sprintf("yande_%d", p.ID)] = true
						}
					}
					
					// ✅ 每处理完一组图，立即保存历史到云端
					db.PushHistory()
					
					time.Sleep(15 * time.Second)
				}

				log.Printf("✅ Task [%s] finished. Cooldown 10s...", currentTags)
				time.Sleep(20 * time.Second)
			}

			//轮询，长睡眠
			log.Println("😴 All Yande Tasks Done. Sleeping 80m...") 
			time.Sleep(61 * time.Minute)
		}
	}
}

//先查父图再查子图
func fetchFamilyWithParent(client *resty.Client, parentID int) []YandePost {
	var finalFamily []YandePost


	urlParent := fmt.Sprintf("https://yande.re/post.json?tags=id:%d", parentID)
	respP, errP := client.R().Get(urlParent)
	var parents []YandePost
	if errP == nil {
		_ = json.Unmarshal(respP.Body(), &parents)
		if len(parents) > 0 {
			finalFamily = append(finalFamily, parents[0])
		}
	}

	// 获取所有子图
	urlChildren := fmt.Sprintf("https://yande.re/post.json?tags=parent:%d", parentID)
	respC, errC := client.R().Get(urlChildren)
	var children []YandePost
	if errC == nil {
		_ = json.Unmarshal(respC.Body(), &children)
		finalFamily = append(finalFamily, children...)
	}

	return finalFamily
}

func processSingleImage(ctx context.Context, client *resty.Client, post YandePost, db *database.D1Client, botHandler *telegram.BotHandler) {
	imgURL := selectBestImageURL(post)
	log.Printf("⬇️ Downloading Yande: %d", post.ID)

	imgResp, err := client.R().Get(imgURL)
	if err != nil {
		log.Printf("Failed to download image: %v", err)
		return
	}

	pid := fmt.Sprintf("yande_%d", post.ID)
	caption := fmt.Sprintf("Yande: %d\nTags: #%s", post.ID, strings.ReplaceAll(post.Tags, " ", " #"))

	botHandler.ProcessAndSend(ctx, imgResp.Body(), pid, post.Tags, caption, "Yande artist", "yande", post.Width, post.Height)
}

// 修改 ID 生成逻辑
func processMediaGroup(ctx context.Context, client *resty.Client, posts []YandePost, parentID int, db *database.D1Client, botHandler *telegram.BotHandler) {
	log.Printf("📦 Processing Family Group: %d (Count: %d)", parentID, len(posts))

	for i, p := range posts {
		if i >= 10 {
			break
		}

		imgURL := selectBestImageURL(p)
		imgResp, err := client.R().Get(imgURL)
		if err != nil {
			continue
		}

		// 格式化 Caption
		tags := strings.Split(p.Tags, " ")
		firstTag := ""
		if len(tags) > 0 {
			firstTag = tags[0]
		}
		caption := fmt.Sprintf("Yande Set: %d [%d/%d]\nTags: #%s", parentID, i+1, len(posts), firstTag)

		pid := fmt.Sprintf("yande_%d_p%d", parentID, i)

		botHandler.ProcessAndSend(ctx, imgResp.Body(), pid, p.Tags, caption, "Yande artist", "yande", p.Width, p.Height)
		time.Sleep(1 * time.Second)
	}
}

func selectBestImageURL(post YandePost) string {
	const MaxSize = 13 * 1024 * 1024
	if post.FileSize > 0 && post.FileSize < MaxSize {
		return post.FileURL
	}
	if post.SampleURL == "" {
		return post.FileURL
	}
	return post.SampleURL
}
