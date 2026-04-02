package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {
	url := flag.String("url", "http://localhost:8081/openai/v1/chat/completions", "API endpoint URL")
	model := flag.String("model", "local-rag", "Model to request")
	prompt := flag.String("prompt", "", "Prompt / message content")
	stream := flag.Bool("stream", true, "Use streaming (SSE)")
	flag.Parse()

	if *prompt == "" {
		fmt.Fprintln(os.Stderr, "prompt is required: -prompt \"text\"")
		os.Exit(2)
	}

	reqBody := map[string]interface{}{
		"model": *model,
		"messages": []map[string]string{{
			"role":    "user",
			"content": *prompt,
		}},
		"stream": *stream,
	}

	data, _ := json.Marshal(reqBody)
	resp, err := http.Post(*url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		fmt.Fprintln(os.Stderr, "request failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if !*stream {
		var out map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			fmt.Fprintln(os.Stderr, "decode error:", err)
			os.Exit(1)
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return
	}

	// streaming: read lines and handle "data: ..." events
	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintln(os.Stderr, "read error:", err)
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				break
			}
			// try decode JSON, otherwise print raw
			var obj any
			if err := json.Unmarshal([]byte(payload), &obj); err != nil {
				fmt.Println(payload)
				continue
			}
			// print human-friendly content if possible
			if m, ok := obj.(map[string]interface{}); ok {
				// try chat chunk structure
				if choices, found := m["choices"]; found {
					if arr, ok := choices.([]interface{}); ok && len(arr) > 0 {
						if first, ok := arr[0].(map[string]interface{}); ok {
							if delta, has := first["delta"]; has {
								if dmap, ok := delta.(map[string]interface{}); ok {
									if content, ok := dmap["content"].(string); ok {
										fmt.Print(content)
										continue
									}
								}
							}
							if msg, has := first["message"]; has {
								if mmsg, ok := msg.(map[string]interface{}); ok {
									if content, ok := mmsg["content"].(string); ok {
										fmt.Println(content)
										continue
									}
								}
							}
						}
					}
				}
			}
			// fallback: pretty print JSON
			b, _ := json.MarshalIndent(obj, "", "  ")
			fmt.Println(string(b))
		}
	}
}
