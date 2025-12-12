package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"my-bot-go/internal/config"
	"my-bot-go/internal/database"
	"my-bot-go/internal/telegram"
	"time"

	"github.com/go-resty/resty/v2"
)

type ManyACGArtwork struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	ImageURL string `json:"image"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

func StartManyACG(ctx context.Context, cfg *config.Config, db *database.D1Client, botHandler *telegram.BotHandler) {
	client := resty.New()
	client.SetTimeout(30 * time.Second)
	client.SetRetryCount(2)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			log.Println("🔍 Checking ManyACG...")

			url := "https://api.manyacg.com/artworks?limit=10" // 根据 ManyACG API 文档调整

			resp, err := client.R().Get(url)
			if err != nil {
				log.Printf("ManyACG API Error: %v", err)
				time.Sleep(1 * time.Minute)
				continue
			}

			var artworks []ManyACGArtwork
			if err := json.Unmarshal(resp.Body(), &artworks); err != nil {
				log.Printf("ManyACG JSON Error: %v", err)
				time.Sleep(1 * time.Minute)
				continue
			}

			for _, artwork := range artworks {
				pid := fmt.Sprintf("manyacg_%d", artwork.ID)
				if db.History[pid] {
					continue
				}

				log.Printf("⬇️ Downloading ManyACG: %d", artwork.ID)

				imgResp, err := client.R().Get(artwork.ImageURL)
				if err != nil {
					log.Printf("Failed to download image: %v", err)
					continue
				}

				caption := fmt.Sprintf("ManyACG: %s\nID: %d", artwork.Title, artwork.ID)

				botHandler.ProcessAndSend(ctx, imgResp.Body(), pid, artwork.Title, caption, "manyacg", artwork.Width, artwork.Height)

				db.History[pid] = true

				db.PushHistory()
				
				time.Sleep(3 * time.Second)
			}

			log.Println("😴 ManyACG Done. Sleeping 10m...")
			time.Sleep(10 * time.Minute)
		}
	}
}
