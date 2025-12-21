package manyacg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	Artist      string   `json:"artist"`
	Tags        []string `json:"tags"`
	Pictures    []struct {
		ID       string `json:"id"`
		FileName string `json:"file_name"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
	} `json:"pictures"`
}

// GetArtworkInfo 通过 ManyACG artwork 链接获取作品信息
func GetArtworkInfo(artworkURL string) (*ArtworkInfo, error) {
	// 提取 artwork id
	re := regexp.MustCompile(`artwork/([a-zA-Z0-9]+)`)
	matches := re.FindStringSubmatch(artworkURL)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid ManyACG artwork URL")
	}
	artworkID := matches[1]

	// 请求 API
	url := fmt.Sprintf("https://api.manyacg.top/v1/artwork/%s", artworkID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Status int           `json:"status"`
		Data   ArtworkInfo   `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Status != 5 {
		return nil, fmt.Errorf("API returned error: %d", result.Status)
	}

	return &result.Data, nil
}

// DownloadPreview 下载预览图（regular）
func DownloadPreview(ctx context.Context, pictureID string) ([]byte, error) {
	url := fmt.Sprintf("https://cdn.manyacg.top/twitter/4468925295/%s_regular.webp", pictureID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// DownloadOriginal 下载原图
func DownloadOriginal(ctx context.Context, pictureID string) ([]byte, error) {
	url := fmt.Sprintf("https://api.manyacg.top/v1/picture/file/%s", pictureID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// FormatTags 将标签数组转换为 #tag 字符串
func FormatTags(tags []string) string {
	var sb strings.Builder
	for _, tag := range tags {
		sb.WriteString("#")
		sb.WriteString(tag)
		sb.WriteString(" ")
	}
	return strings.TrimSpace(sb.String())
}

// Example Usage (for testing)
func Example() {
	info, err := GetArtworkInfo("https://manyacg.top/artwork/6936cb9106ba014081ff1116")
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	log.Printf("Title: %s", info.Title)
	log.Printf("Artist: %s", info.Artist)
	log.Printf("Tags: %s", FormatTags(info.Tags))
	log.Printf("Source: %s", info.SourceURL)

	// 下载第一张图的原图
	if len(info.Pictures) > 0 {
		originalData, err := DownloadOriginal(context.Background(), info.Pictures[0].ID)
		if err != nil {
			log.Printf("Download failed: %v", err)
			return
		}
		log.Printf("Downloaded original image: %d bytes", len(originalData))
	}
}
