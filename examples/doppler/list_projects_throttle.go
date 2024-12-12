package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/opengovern/reslient-bridge/adapters"
)

type DopplerProjectsResponse struct {
	Projects []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"projects"`
	Page       int  `json:"page"`
	TotalPages int  `json:"total_pages"`
	Success    bool `json:"success"`
}

func main() {
	token := os.Getenv("YOUR_DOPPLER_API_TOKEN")
	if token == "" {
		log.Fatal("Environment variable YOUR_DOPPLER_API_TOKEN not set")
	}

	sdk := unifiedsdk.NewUnifiedSDK()
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, &unifiedsdk.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       200 * time.Millisecond,
	})

	rand.Seed(time.Now().UnixNano())

	numWorkers := 4
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	done := make(chan struct{})
	var once sync.Once
	var got429 bool

	for w := 0; w < numWorkers; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			attempt := 0
			// Each worker starts with a different base page offset to add variability
			pageOffset := workerID * 2
			for {
				select {
				case <-done:
					return
				default:
				}

				attempt++

				// Introduce variability: random sleep before each request
				sleepTime := time.Duration(rand.Intn(500)) * time.Millisecond
				fmt.Printf("[Worker %d][Attempt %d] Sleeping %v before request for variability.\n", workerID, attempt, sleepTime)
				time.Sleep(sleepTime)

				// Vary the page parameter slightly over time
				page := 1 + pageOffset + (attempt % 5) // cycles pages for each worker
				q := url.Values{}
				q.Set("page", fmt.Sprintf("%d", page))
				q.Set("per_page", "20")

				req := &unifiedsdk.NormalizedRequest{
					Method:   "GET",
					Endpoint: "/v3/projects?" + q.Encode(),
					Headers:  map[string]string{"accept": "application/json"},
				}

				resp, err := sdk.Request("doppler", req)
				if err != nil {
					fmt.Printf("[Worker %d][Attempt %d] After retries, request failed: %v\n", workerID, attempt, err)
					return
				}

				if resp.StatusCode == 429 {
					once.Do(func() {
						got429 = true
						fmt.Printf("\n---\n[Worker %d][Attempt %d] Hit the rate limit!\n", workerID, attempt)
						printRateLimitInfo(sdk)
						fmt.Println("The SDK applied exponential backoff and retries but we hit 429 anyway.")
						fmt.Println("Stopping all requests now.")
						close(done)
					})
					return
				} else if resp.StatusCode >= 400 {
					fmt.Printf("[Worker %d][Attempt %d] Unexpected error %d: %s\n", workerID, attempt, resp.StatusCode, string(resp.Data))
					return
				}

				var projectsResp DopplerProjectsResponse
				if err := json.Unmarshal(resp.Data, &projectsResp); err != nil {
					fmt.Printf("[Worker %d][Attempt %d] Error parsing projects response: %v\n", workerID, attempt, err)
					return
				}

				fmt.Printf("[Worker %d][Attempt %d] Page=%d, Projects=%d\n", workerID, attempt, projectsResp.Page, len(projectsResp.Projects))
			}
		}()
	}

	wg.Wait()

	if !got429 {
		fmt.Println("Did not receive 429 after many attempts.")
	} else {
		fmt.Println("Test completed, rate limit observed with variability in requests.")
	}
}

func printRateLimitInfo(sdk *unifiedsdk.UnifiedSDK) {
	info := sdk.GetRateLimitInfo("doppler")
	if info == nil {
		fmt.Println("[DEBUG] No rate limit info available.")
		return
	}

	fmt.Println("[DEBUG] Rate Limit Info:")
	fmt.Printf("- MaxRequests: %v\n", intOrNil(info.MaxRequests))
	fmt.Printf("- RemainingRequests: %v\n", intOrNil(info.RemainingRequests))
	fmt.Printf("- ResetRequestsAt: %v\n", timeOrNil(info.ResetRequestsAt))
	if info.ResetRequestsAt != nil {
		resetTime := time.UnixMilli(*info.ResetRequestsAt)
		fmt.Printf("[DEBUG] Requests reset at: %v\n", resetTime)
	}
}

// Helper functions
func intOrNil(i *int) interface{} {
	if i == nil {
		return nil
	}
	return *i
}

func timeOrNil(tms *int64) interface{} {
	if tms == nil {
		return nil
	}
	return time.UnixMilli(*tms)
}
