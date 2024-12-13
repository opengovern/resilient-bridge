// github_throttle.go

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

// GraphQLResponse is a minimal structure to parse the GraphQL JSON response.
// Adjust fields as needed to capture commits.
type GraphQLResponse struct {
	Data struct {
		Repository struct {
			Object struct {
				History struct {
					Edges []struct {
						Node struct {
							Message       string `json:"message"`
							CommittedDate string `json:"committedDate"`
							OID           string `json:"oid"`
							Author        struct {
								Name  string    `json:"name"`
								Email string    `json:"email"`
								Date  time.Time `json:"date"`
							} `json:"author"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"history"`
			} `json:"object"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func main() {
	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Fatal("GITHUB_API_TOKEN environment variable not set. It needs repo scope for GitHub GraphQL.")
	}

	sdk := resilientbridge.NewResilientBridge()
	// Enable debug for observing internal behavior
	sdk.SetDebug(true)

	// Register GitHub provider with defaults, using provider limits:
	sdk.RegisterProvider("github", adapters.NewGitHubAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       500 * time.Millisecond,
	})

	// Determine the date since last 7 days:
	sinceTime := time.Now().Add(-168 * time.Hour).UTC().Format(time.RFC3339)

	// GraphQL query for commits in last 7 days:
	query := fmt.Sprintf(`
query {
  repository(name: "opencomply", owner: "opengovern") {
    object(expression: "main") {
      ... on Commit {
        history(since:"%s") {
          edges {
            node {
              message
              committedDate
              author {
                name
                email
                date
              }
              oid
            }
          }
        }
      }
    }
  }
}`, sinceTime)

	// We'll make multiple workers to attempt many requests to test throttling:
	numWorkers := 20
	maxAttemptsPerWorker := 500 // total = 10 * 50 = 500 queries

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	done := make(chan struct{})
	var once sync.Once
	var got429 bool

	rand.Seed(time.Now().UnixNano())

	for w := 0; w < numWorkers; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			for attempt := 1; attempt <= maxAttemptsPerWorker; attempt++ {
				select {
				case <-done:
					return
				default:
				}

				// Small random sleep to add variability and avoid synchronized hits:
				sleepTime := time.Duration(rand.Intn(200)) * time.Millisecond
				fmt.Printf("[Worker %d][Attempt %d] Sleeping %v before GraphQL request.\n", workerID, attempt, sleepTime)
				time.Sleep(sleepTime)

				req := &resilientbridge.NormalizedRequest{
					Method: "POST",
					// GitHub GraphQL endpoint
					Endpoint: "/graphql",
					Headers: map[string]string{
						"Accept":       "application/vnd.github+json",
						"Content-Type": "application/json",
					},
					Body: []byte(fmt.Sprintf(`{"query": %q}`, query)),
				}

				resp, err := sdk.Request("github", req)
				if err != nil {
					// After all retries, request failed
					fmt.Printf("[Worker %d][Attempt %d] After retries, request failed: %v\n", workerID, attempt, err)
					return
				}

				if resp.StatusCode == 429 {
					once.Do(func() {
						got429 = true
						fmt.Printf("\n---\n[Worker %d][Attempt %d] Hit the rate limit (429) from provider!\n", workerID, attempt)
						printRateLimitInfo(sdk)
						fmt.Println("We will stop all requests now.")
						close(done)
					})
					return
				} else if resp.StatusCode >= 400 {
					fmt.Printf("[Worker %d][Attempt %d] Unexpected error %d: %s\n", workerID, attempt, resp.StatusCode, string(resp.Data))
					return
				}

				var gqlResp GraphQLResponse
				if err := json.Unmarshal(resp.Data, &gqlResp); err != nil {
					fmt.Printf("[Worker %d][Attempt %d] Error parsing GraphQL response: %v\n", workerID, attempt, err)
					return
				}

				if len(gqlResp.Errors) > 0 {
					fmt.Printf("[Worker %d][Attempt %d] GraphQL errors: %+v\n", workerID, attempt, gqlResp.Errors)
					return
				}

				// Print how many commits we got:
				commitCount := len(gqlResp.Data.Repository.Object.History.Edges)
				fmt.Printf("[Worker %d][Attempt %d] Found %d commits since %s\n", workerID, attempt, commitCount, sinceTime)
			}
		}()
	}

	wg.Wait()

	if !got429 {
		fmt.Println("Did not receive a 429 rate limit error from GitHub after many attempts.")
	} else {
		fmt.Println("Test completed, 429 rate limit error observed.")
	}
}

func printRateLimitInfo(sdk *resilientbridge.ResilientBridge) {
	info := sdk.GetRateLimitInfo("github")
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
