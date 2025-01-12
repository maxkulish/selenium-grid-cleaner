// internal/downloader/downloader.go
package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Status struct {
        Value struct {
                Nodes []struct {
                        SessionId string `json:"sessionId"`
                        StartTime int64  `json:"startTime"`
                } `json:"nodes"`
        } `json:"value"`
}

func DownloadStatus(url string) (*Status, error) {
        resp, err := http.Get(url)
        if err != nil {
                return nil, fmt.Errorf("http get error: %w", err)
        }
        defer resp.Body.Close()

        body, err := io.ReadAll(resp.Body)
        if err != nil {
                return nil, fmt.Errorf("read response body error: %w", err)
        }

        var status Status
        if err := json.Unmarshal(body, &status); err != nil {
                return nil, fmt.Errorf("unmarshal json error: %w", err)
        }

        return &status, nil
}
