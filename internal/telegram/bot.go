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
            // 默认不做任何事，防止多重触发
		}),
	}

	b, err := bot.New(cfg.BotToken, opts...)
	if err != nil {
		return nil, err
	}

	h := &BotHandler{API: b, Cfg: cfg, DB: db, Sessions: make(map[int64]*UserSession)}

    // ---------------------------------------------------------
    // ✅ 重新梳理 Handler 注册，防止冲突
    // ---------------------------------------------------------

	// 1. 优先处理按钮回调 (Callback Query)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "", bot.MatchTypePrefix, h.handleTagCallback)

	// 2. 注册具体指令 /save
	b.RegisterHandler(bot.HandlerTypeMessageText, "/save", bot.MatchTypeExact, h.handleSave)

	// 3. 统一消息入口：处理 图片 OR 文本回复 (/title, /no)
    //    使用 MatchTypePrefix + "" 匹配所有文本/图片消息，然后在函数内部判断
	b.RegisterHandler(bot.HandlerTypeMessageText, "", bot.MatchTypePrefix, h.handleMainRouter)

	return h, nil
}

func (h *BotHandler) Start(ctx context.Context) {
	h.API.Start(ctx)
}

// =====================================================================================
// ✅ 核心逻辑路由 (解决冲突的关键)
// =====================================================================================

// 统一路由：根据消息类型分发
func (h *BotHandler) handleMainRouter(ctx context.Context, b *bot.Bot, update *models.Update) {
    if update.Message == nil {
        return
    }

    // A. 如果是图片 -> 进入新图片处理流程
    if len(update.Message.Photo) > 0 {
        h.handleNewPhoto(ctx, b, update)
        return
    }

    // B. 如果是文本 -> 检查是否是指令回复
    if update.Message.Text != "" {
        h.handleTextReply(ctx, b, update)
        return
    }
}

// 处理新收到的图片
func (h *BotHandler) handleNewPhoto(ctx context.Context, b *bot.Bot, update *models.Update) {
	userID := update.Message.From.ID
	photo := update.Message.Photo[len(update.Message.Photo)-1]

	caption := update.Message.Caption
    
    // 🛠️ 修复多图逻辑：如果这只是多图中的一张且没标题，尽量不要覆盖掉正在进行的会话
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
		Text:   fmt.Sprintf("📩 收到图片了,Daishiki喵！\n\n当前标题：\n%s\n\n主人要自定义标题吗,喵？\n1️和我说 `/title` 就可以使用新标题了喵\n2️说 `/no` 那就只能使用原标题的说,喵", caption),
		ReplyParameters: &models.ReplyParameters{
			MessageID: update.Message.ID,
		},
	})
}

// 处理文本回复 (/title, /no)
func (h *BotHandler) handleTextReply(ctx context.Context, b *bot.Bot, update *models.Update) {
	userID := update.Message.From.ID
	session, exists := h.Sessions[userID]

	// 1. 如果没有会话，或者状态不对，说明用户可能在瞎聊，直接忽略
	if !exists || session.State != StateWaitingTitle {
		return
	}

	text := update.Message.Text

	if text == "/no" {
		// 用户确认使用原标题
	} else if strings.HasPrefix(text, "/title ") {
        // 用户修改标题
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

    // 状态流转 -> 等待标签
	session.State = StateWaitingTag

	kb := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "TG-SFW", CallbackData: "tag_sfw"},
				{Text: "TG-NSFW", CallbackData: "tag_nsfw"},
			},
		},
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      update.Message.Chat.ID,
		Text:        fmt.Sprintf("✅ 狗修金,标题确认好了喵~: \n%s\n\n请主人狠狠点击下方按钮选择标签,打上只属于主人的标记吧。：", session.Caption),
		ReplyMarkup: kb,
	})
}

// 处理按钮回调
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
		delete(h.Sessions, userID) // 上传完清除会话

		// ✅ MessageID 字段修复
		b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: update.CallbackQuery.Message.MessageID, 
			Text:      fmt.Sprintf("✅ 已处理: \n%s\n\nTags: %s", session.Caption, tag),
		})
	}

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
	})
}

// 核心上传逻辑
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

// =====================================================================================
// ✅ 补回被遗漏的公共方法 (ProcessAndSend, PushHistoryToCloud, compressImage)
// =====================================================================================

// ProcessAndSend 供爬虫模块调用
func (h *BotHandler) ProcessAndSend(ctx context.Context, imgData []byte, postID, tags, caption, source string, width, height int) {
	// 1. 先检查内存历史，如果有了就直接跳过
	if h.DB.History[postID] {
		log.Printf("⏭️ Skip %s: already in history", postID)
		return
	}

	// 2. 检查图片大小，如果超过 9MB 则压缩 (Telegram 限制 10MB)
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

	// 3. 发送到 Telegram
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

	// 4. 存入 D1 数据库
	err = h.DB.SaveImage(postID, fileID, caption, tags, source, width, height)
	if err != nil {
		log.Printf("❌ D1 Save Failed: %v", err)
	} else {
		log.Printf("✅ Saved: %s (%dx%d)", postID, width, height)
	}
}

// PushHistoryToCloud 供爬虫模块或手动调用
func (h *BotHandler) PushHistoryToCloud() {
	if h.DB != nil {
		h.DB.PushHistory()
	}
}

// handleSave 手动触发保存
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
