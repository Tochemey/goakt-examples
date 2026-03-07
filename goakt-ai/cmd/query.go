// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

var (
	queryEndpoint  string
	queryProvider  string
	queryOpenAI    string
	queryAnthropic string
	queryGoogle    string
	queryMistral   string
)

var queryCmd = &cobra.Command{
	Use:   "query [question]",
	Short: "Submit a query to the goakt-ai cluster",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		endpoint := queryEndpoint
		if endpoint == "" {
			endpoint = os.Getenv("GOAKT_AI_ENDPOINT")
		}
		if endpoint == "" {
			endpoint = "http://localhost:8080"
		}

		reqBody := map[string]string{"query": args[0]}
		body, _ := json.Marshal(reqBody)

		resp, err := http.Post(endpoint+"/query", "application/json", bytes.NewReader(body))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		var result struct {
			SessionID string `json:"session_id"`
			Result    string `json:"result"`
			Error     string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			fmt.Fprintf(os.Stderr, "Error decoding response: %v\n", err)
			os.Exit(1)
		}

		if result.Error != "" {
			fmt.Fprintf(os.Stderr, "Error: %s\n", result.Error)
			os.Exit(1)
		}

		fmt.Println(result.Result)
	},
}

func init() {
	rootCmd.AddCommand(queryCmd)
	queryCmd.Flags().StringVar(&queryEndpoint, "endpoint", "", "Load balancer endpoint (default: http://localhost:8080)")
	queryCmd.Flags().StringVar(&queryProvider, "provider", "", "LLM provider (openai|anthropic|google|mistral)")
	queryCmd.Flags().StringVar(&queryOpenAI, "openai-key", "", "OpenAI API key")
	queryCmd.Flags().StringVar(&queryAnthropic, "anthropic-key", "", "Anthropic API key")
	queryCmd.Flags().StringVar(&queryGoogle, "google-key", "", "Google API key")
	queryCmd.Flags().StringVar(&queryMistral, "mistral-key", "", "Mistral API key")
}
