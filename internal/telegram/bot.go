package telegram

import (
\t"bytes"
\t"context"
\t"fmt"
\t"log"
\t"my-bot-go/internal/config"
\t"my-bot-go/internal/database"

\t"github.com/go-telegram/bot"
\t"github.com/go-telegram/bot/models"
)

type BotHandler struct {
\tAPI *bot.Bot
\tCfg *config.Config
\tDB  *database.D1Client
}

func NewBot(cfg *config.Config, db *database.D1Client) (*BotHandler, error) {
\topts := []bot.Option{
\t\tbot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, update *models.Update) {
\t\t\t// 默认不做处理
\t\t}),
\t}

\tb, err := bot.New(cfg.BotToken, opts...)
\tif err != nil {
\t\treturn nil, err
\t}
\t
\th := &BotHandler{API: b, Cfg: cfg, DB: db}
\t
\t// 注册手动转发监听
\tb.RegisterHandler(bot.HandlerTypeMessageText, "", bot.MatchTypePrefix, h.handleManual)
\tb.RegisterHandler(bot.HandlerTypeMessageText, "", bot.MatchTypePrefix, func(ctx context.Context, b *bot.Bot, update *models.Update) {
\t\t// Fallback for photos if necessary, usually check update.Message.Photo
\t\tif update.Message != nil && len(update.Message.Photo) > 0 {
\t\t\th.handleManual(ctx, b, update)
\t\t}
\t})

\treturn h, nil
}

func (h *BotHandler) Start(ctx context.Context) {
\th.API.Start(ctx)
}

// ProcessAndSend 处理并发送图片
func (h *BotHandler) ProcessAndSend(ctx context.Context, imgData []byte, postID, tags, caption, source string) {
\t// 1. 发送图片
\tparams := &bot.SendPhotoParams{
\t\tChatID:  h.Cfg.ChannelID,
\t\tPhoto:   &models.InputFileUpload{Filename: source + ".jpg", Data: bytes.NewReader(imgData)},
\t\tCaption: caption,
\t}

\tmsg, err := h.API.SendPhoto(ctx, params)
\tif err != nil {
\t\tlog.Printf("❌ Telegram Send Failed [%s]: %v", postID, err)
\t\treturn
\t}

\t// 2. 获取 FileID (取最大尺寸)
\tif len(msg.Photo) == 0 {
\t\treturn 
\t}
\tfileID := msg.Photo[len(msg.Photo)-1].FileID

\t// 3. 存库
\terr = h.DB.SaveImage(postID, fileID, caption, tags, source)
\tif err != nil {
\t\tlog.Printf("❌ D1 Save Failed: %v", err)
\t} else {
\t\tlog.Printf("✅ Saved: %s", postID)
\t}
}

func (h *BotHandler) handleManual(ctx context.Context, b *bot.Bot, update *models.Update) {
\tif update.Message == nil || len(update.Message.Photo) == 0 {
\t\treturn
\t}
\t
\tphoto := update.Message.Photo[len(update.Message.Photo)-1]
\tpostID := fmt.Sprintf("manual_%d", update.Message.ID)
\tcaption := update.Message.Caption
\tif caption == "" {
\t\tcaption = "Forwarded Image"
\t}

\t// 既然已经是 TG 图片，直接用 FileID 发送
\tmsg, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
\t\tChatID:  h.Cfg.ChannelID,
\t\tPhoto:   &models.InputFileString{Data: photo.FileID},
\t\tCaption: caption,
\t})
\t
\tif err != nil {
\t\tb.SendMessage(ctx, &bot.SendMessageParams{
\t\t\tChatID: update.Message.Chat.ID,
\t\t\tText:   "❌ Forward failed: " + err.Error(),
\t\t})
\t\treturn
\t}

\tfinalFileID := msg.Photo[len(msg.Photo)-1].FileID
\th.DB.SaveImage(postID, finalFileID, caption, "manual forwarded", "manual")
\t
\tb.SendMessage(ctx, &bot.SendMessageParams{
\t\tChatID:           update.Message.Chat.ID,
\t\tText:             "✅ Saved to D1!",
\t\tReplyToMessageID: update.Message.ID,
\t})
}