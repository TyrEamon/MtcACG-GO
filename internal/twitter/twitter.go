package twitter

import (
    "fmt"
    "io"
    "net/http"
    "regexp"

    "github.com/PuerkitoBio/goquery"
)

type Tweet struct {
    ID       string
    Text     string
    ImageURL string
    Width    int
    Height   int
}

func GetTweetWithCookie(url string, cookie string) (*Tweet, error) {
    // 1. 构造请求并带上 Cookie
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Cookie", cookie)
    // 带上一个正常浏览器 UA，成功率更高
    req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    // 2. 解析 HTML
    doc, err := goquery.NewDocumentFromReader(resp.Body)
    if err != nil {
        return nil, err
    }

    // 3. 提取推文文本（描述）
    var text string
    doc.Find("meta[property='og:description']").Each(func(i int, s *goquery.Selection) {
        if text == "" {
            text = s.AttrOr("content", "")
        }
    })

    // 4. 提取图片链接
    var imageURL string
    doc.Find("meta[property='og:image']").Each(func(i int, s *goquery.Selection) {
        if imageURL == "" {
            imageURL = s.AttrOr("content", "")
        }
    })

    // 如果解析不到图片，直接返回错误，避免后面 DownloadImage 传空串
    if imageURL == "" {
        return nil, fmt.Errorf("no og:image found, imageURL empty")
    }

    // 5. 提取图片尺寸（如果没有就保持 0）
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

    // 6. 从 URL 里提取推文 ID
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

func DownloadImage(imageURL string, cookie string) ([]byte, error) {
    if imageURL == "" {
        return nil, fmt.Errorf("imageURL is empty")
    }

    req, err := http.NewRequest("GET", imageURL, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("Cookie", cookie)
    req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    return io.ReadAll(resp.Body)
}
