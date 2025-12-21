package pixiv

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// 数据结构定义 (跟 crawler 里的一样，但独立出来)
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
		IllustId    string `json:"illustId"`
		IllustTitle string `json:"illustTitle"`
		UserName    string `json:"userName"`
		IllustType  int    `json:"illustType"` // 2=动图
		Tags        struct {
			Tags []struct {
				Tag string `json:"tag"`
			} `json:"tags"`
		} `json:"tags"`
	} `json:"body"`
}

// 统一的结构体，返回给 Bot 使用
type Illust struct {
	ID       string
	Title    string
	Artist   string
	Tags     string
	Pages    []PixivPage
}

// GetIllust 抓取单作品详情
func GetIllust(id string, cookie string) (*Illust, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. 获取详情 (标题、Tags)
	reqDetail, _ := http.NewRequest("GET", fmt.Sprintf("https://www.pixiv.net/ajax/illust/%s", id), nil)
	reqDetail.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	reqDetail.Header.Set("Cookie", "PHPSESSID="+cookie)
	
	respDetail, err := client.Do(reqDetail)
	if err != nil {
		return nil, err
	}
	defer respDetail.Body.Close()

	var detail PixivDetailResp
	if err := json.NewDecoder(respDetail.Body).Decode(&detail); err != nil {
		return nil, err
	}

	// 2. 获取图片列表 (Pages)
	reqPages, _ := http.NewRequest("GET", fmt.Sprintf("https://www.pixiv.net/ajax/illust/%s/pages?lang=zh", id), nil)
	reqPages.Header.Set("User-Agent", "Mozilla/5.0")
	reqPages.Header.Set("Cookie", "PHPSESSID="+cookie)
	reqPages.Header.Set("Referer", "https://www.pixiv.net/artworks/"+id)

	respPages, err := client.Do(reqPages)
	if err != nil {
		return nil, err
	}
	defer respPages.Body.Close()

	var pages PixivPagesResp
	if err := json.NewDecoder(respPages.Body).Decode(&pages); err != nil {
		return nil, err
	}

	// 拼装 Tags
	var tagStrs []string
	for _, t := range detail.Body.Tags.Tags {
		tagStrs = append(tagStrs, t.Tag)
	}
	
	return &Illust{
		ID:     detail.Body.IllustId,
		Title:  detail.Body.IllustTitle,
		Artist: detail.Body.UserName,
		Tags:   strings.Join(tagStrs, " "),
		Pages:  pages.Body,
	}, nil
}

// DownloadImage 下载图片数据 (带 Referer 防盗链)
func DownloadImage(url string, cookie string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://www.pixiv.net/")
	req.Header.Set("Cookie", "PHPSESSID="+cookie)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	
	return io.ReadAll(resp.Body)
}
