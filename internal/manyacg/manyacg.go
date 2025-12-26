package manyacg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// ArtworkInfo 存储从 ManyACG 爬取的结构化信息
type ArtworkInfo struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	SourceURL   string   `json:"source_url"`
	R18         bool     `json:"r18"`
	Tags        []string `json:"tags"`
	Artist      struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		UID      string `json:"uid"`
		Username string `json:"username"`
	} `json:"artist"`
	Pictures []struct {
		ID       string `json:"id"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Index    int    `json:"index"`
		FileName string `json:"file_name"`
		Regular  string `json:"regular"` 
	} `json:"pictures"`
}

//ManyACG artwork 链接获取作品信息
func GetArtworkInfo(artworkURL string) (*ArtworkInfo, error) {
	re := regexp.MustCompile(`artwork/([a-zA-Z0-9]+)`)
	matches := re.FindStringSubmatch(artworkURL)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid ManyACG artwork URL")
	}
	artworkID := matches[1]

	//请求 API
	url := fmt.Sprintf("https://api.manyacg.top/v1/artwork/%s", artworkID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// 定义响应结构
	var result struct {
		Status  int         `json:"status"`
		Message string      `json:"message"`
		Data    ArtworkInfo `json:"data"` // 这里假设详情页返回的是单对象
	}

	// 读取 Body 用于调试（可选，如果报错可以打开）
	// bodyBytes, _ := io.ReadAll(resp.Body)
	// fmt.Println(string(bodyBytes))
	// resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode error: %v", err)
	}

	// ManyACG 成功状态码通常是 200 或 500
	if result.Data.ID == "" {
		return nil, fmt.Errorf("API returned error or empty data: %s", result.Message)
	}

	return &result.Data, nil
}

//下载原图
func DownloadOriginal(ctx context.Context, pictureID string) ([]byte, error) {
	url := fmt.Sprintf("https://api.manyacg.top/v1/picture/file/%s", pictureID)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// FormatTags 将标签数组转换为 #tag 字符串，并去重
func FormatTags(tags []string) string {
	seen := make(map[string]bool)
	var sb strings.Builder
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		tag = strings.ReplaceAll(tag, " ", "_")
		
		if !seen[tag] {
			sb.WriteString("#")
			sb.WriteString(tag)
			sb.WriteString(" ")
			seen[tag] = true
		}
	}
	return strings.TrimSpace(sb.String())
}
