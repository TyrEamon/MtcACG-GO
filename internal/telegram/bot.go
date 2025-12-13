package telegram

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log"
	"strings"

	"my-bot-go/internal/config"
	"my-bot-go/internal/database"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// 状态常量
const (
	StateNone = iota
	StateWaitingTitle    // 等待用户确认标题
	StateWaitingTag      // 等待用户选择标签
)

// 用户会话
type UserSession struct {
	State       int
	PhotoFileID string
	Width       int
	Height      int
	Caption     string
	MessageID   int
}

type BotHandler struct {
	API      *bot.Bot
	Cfg      *config.Config
	DB       *database.D1Client
	Sessions map[int64]*UserSession
}

func NewBot(cfg *config.Config, db *database.D1Client) (*BotHandler, error) {
	opts := []bot.Option{
		bot.WithDefaultHandler(func(ctx context.Context, b *bot.Bot, update *models.Update) {
		}),
	}

	b, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		return nil, err
	}

	h := &BotHandler{API: b, Cfg: cfg, DB: db, Sessions: make(map[int64]*UserSession)}

	// 注册 /save 命令
	b.RegisterHandler(bot.HandlerTypeMessageText, "/save", bot.MatchTypeExact, h.handleSave)

	// 监听所有文本消息
	b.RegisterHandler(bot.HandlerTypeMessageText, "", bot.MatchTypePrefix, h.handleTextReply)

	// ✅ 监听按钮回调
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "", bot.MatchTypePrefix, h.handleTagCallback)

	// 其他 Handlers
	b.RegisterHandler(bot.HandlerTypeMessageText, "", bot.MatchTypePrefix, h.handleManual)
	b.RegisterHandler(bot.HandlerTypeMessageText, "", bot.MatchTypePrefix, func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message != nil && len(update.Message.Photo) > 0 {
			h.handleManual(ctx, b, update)
		}
	})

	return h, nil
}

func (h *BotHandler) Start(ctx context.Context) {
	h.API.Start(ctx)
}

// ProcessAndSend
func (h *BotHandler) ProcessAndSend(ctx context.Context, imgData []byte, postID, tags, caption, source string, width, height int) {
	if h.DB.History[postID] {
		log.Printf("⏭️ Skip %s: already in history", postID)
		return
	}

	const MaxPhotoSize = 9 * 1024 * 1024
	finalData := imgData

	if int64(len(imgData)) > MaxPhotoSize {
		log.Printf("⚠️ Image %s is too large (%.2f MB), compressing...", postID, float64(len(imgData))/1024/1024)
		compressed, err := compressImage(imgData, MaxPhotoSize)
		if err != nil {
			log.Printf("❌ Compression failed: %v. Trying original...", err)
		} else {
			finalData = compressed
		}
	}

	params := &bot.SendPhotoParams{
		ChatID:  h.Cfg.ChannelID,
		Photo:   &models.InputFileUpload{Filename: source + ".jpg", Data: bytes.NewReader(finalData)},
		Caption: caption,
	}

	msg, err := h.API.SendPhoto(ctx, params)
	if err != nil {
		log.Printf("❌ Telegram Send Failed [%s]: %v", postID, err)
		return
	}

	if len(msg.Photo) == 0 {
		return
	}
	fileID := msg.Photo[len(msg.Photo)-1].FileID

	err = h.DB.SaveImage(postID, fileID, caption, tags, source, width, height)
	if err != nil {
		log.Printf("❌ D1 Save Failed: %v", err)
	} else {
		log.Printf("✅ Saved: %s (%dx%d)", postID, width, height)
	}
}

func (h *BotHandler) PushHistoryToCloud() {
	if h.DB != nil {
		h.DB.PushHistory()
	}
}

func (h *BotHandler) handleSave(ctx context.Context, b *bot.Bot, update *models.Update) {
	userID := update.Message.From.ID

	if userID != 8040798522 && userID != 6874581126 {
		log.Printf("⛔ Unauthorized /save attempt from UserID: %d", userID)
		return
	}

	log.Printf("💾 Manual save triggered by UserID: %d", userID)

	if h.DB != nil {
		h.DB.PushHistory()
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "✅ History successfully saved to Cloudflare D1!",
		})
	} else {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Database client is not initialized.",
		})
	}
}

func (h *BotHandler) handleManual(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || len(update.Message.Photo) == 0 {
		return
	}
	userID := update.Message.From.ID
	photo := update.Message.Photo[len(update.Message.Photo)-1]

	caption := update.Message.Caption
	if caption == "" {
		caption = "MtcACG:TG"
	}

	h.Sessions[userID] = &UserSession{
		State:       StateWaitingTitle,
		PhotoFileID: photo.FileID,
		Width:       photo.Width,
		Height:      photo.Height,
		Caption:     caption,
		MessageID:   update.Message.ID,
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   fmt.Sprintf("📩 收到图片了,Daishiki喵！\n\n当前标题：\n%s\n\n主人要自定义标题吗,喵？\n1️和我说 `/title 就可以使用新标题了喵`\n2️说 `/no` 那就只能使用原标题的说,喵", caption),
		ReplyParameters: &models.ReplyParameters{
			MessageID: update.Message.ID,
		},
	})
}

func (h *BotHandler) handleTextReply(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	userID := update.Message.From.ID
	session, exists := h.Sessions[userID]

	if !exists || session.State == StateNone {
		return
	}

	text := update.Message.Text

	if text == "/no" {
		// 使用默认标题
	} else if strings.HasPrefix(text, "/title ") {
		newTitle := strings.TrimSpace(strings.TrimPrefix(text, "/title "))
		if newTitle != "" {
			session.Caption = newTitle
		} else {
			b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: update.Message.Chat.ID,
				Text:   "⚠️ 标题不能为空啊喵，请重新跟我说说吧 `/title 你的标题`",
			})
			return
		}
	} else {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "⚠️ 格式错误,喵~！\n- 确认原标题请回复 `/no`喵~\n- 自定义标题请回复 `/title 新标题`喵~",
		})
		return
	}

	session.State = StateWaitingTag

	kb := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "TG-SFW", CallbackData: "tag_sfw"},
				{Text: "TG-NSFW", CallbackData: "tag_nsfw"},
			},
		},
	}

	// ✅ 已修复隐患：去掉了 ParseMode，防止特殊字符报错
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        fmt.Sprintf("✅ 狗修金,标题确认好了喵~: \n%s\n\n请主人狠狠点击下方按钮选择标签,打上只属于主人的标记吧。：", session.Caption),
		ReplyMarkup: kb,
	})
}

func (h *BotHandler) handleTagCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	userID := update.CallbackQuery.From.ID
	session, exists := h.Sessions[userID]

	if !exists || session.State != StateWaitingTag {
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: update.CallbackQuery.ID,
			Text:            "⚠️ 哎哟,会话已过期，请重新转发图片,喵~。",
		})
		return
	}

	data := update.CallbackQuery.Data
	tag := ""
	if data == "tag_sfw" {
		tag = "#TGC #SFW"
	} else if data == "tag_nsfw" {
		tag = "#TGC #NSFW #R18"
	}

	if tag != "" {
		chatID := update.CallbackQuery.Message.Chat.ID

		h.processForwardUpload(ctx, b, chatID, session, tag)
		delete(h.Sessions, userID)

		// ✅ 核心修复：Message.ID 改为 Message.MessageID
		// ✅ 已修复隐患：去掉了 ParseMode
		b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: update.CallbackQuery.Message.MessageID, // 👈 这一行是报错的关键修复
			Text:      fmt.Sprintf("✅ 已处理: \n%s\n\nTags: %s", session.Caption, tag),
		})
	}

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

func (h *BotHandler) processForwardUpload(ctx context.Context, b *bot.Bot, chatID int64, session *UserSession, tag string) {
	msg, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:  h.Cfg.ChannelID,
		Photo:   &models.InputFileString{Data: session.PhotoFileID},
		Caption: fmt.Sprintf("%s\nTags: %s", session.Caption, tag),
	})

	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ 发送失败，喵~ (" + err.Error() + ")",
		})
		return
	}

	postID := fmt.Sprintf("manual_%d", msg.ID)
	finalFileID := msg.Photo[len(msg.Photo)-1].FileID

	err = h.DB.SaveImage(postID, finalFileID, session.Caption, tag, "manual", session.Width, session.Height)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ 图片已发频道，但数据库保存失败，喵~",
		})
	} else {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "上传成功，喵~ 🐱",
			ReplyParameters: &models.ReplyParameters{
				MessageID: session.MessageID,
			},
		})
	}
}

func compressImage(data []byte, targetSize int64) ([]byte, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode error: %v", err)
	}
	log.Printf("📉 Compressing %s image...", format)

	quality := 98
	for {
		buf := new(bytes.Buffer)
		err = jpeg.Encode(buf, img, &jpeg.Options{Quality: quality})
		if err != nil {
			return nil, fmt.Errorf("encode error: %v", err)
		}

		compressedData := buf.Bytes()
		size := int64(len(compressedData))

		if size <= targetSize || quality <= 40 {
			log.Printf("✅ Compressed to %.2f MB (Quality: %d)", float64(size)/1024/1024, quality)
			return compressedData, nil
		}
		quality -= 5
	}
}
