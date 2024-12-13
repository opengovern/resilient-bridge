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

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

// DopplerProjectsResponse represents the JSON structure returned by Doppler for listing projects.
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

	// Initialize the SDK
	sdk := resilientbridge.NewResilientBridge()

	// You can enable debug to see internal decision making, but it can be very verbose:
	sdk.SetDebug(true)

	// Register the Doppler provider, using provider limits and enabling retries with backoff.
	sdk.RegisterProvider("doppler", &adapters.DopplerAdapter{APIToken: token}, &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       200 * time.Millisecond,
	})

	rand.Seed(time.Now().UnixNano())

	numWorkers := 20
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	done := make(chan struct{})
	var once sync.Once
	var got429 bool

	// We want to make a large number of requests. Let's say each worker tries 150 attempts.
	// Total requests = numWorkers * 150
	maxAttemptsPerWorker := 150

	for w := 0; w < numWorkers; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			attempt := 0
			// Each worker starts with a different base page offset to add variability
			pageOffset := workerID * 2

			for attempt < maxAttemptsPerWorker {
				select {
				case <-done:
					return
				default:
					// continue
				}

				attempt++

				// Introduce variability in timing
				sleepTime := time.Duration(rand.Intn(500)) * time.Millisecond
				fmt.Printf("[Worker %d][Attempt %d] Sleeping %v for variability before request.\n", workerID, attempt, sleepTime)
				time.Sleep(sleepTime)

				// Vary the page parameter slightly over time
				page := 1 + pageOffset + (attempt % 5) // cycles pages for each worker
				q := url.Values{}
				q.Set("page", fmt.Sprintf("%d", page))
				q.Set("per_page", "20")

				req := &resilientbridge.NormalizedRequest{
					Method:   "GET",
					Endpoint: "/v3/projects?" + q.Encode(),
					Headers:  map[string]string{"accept": "application/json"},
				}

				resp, err := sdk.Request("doppler", req)
				if err != nil {
					// If we failed after all retries, print error and stop this worker
					fmt.Printf("[Worker %d][Attempt %d] After retries, request failed: %v\n", workerID, attempt, err)
					return
				}

				if resp.StatusCode == 429 {
					// Rate limit hit from actual provider
					once.Do(func() {
						got429 = true
						fmt.Printf("\n---\n[Worker %d][Attempt %d] Hit the rate limit!\n", workerID, attempt)
						printRateLimitInfo(sdk)
						fmt.Println("The SDK applied retries with backoff, but we still hit 429 from the provider.")
						fmt.Println("Stopping all requests now.")
						close(done)
					})
					return
				} else if resp.StatusCode >= 400 {
					fmt.Printf("[Worker %d][Attempt %d] Unexpected error %d: %s\n", workerID, attempt, resp.StatusCode, string(resp.Data))
					return
				}

				// Parse the response
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
		fmt.Println("Did not receive a 429 rate limit error from the provider after many attempts.")
	} else {
		fmt.Println("Test completed, a 429 rate limit error was observed from the provider and handled.")
	}
}

func printRateLimitInfo(sdk *resilientbridge.ResilientBridge) {
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
