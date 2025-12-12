package crawler

import (
\t"context"
\t"encoding/json"
\t"fmt"
\t"log"
\t"my-bot-go/internal/config"
\t"my-bot-go/internal/database"
\t"my-bot-go/internal/telegram"
\t"strings"
\t"time"

\t"github.com/go-resty/resty/v2"
)

type YandePost struct {
\tID        int    `json:"id"`
\tSampleURL string `json:"sample_url"`
\tFileURL   string `json:"file_url"`
\tTags      string `json:"tags"`
}

func StartYande(ctx context.Context, cfg *config.Config, db *database.D1Client, bot *telegram.BotHandler) {
\tclient := resty.New()
\t
\tfor {
\t\tselect {
\t\tcase <-ctx.Done():
\t\t\treturn
\t\tdefault:
\t\t\tlog.Println("🔍 Checking Yande...")
\t\t\turl := fmt.Sprintf("https://yande.re/post.json?limit=%d&tags=%s", cfg.YandeLimit, cfg.YandeTags)
\t\t\t
\t\t\tresp, err := client.R().Get(url)
\t\t\tif err != nil {
\t\t\t\tlog.Printf("Yande Error: %v", err)
\t\t\t\ttime.Sleep(1 * time.Minute)
\t\t\t\tcontinue
\t\t\t}

\t\t\tvar posts []YandePost
\t\t\tif err := json.Unmarshal(resp.Body(), &posts); err != nil {
\t\t\t\tlog.Printf("Yande JSON Error: %v", err)
\t\t\t\tcontinue
\t\t\t}

\t\t\tfor _, post := range posts {
\t\t\t\tpid := fmt.Sprintf("yande_%d", post.ID)
\t\t\t\t
\t\t\t\t// 简单的内存去重检查 (可选)
\t\t\t\tif db.History[pid] {
\t\t\t\t\tcontinue
\t\t\t\t}

\t\t\t\timgURL := post.SampleURL
\t\t\t\tif imgURL == "" {
\t\t\t\t\timgURL = post.FileURL
\t\t\t\t}

\t\t\t\tlog.Printf("Downloading Yande: %d", post.ID)
\t\t\t\timgResp, err := client.R().Get(imgURL)
\t\t\t\tif err != nil {
\t\t\t\t\tcontinue
\t\t\t\t}

\t\t\t\tcaption := fmt.Sprintf("Yande: %d
Tags: #%s", post.ID, strings.ReplaceAll(post.Tags, " ", " #"))
\t\t\t\t
\t\t\t\tbot.ProcessAndSend(ctx, imgResp.Body(), pid, post.Tags, caption, "yande")
\t\t\t\ttime.Sleep(2 * time.Second)
\t\t\t}

\t\t\tlog.Println("😴 Yande Done. Sleeping 10m...")
\t\t\ttime.Sleep(10 * time.Minute)
\t\t}
\t}
}