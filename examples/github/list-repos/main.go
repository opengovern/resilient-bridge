package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	resilientbridge "github.com/opengovern/resilient-bridge"
	"github.com/opengovern/resilient-bridge/adapters"
)

type UserOrOrg struct {
	Login        string `json:"login"`
	ID           int    `json:"id,omitempty"`
	NodeID       string `json:"node_id,omitempty"`
	HTMLURL      string `json:"html_url,omitempty"`
	Type         string `json:"type,omitempty"`
	UserViewType string `json:"user_view_type,omitempty"`
	SiteAdmin    bool   `json:"site_admin,omitempty"`
}

type License struct {
	Key    string `json:"key,omitempty"`
	Name   string `json:"name,omitempty"`
	SPDXID string `json:"spdx_id,omitempty"`
	URL    string `json:"url,omitempty"`
	NodeID string `json:"node_id,omitempty"`
}

type Permissions struct {
	Admin    bool `json:"admin,omitempty"`
	Maintain bool `json:"maintain,omitempty"`
	Push     bool `json:"push,omitempty"`
	Triage   bool `json:"triage,omitempty"`
	Pull     bool `json:"pull,omitempty"`
}

type StatusObj struct {
	Status string `json:"status"`
}

type SecurityAndAnalysis struct {
	SecretScanning                    *StatusObj `json:"secret_scanning,omitempty"`
	SecretScanningPushProtection      *StatusObj `json:"secret_scanning_push_protection,omitempty"`
	DependabotSecurityUpdates         *StatusObj `json:"dependabot_security_updates,omitempty"`
	SecretScanningNonProviderPatterns *StatusObj `json:"secret_scanning_non_provider_patterns,omitempty"`
	SecretScanningValidityChecks      *StatusObj `json:"secret_scanning_validity_checks,omitempty"`
}

type RepoDetail struct {
	ID                        int                    `json:"id,omitempty"`
	NodeID                    string                 `json:"node_id,omitempty"`
	Name                      string                 `json:"name,omitempty"`
	FullName                  string                 `json:"full_name,omitempty"`
	Private                   bool                   `json:"private,omitempty"`
	Owner                     *UserOrOrg             `json:"owner,omitempty"`
	HTMLURL                   string                 `json:"html_url,omitempty"`
	Description               *string                `json:"description"`
	Fork                      bool                   `json:"fork,omitempty"`
	CreatedAt                 string                 `json:"created_at,omitempty"`
	UpdatedAt                 string                 `json:"updated_at,omitempty"`
	PushedAt                  string                 `json:"pushed_at,omitempty"`
	GitURL                    string                 `json:"git_url,omitempty"`
	SSHURL                    string                 `json:"ssh_url,omitempty"`
	CloneURL                  string                 `json:"clone_url,omitempty"`
	SVNURL                    string                 `json:"svn_url,omitempty"`
	Homepage                  *string                `json:"homepage"`
	Size                      int                    `json:"size,omitempty"`
	StargazersCount           int                    `json:"stargazers_count,omitempty"`
	WatchersCount             int                    `json:"watchers_count,omitempty"`
	Language                  *string                `json:"language"`
	HasIssues                 bool                   `json:"has_issues,omitempty"`
	HasProjects               bool                   `json:"has_projects,omitempty"`
	HasDownloads              bool                   `json:"has_downloads,omitempty"`
	HasWiki                   bool                   `json:"has_wiki,omitempty"`
	HasPages                  bool                   `json:"has_pages,omitempty"`
	HasDiscussions            bool                   `json:"has_discussions,omitempty"`
	ForksCount                int                    `json:"forks_count,omitempty"`
	MirrorURL                 *string                `json:"mirror_url"`
	Archived                  bool                   `json:"archived,omitempty"`
	Disabled                  bool                   `json:"disabled,omitempty"`
	OpenIssuesCount           int                    `json:"open_issues_count,omitempty"`
	License                   *License               `json:"license,omitempty"`
	AllowForking              bool                   `json:"allow_forking,omitempty"`
	IsTemplate                bool                   `json:"is_template,omitempty"`
	WebCommitSignoffRequired  bool                   `json:"web_commit_signoff_required,omitempty"`
	Topics                    []string               `json:"topics,omitempty"`
	Visibility                string                 `json:"visibility,omitempty"`
	Forks                     int                    `json:"forks,omitempty"`
	OpenIssues                int                    `json:"open_issues,omitempty"`
	Watchers                  int                    `json:"watchers,omitempty"`
	DefaultBranch             string                 `json:"default_branch,omitempty"`
	Permissions               *Permissions           `json:"permissions,omitempty"`
	AllowSquashMerge          bool                   `json:"allow_squash_merge,omitempty"`
	AllowMergeCommit          bool                   `json:"allow_merge_commit,omitempty"`
	AllowRebaseMerge          bool                   `json:"allow_rebase_merge,omitempty"`
	AllowAutoMerge            bool                   `json:"allow_auto_merge,omitempty"`
	DeleteBranchOnMerge       bool                   `json:"delete_branch_on_merge,omitempty"`
	AllowUpdateBranch         bool                   `json:"allow_update_branch,omitempty"`
	UseSquashPRTitleAsDefault bool                   `json:"use_squash_pr_title_as_default,omitempty"`
	SquashMergeCommitMessage  string                 `json:"squash_merge_commit_message,omitempty"`
	SquashMergeCommitTitle    string                 `json:"squash_merge_commit_title,omitempty"`
	MergeCommitMessage        string                 `json:"merge_commit_message,omitempty"`
	MergeCommitTitle          string                 `json:"merge_commit_title,omitempty"`
	CustomProperties          map[string]interface{} `json:"custom_properties,omitempty"`
	Organization              *UserOrOrg             `json:"organization,omitempty"`
	Parent                    *RepoDetail            `json:"parent,omitempty"`
	Source                    *RepoDetail            `json:"source,omitempty"`
	SecurityAndAnalysis       *SecurityAndAnalysis   `json:"security_and_analysis,omitempty"`
	NetworkCount              int                    `json:"network_count,omitempty"`
	SubscribersCount          int                    `json:"subscribers_count,omitempty"`
	BlankIssuesEnabled        bool                   `json:"blank_issues_enabled,omitempty"`
	Locked                    bool                   `json:"locked,omitempty"`
}

type RepositorySettings struct {
	HasDiscussionsEnabled         bool `json:"has_discussions_enabled"`
	HasIssuesEnabled              bool `json:"has_issues_enabled"`
	HasProjectsEnabled            bool `json:"has_projects_enabled"`
	HasVulnerabilityAlertsEnabled bool `json:"has_vulnerability_alerts_enabled"`
	HasWikiEnabled                bool `json:"has_wiki_enabled"`

	MergeCommitAllowed bool   `json:"merge_commit_allowed"`
	MergeCommitMessage string `json:"merge_commit_message"`
	MergeCommitTitle   string `json:"merge_commit_title"`

	SquashMergeAllowed       bool   `json:"squash_merge_allowed"`
	SquashMergeCommitMessage string `json:"squash_merge_commit_message"`
	SquashMergeCommitTitle   string `json:"squash_merge_commit_title"`

	HasDownloads             bool `json:"has_downloads"`
	HasPages                 bool `json:"has_pages"`
	WebCommitSignoffRequired bool `json:"web_commit_signoff_required"`

	MirrorURL                 *string                `json:"mirror_url"`
	AllowAutoMerge            bool                   `json:"allow_auto_merge"`
	DeleteBranchOnMerge       bool                   `json:"delete_branch_on_merge"`
	AllowUpdateBranch         bool                   `json:"allow_update_branch"`
	UseSquashPRTitleAsDefault bool                   `json:"use_squash_pr_title_as_default"`
	CustomProperties          map[string]interface{} `json:"custom_properties,omitempty"`
	AllowForking              bool                   `json:"allow_forking"`
	IsTemplate                bool                   `json:"is_template"`
	AllowRebaseMerge          bool                   `json:"allow_rebase_merge"`
}

type RepoURLs struct {
	GitURL   string `json:"git_url,omitempty"`
	SSHURL   string `json:"ssh_url,omitempty"`
	CloneURL string `json:"clone_url,omitempty"`
	SVNURL   string `json:"svn_url,omitempty"`
	HTMLURL  string `json:"html_url,omitempty"`
}

type RepoMetrics struct {
	StargazersCount   int `json:"stargazers_count"`
	WatchersCount     int `json:"watchers_count"`
	ForksCount        int `json:"forks_count"`
	OpenIssuesCount   int `json:"open_issues_count"`
	NetworkCount      int `json:"network_count"`
	SubscribersCount  int `json:"subscribers_count"`
	Size              int `json:"size"`
	TotalCommits      int `json:"total_commits"`
	TotalIssues       int `json:"total_issues"`
	TotalBranches     int `json:"total_branches"`
	TotalPullRequests int `json:"total_pull_requests"`
	TotalReleases     int `json:"total_releases"`
}

type FinalRepoDetail struct {
	GitHubRepoID        int                  `json:"github_repo_id,omitempty"`
	NodeID              string               `json:"node_id,omitempty"`
	Name                string               `json:"name,omitempty"`
	FullName            string               `json:"full_name,omitempty"`
	Private             bool                 `json:"private,omitempty"`
	Owner               *UserOrOrg           `json:"owner,omitempty"`
	Description         *string              `json:"description"`
	CreatedAt           string               `json:"created_at,omitempty"`
	UpdatedAt           string               `json:"updated_at,omitempty"`
	PushedAt            string               `json:"pushed_at,omitempty"`
	Homepage            *string              `json:"homepage"`
	Language            *string              `json:"language"`
	License             *License             `json:"license,omitempty"`
	Topics              []string             `json:"topics,omitempty"`
	Visibility          string               `json:"visibility,omitempty"`
	DefaultBranch       string               `json:"default_branch,omitempty"`
	Permissions         *Permissions         `json:"permissions,omitempty"`
	Organization        *UserOrOrg           `json:"organization,omitempty"`
	Parent              *FinalRepoDetail     `json:"parent,omitempty"`
	Source              *FinalRepoDetail     `json:"source,omitempty"`
	SecurityAndAnalysis *SecurityAndAnalysis `json:"security_and_analysis,omitempty"`

	RepositorySettings RepositorySettings `json:"repository_settings"`
	RepoURLs           RepoURLs           `json:"repo_urls"`
	RepoMetrics        RepoMetrics        `json:"repo_metrics"`

	IsArchived                    bool `json:"is_archived"`
	IsDisabled                    bool `json:"is_disabled"`
	IsEmpty                       bool `json:"is_empty"`
	IsFork                        bool `json:"is_fork"`
	IsInOrganization              bool `json:"is_in_organization"`
	IsLocked                      bool `json:"is_locked"`
	IsMirror                      bool `json:"is_mirror"`
	IsPrivate                     bool `json:"is_private"`
	IsSecurityPolicyEnabled       bool `json:"is_security_policy_enabled"`
	IsTemplate                    bool `json:"is_template"`
	IsUserConfigurationRepository bool `json:"is_user_configuration_repository"`
}

type MinimalRepoInfo struct {
	Name  string `json:"name"`
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
}

func main() {
	repoFlag := flag.String("repo", "", "Repository or organization in format https://github.com/<org> or https://github.com/<org>/<repo>")
	maxResultsFlag := flag.Int("max-results", 100, "Maximum number of repositories to fetch (default 100) for org listing")
	flag.Parse()

	if *repoFlag == "" {
		log.Fatal("You must provide a -repo parameter")
	}

	apiToken := os.Getenv("GITHUB_API_TOKEN")
	if apiToken == "" {
		log.Println("GITHUB_API_TOKEN not set; you may only be able to access public repos")
	}

	owner, repoName, err := parseScopeURL(*repoFlag)
	if err != nil {
		log.Fatalf("Error parsing scope URL: %v", err)
	}

	maxResults := *maxResultsFlag
	if maxResults <= 0 {
		maxResults = 100
	}

	sdk := resilientbridge.NewResilientBridge()
	sdk.RegisterProvider("github", adapters.NewGitHubAdapter(apiToken), &resilientbridge.ProviderConfig{
		UseProviderLimits: true,
		MaxRetries:        3,
		BaseBackoff:       0,
	})

	if repoName == "" {
		// Org-level
		allRepos, err := fetchOrgRepos(sdk, owner, maxResults)
		if err != nil {
			log.Fatalf("Error fetching organization repositories: %v", err)
		}

		for _, r := range allRepos {
			repoDetail, err := fetchRepoDetails(sdk, r.Owner.Login, r.Name)
			if err != nil {
				log.Printf("Error fetching details for %s/%s: %v", r.Owner.Login, r.Name, err)
				continue
			}

			finalDetail := transformToFinalRepoDetail(repoDetail)

			// Count metrics
			err = enrichRepoMetrics(sdk, r.Owner.Login, r.Name, finalDetail)
			if err != nil {
				log.Printf("Error enriching repo metrics for %s/%s: %v", r.Owner.Login, r.Name, err)
			}

			data, err := json.MarshalIndent(finalDetail, "", "  ")
			if err != nil {
				log.Printf("Error marshalling repo detail for %s/%s: %v", r.Owner.Login, r.Name, err)
				continue
			}
			fmt.Println(string(data))
		}
	} else {
		// Single repo
		repoDetail, err := fetchRepoDetails(sdk, owner, repoName)
		if err != nil {
			log.Fatalf("Error fetching repository details: %v", err)
		}

		finalDetail := transformToFinalRepoDetail(repoDetail)

		// Count metrics
		err = enrichRepoMetrics(sdk, owner, repoName, finalDetail)
		if err != nil {
			log.Printf("Error enriching repo metrics for %s/%s: %v", owner, repoName, err)
		}

		data, err := json.MarshalIndent(finalDetail, "", "  ")
		if err != nil {
			log.Fatalf("Error marshalling repo detail: %v", err)
		}
		fmt.Println(string(data))
	}
}

func enrichRepoMetrics(sdk *resilientbridge.ResilientBridge, owner, repoName string, finalDetail *FinalRepoDetail) error {
	defaultBranch := finalDetail.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	commitsCount, err := countCommits(sdk, owner, repoName, defaultBranch)
	if err != nil {
		return fmt.Errorf("counting commits: %w", err)
	}
	finalDetail.RepoMetrics.TotalCommits = commitsCount

	issuesCount, err := countIssues(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting issues: %w", err)
	}
	finalDetail.RepoMetrics.TotalIssues = issuesCount

	branchesCount, err := countBranches(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting branches: %w", err)
	}
	finalDetail.RepoMetrics.TotalBranches = branchesCount

	prCount, err := countPullRequests(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting PRs: %w", err)
	}
	finalDetail.RepoMetrics.TotalPullRequests = prCount

	releasesCount, err := countReleases(sdk, owner, repoName)
	if err != nil {
		return fmt.Errorf("counting releases: %w", err)
	}
	finalDetail.RepoMetrics.TotalReleases = releasesCount

	return nil
}

func parseScopeURL(repoURL string) (owner, repo string, err error) {
	if !strings.HasPrefix(repoURL, "https://github.com/") {
		return "", "", fmt.Errorf("URL must start with https://github.com/")
	}
	parts := strings.Split(strings.TrimPrefix(repoURL, "https://github.com/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", fmt.Errorf("invalid URL format")
	}
	owner = parts[0]
	if len(parts) > 1 {
		repo = parts[1]
	}
	return owner, repo, nil
}

func fetchOrgRepos(sdk *resilientbridge.ResilientBridge, org string, maxResults int) ([]MinimalRepoInfo, error) {
	var allRepos []MinimalRepoInfo
	perPage := 100
	page := 1

	for len(allRepos) < maxResults {
		remaining := maxResults - len(allRepos)
		if remaining < perPage {
			perPage = remaining
		}

		endpoint := fmt.Sprintf("/orgs/%s/repos?per_page=%d&page=%d", org, perPage, page)
		listReq := &resilientbridge.NormalizedRequest{
			Method:   "GET",
			Endpoint: endpoint,
			Headers:  map[string]string{"Accept": "application/vnd.github+json"},
		}

		listResp, err := sdk.Request("github", listReq)
		if err != nil {
			return nil, fmt.Errorf("error fetching repos: %w", err)
		}

		if listResp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP error %d: %s", listResp.StatusCode, string(listResp.Data))
		}

		var repos []MinimalRepoInfo
		if err := json.Unmarshal(listResp.Data, &repos); err != nil {
			return nil, fmt.Errorf("error decoding repos list: %w", err)
		}

		if len(repos) == 0 {
			break
		}

		allRepos = append(allRepos, repos...)
		if len(allRepos) >= maxResults {
			break
		}
		page++
	}
	if len(allRepos) > maxResults {
		allRepos = allRepos[:maxResults]
	}
	return allRepos, nil
}

func fetchRepoDetails(sdk *resilientbridge.ResilientBridge, owner, repo string) (*RepoDetail, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: fmt.Sprintf("/repos/%s/%s", owner, repo),
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}
	resp, err := sdk.Request("github", req)
	if err != nil {
		return nil, fmt.Errorf("error fetching repo details: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var detail RepoDetail
	if err := json.Unmarshal(resp.Data, &detail); err != nil {
		return nil, fmt.Errorf("error decoding repo details: %w", err)
	}
	return &detail, nil
}

func transformToFinalRepoDetail(detail *RepoDetail) *FinalRepoDetail {
	var parent *FinalRepoDetail
	if detail.Parent != nil {
		parent = transformToFinalRepoDetail(detail.Parent)
	}
	var source *FinalRepoDetail
	if detail.Source != nil {
		source = transformToFinalRepoDetail(detail.Source)
	}

	isInOrganization := (detail.Organization != nil && detail.Organization.Type == "Organization")
	isMirror := (detail.MirrorURL != nil)
	isSecurityPolicyEnabled := detail.SecurityAndAnalysis != nil && detail.SecurityAndAnalysis.DependabotSecurityUpdates != nil && detail.SecurityAndAnalysis.DependabotSecurityUpdates.Status == "enabled"
	isEmpty := (detail.Size == 0)
	isUserConfigurationRepository := false

	return &FinalRepoDetail{
		GitHubRepoID:        detail.ID,
		NodeID:              detail.NodeID,
		Name:                detail.Name,
		FullName:            detail.FullName,
		Private:             detail.Private,
		Owner:               detail.Owner,
		Description:         detail.Description,
		CreatedAt:           detail.CreatedAt,
		UpdatedAt:           detail.UpdatedAt,
		PushedAt:            detail.PushedAt,
		Homepage:            detail.Homepage,
		Language:            detail.Language,
		License:             detail.License,
		Topics:              detail.Topics,
		Visibility:          detail.Visibility,
		DefaultBranch:       detail.DefaultBranch,
		Permissions:         detail.Permissions,
		Organization:        detail.Organization,
		Parent:              parent,
		Source:              source,
		SecurityAndAnalysis: detail.SecurityAndAnalysis,

		RepositorySettings: RepositorySettings{
			HasDiscussionsEnabled:         detail.HasDiscussions,
			HasIssuesEnabled:              detail.HasIssues,
			HasProjectsEnabled:            detail.HasProjects,
			HasWikiEnabled:                detail.HasWiki,
			HasVulnerabilityAlertsEnabled: isSecurityPolicyEnabled,

			MergeCommitAllowed: detail.AllowMergeCommit,
			MergeCommitMessage: detail.MergeCommitMessage,
			MergeCommitTitle:   detail.MergeCommitTitle,

			SquashMergeAllowed:       detail.AllowSquashMerge,
			SquashMergeCommitMessage: detail.SquashMergeCommitMessage,
			SquashMergeCommitTitle:   detail.SquashMergeCommitTitle,

			HasDownloads:             detail.HasDownloads,
			HasPages:                 detail.HasPages,
			WebCommitSignoffRequired: detail.WebCommitSignoffRequired,

			MirrorURL:                 detail.MirrorURL,
			AllowAutoMerge:            detail.AllowAutoMerge,
			DeleteBranchOnMerge:       detail.DeleteBranchOnMerge,
			AllowUpdateBranch:         detail.AllowUpdateBranch,
			UseSquashPRTitleAsDefault: detail.UseSquashPRTitleAsDefault,
			CustomProperties:          detail.CustomProperties,
			AllowForking:              detail.AllowForking,
			IsTemplate:                detail.IsTemplate,
			AllowRebaseMerge:          detail.AllowRebaseMerge,
		},

		RepoURLs: RepoURLs{
			GitURL:   detail.GitURL,
			SSHURL:   detail.SSHURL,
			CloneURL: detail.CloneURL,
			SVNURL:   detail.SVNURL,
			HTMLURL:  detail.HTMLURL,
		},

		RepoMetrics: RepoMetrics{
			StargazersCount:  detail.StargazersCount,
			WatchersCount:    detail.WatchersCount,
			ForksCount:       detail.ForksCount,
			OpenIssuesCount:  detail.OpenIssuesCount,
			NetworkCount:     detail.NetworkCount,
			SubscribersCount: detail.SubscribersCount,
			Size:             detail.Size,
		},

		IsArchived:                    detail.Archived,
		IsDisabled:                    detail.Disabled,
		IsEmpty:                       isEmpty,
		IsFork:                        detail.Fork,
		IsInOrganization:              isInOrganization,
		IsLocked:                      detail.Locked,
		IsMirror:                      isMirror,
		IsPrivate:                     detail.Private,
		IsSecurityPolicyEnabled:       isSecurityPolicyEnabled,
		IsTemplate:                    detail.IsTemplate,
		IsUserConfigurationRepository: isUserConfigurationRepository,
	}
}

func countCommits(sdk *resilientbridge.ResilientBridge, owner, repoName, defaultBranch string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/commits?sha=%s&per_page=1", owner, repoName, defaultBranch)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countIssues(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/issues?state=all&per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countBranches(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/branches?per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countPullRequests(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/pulls?state=all&per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countReleases(sdk *resilientbridge.ResilientBridge, owner, repoName string) (int, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/releases?per_page=1", owner, repoName)
	return countItemsFromEndpoint(sdk, endpoint)
}

func countItemsFromEndpoint(sdk *resilientbridge.ResilientBridge, endpoint string) (int, error) {
	req := &resilientbridge.NormalizedRequest{
		Method:   "GET",
		Endpoint: endpoint,
		Headers:  map[string]string{"Accept": "application/vnd.github+json"},
	}

	resp, err := sdk.Request("github", req)
	if err != nil {
		return 0, fmt.Errorf("error fetching data: %w", err)
	}

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(resp.Data))
	}

	var linkHeader string
	for k, v := range resp.Headers {
		if strings.ToLower(k) == "link" {
			linkHeader = v
			break
		}
	}

	if linkHeader == "" {
		// No Link header, check if there's at least one item
		if len(resp.Data) > 2 {
			var items []interface{}
			if err := json.Unmarshal(resp.Data, &items); err != nil {
				return 1, nil
			}
			return len(items), nil
		}
		return 0, nil
	}

	lastPage, err := parseLastPage(linkHeader)
	if err != nil {
		return 0, fmt.Errorf("could not parse last page: %w", err)
	}

	return lastPage, nil
}

func parseLastPage(linkHeader string) (int, error) {
	re := regexp.MustCompile(`page=(\d+)>; rel="last"`)
	matches := re.FindStringSubmatch(linkHeader)
	if len(matches) < 2 {
		return 1, nil
	}
	var lastPage int
	_, err := fmt.Sscanf(matches[1], "%d", &lastPage)
	if err != nil {
		return 0, err
	}
	return lastPage, nil
}
