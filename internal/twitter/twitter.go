package twitter

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type Tweet struct {
	ID       string
	Text     string
	ImageURL string
	Width    int
	Height   int
}

// GetTweetWithCookie 尝试获取推文信息
func GetTweetWithCookie(url string, cookie string) (*Tweet, error) {
	// 1. 构造请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// 清理 Cookie，防止回车换行符
	cleanCookie := strings.TrimSpace(cookie)
	req.Header.Set("Cookie", cleanCookie)

	// ⚠️ 关键设置：使用真实浏览器 UA，配合有效 Cookie
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
	
	// 尝试自动提取 x-csrf-token (ct0)，这对通过 X 的 WAF 很重要
	if strings.Contains(cleanCookie, "ct0=") {
		parts := strings.Split(cleanCookie, "ct0=")
		if len(parts) > 1 {
			// 取 ct0= 后面直到分号的部分
			ct0 := strings.Split(parts[1], ";")[0]
			req.Header.Set("x-csrf-token", ct0)
		}
	}

	// 其它仿生 Header，增加成功率
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http status: %d", resp.StatusCode)
	}

	// 2. 解析 HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	// 3. 提取推文文本
	var text string
	// 优先尝试 og:description
	doc.Find("meta[property='og:description']").Each(func(i int, s *goquery.Selection) {
		if text == "" {
			text = s.AttrOr("content", "")
		}
	})
	// 备选: twitter:description
	if text == "" {
		doc.Find("meta[name='twitter:description']").Each(func(i int, s *goquery.Selection) {
			if text == "" {
				text = s.AttrOr("content", "")
			}
		})
	}

	// 4. 提取图片链接 (增强版)
	var imageURL string
	// 尝试 og:image
	doc.Find("meta[property='og:image']").Each(func(i int, s *goquery.Selection) {
		if imageURL == "" {
			imageURL = s.AttrOr("content", "")
		}
	})
	// 尝试 twitter:image
	if imageURL == "" {
		doc.Find("meta[name='twitter:image']").Each(func(i int, s *goquery.Selection) {
			if imageURL == "" {
				imageURL = s.AttrOr("content", "")
			}
		})
	}

	// 检查是否拿到了默认头像或者占位图，虽然可能不是大图，但总比没有好
	// 如果需要过滤头像，可以在这里加 if strings.Contains(imageURL, "profile_images") { ... }

	if imageURL == "" {
		// 调试信息：带上 Title 方便排查 403/Verify 情况
		title := doc.Find("title").Text()
		return nil, fmt.Errorf("no image found. Page Title: %s", strings.TrimSpace(title))
	}

	// 5. 提取尺寸
	var width, height int
	doc.Find("meta[property='og:image:width']").Each(func(i int, s *goquery.Selection) {
		if width == 0 {
			fmt.Sscanf(s.AttrOr("content", ""), "%d", &width)
		}
	})
	doc.Find("meta[property='og:image:height']").Each(func(i int, s *goquery.Selection) {
		if height == 0 {
			fmt.Sscanf(s.AttrOr("content", ""), "%d", &height)
		}
	})

	// 6. 提取 ID
	var id string
	re := regexp.MustCompile(`status/(\d+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) > 1 {
		id = matches[1]
	}

	return &Tweet{
		ID:       id,
		Text:     text,
		ImageURL: imageURL,
		Width:    width,
		Height:   height,
	}, nil
}

// DownloadImage 下载图片
func DownloadImage(imageURL string, cookie string) ([]byte, error) {
	if imageURL == "" {
		return nil, fmt.Errorf("imageURL is empty")
    }
    
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		return nil, err
	}
    
    // 下载图片通常不需要 Cookie，但为了防盗链检查，带上 UA
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36")
    // 有些图床需要 Referer
    req.Header.Set("Referer", "https://x.com/")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
