package config

import (
\t"log"
\t"os"
\t"strconv"
\t"strings"

\t"github.com/joho/godotenv"
)

type Config struct {
\tBotToken       string
\tChannelID      int64
\tCF_AccountID   string
\tCF_APIToken    string
\tD1_DatabaseID  string
\tWorkerURL      string
\tPixivPHPSESSID string
\tPixivLimit     int
\tYandeLimit     int
\tYandeTags      string
\tPixivArtistIDs []string
}

func Load() *Config {
\t// 尝试加载 .env 文件，如果不存在也没关系（生产环境直接读环境变量）
\t_ = godotenv.Load()

\tchannelIDStr := getEnv("CHANNEL_ID", "")
\tchannelID, err := strconv.ParseInt(channelIDStr, 10, 64)
\tif err != nil {
\t\tlog.Printf("⚠️ Warning: Invalid CHANNEL_ID: %v", err)
\t}

\tpixivLimit, _ := strconv.Atoi(getEnv("PIXIV_LIMIT", "3"))
\tyandeLimit, _ := strconv.Atoi(getEnv("YANDE_LIMIT", "1"))

\tartistIDsStr := getEnv("PIXIV_ARTIST_IDS", "")
\tvar artistIDs []string
\tif artistIDsStr != "" {
\t\t// 支持逗号或换行符分隔
\t\tparts := strings.FieldsFunc(artistIDsStr, func(r rune) bool {
\t\t\treturn r == ',' || r == '
'
\t\t})
\t\tfor _, p := range parts {
\t\t\tif strings.TrimSpace(p) != "" {
\t\t\t\tartistIDs = append(artistIDs, strings.TrimSpace(p))
\t\t\t}
\t\t}
\t}

\treturn &Config{
\t\tBotToken:       getEnv("BOT_TOKEN", ""),
\t\tChannelID:      channelID,
\t\tCF_AccountID:   getEnv("CLOUDFLARE_ACCOUNT_ID", ""),
\t\tCF_APIToken:    getEnv("CLOUDFLARE_API_TOKEN", ""),
\t\tD1_DatabaseID:  getEnv("D1_DATABASE_ID", ""),
\t\tWorkerURL:      getEnv("WORKER_URL", ""),
\t\tPixivPHPSESSID: getEnv("PIXIV_PHPSESSID", ""),
\t\tPixivLimit:     pixivLimit,
\t\tYandeLimit:     yandeLimit,
\t\tYandeTags:      getEnv("YANDE_TAGS", "order:random"),
\t\tPixivArtistIDs: artistIDs,
\t}
}

func getEnv(key, fallback string) string {
\tif value, exists := os.LookupEnv(key); exists {
\t\treturn value
\t}
\t// 兼容带空格的key或者旧的命名习惯
\tif value, exists := os.LookupEnv(strings.ReplaceAll(key, "_", " ")); exists {
\t\treturn value
\t}
\treturn fallback
}