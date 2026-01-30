package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"orchids-api/internal/prompt"
)

// Config holds CLI flags
type Config struct {
	BaseURL string
	Model   string
	Stream  bool
	Verbose bool
}

// TestCase defines a single test scenario
type TestCase struct {
	Name        string
	Input       string
	ExpectTool  bool
	ExpectThink bool // Checking if response contains "thinking" blocks or logic
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.BaseURL, "url", "http://localhost:8080/v1/messages", "API Endpoint URL")
	flag.StringVar(&cfg.Model, "model", "claude-3-5-sonnet-20241022", "Model to use")
	flag.BoolVar(&cfg.Stream, "stream", true, "Enable streaming")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Verbose output")
	flag.Parse()

	tests := []TestCase{
		{
			Name:  "Basic Greeting",
			Input: "Hello, say hi locally.",
		},
		{
			Name:        "Simple Math (Thinking Check)",
			Input:       "Calculate 25 * 42 step by step.",
			ExpectThink: true,
		},
		{
			Name:       "Tool Use (ls)",
			Input:      "List the files in the current directory.",
			ExpectTool: true,
		},
	}

	fmt.Printf("Starting QA Tester against %s (Model: %s)\n", cfg.BaseURL, cfg.Model)
	fmt.Println("--------------------------------------------------")

	for _, tc := range tests {
		runTest(cfg, tc)
		fmt.Println("--------------------------------------------------")
	}
}

func runTest(cfg Config, tc TestCase) {
	fmt.Printf("TEST: %s\n", tc.Name)
	fmt.Printf("INPUT: %s\n", tc.Input)

	reqBody := map[string]interface{}{
		"model": cfg.Model,
		"messages": []prompt.Message{
			{
				Role: "user",
				Content: prompt.MessageContent{
					Text: tc.Input,
				},
			},
		},
		"stream": cfg.Stream,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("Error marshalling request: %v\n", err)
		return
	}

	start := time.Now()
	resp, err := http.Post(cfg.BaseURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Network Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("HTTP Error %d: %s\n", resp.StatusCode, string(body))
		return
	}

	fmt.Print("OUTPUT: ")

	// Collect metrics
	var fullText strings.Builder
	hasThinking := false
	hasTool := false

	if cfg.Stream {
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					log.Printf("\nStream Error: %v", err)
				}
				break
			}
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				if data == "[DONE]" {
					break
				}
				var event map[string]interface{}
				if err := json.Unmarshal([]byte(data), &event); err == nil {
					// Parse standard Claude API SSE format
					if evtType, ok := event["type"].(string); ok {
						switch evtType {
						case "content_block_start":
							if blk, ok := event["content_block"].(map[string]interface{}); ok {
								if blk["type"] == "thinking" {
									hasThinking = true
									if cfg.Verbose {
										fmt.Print("\n[THINKING START]\n")
									}
								} else if blk["type"] == "tool_use" {
									hasTool = true
									if toolName, ok := blk["name"].(string); ok {
										fmt.Printf("\n[TOOL USE: %s]\n", toolName)
									}
								}
							}
						case "content_block_delta":
							if delta, ok := event["delta"].(map[string]interface{}); ok {
								if delta["type"] == "text_delta" {
									if text, ok := delta["text"].(string); ok {
										fullText.WriteString(text)
										fmt.Print(text)
									}
								} else if delta["type"] == "thinking_delta" {
									// Typically we might suppress thinking in output unless verbose
									if cfg.Verbose {
										if text, ok := delta["thinking"].(string); ok {
											fmt.Print(text)
										}
									}
								}
							}
						case "message_stop":
							// done
						}
					}
				}
			}
		}
	} else {
		// Non-stream
		var respData map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
			log.Printf("Decode Error: %v", err)
			return
		}
		if content, ok := respData["content"].([]interface{}); ok {
			for _, item := range content {
				if cmap, ok := item.(map[string]interface{}); ok {
					if cmap["type"] == "text" {
						if text, ok := cmap["text"].(string); ok {
							fullText.WriteString(text)
							fmt.Print(text)
						}
					} else if cmap["type"] == "tool_use" {
						hasTool = true
						if name, ok := cmap["name"].(string); ok {
							fmt.Printf("\n[TOOL USE: %s]", name)
						}
					}
				}
			}
		}
	}
	fmt.Println()
	fmt.Printf("STATS: Duration=%v", time.Since(start))
	if tc.ExpectThink {
		if hasThinking {
			fmt.Print(" | Thinking: CHECKED")
		} else {
			// Note: stream handler might suppress thinking unless requested or raw model supports it
			// This is just informational for now
			fmt.Print(" | Thinking: Not Observed")
		}
	}
	if tc.ExpectTool {
		if hasTool {
			fmt.Print(" | Tool Use: CHECKED")
		} else {
			fmt.Print(" | Tool Use: FAILED (?)")
		}
	}
	fmt.Println()
}
