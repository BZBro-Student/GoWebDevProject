package canvas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Module struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Position    int          `json:"position"`
	State       string       `json:"state"`
	CompletedAt *string      `json:"completed_at,omitempty"`
	Items       []ModuleItem `json:"items,omitempty"`
}

type ModuleItem struct {
	ID                    int64                  `json:"id"`
	Title                 string                 `json:"title"`
	Type                  string                 `json:"type"`
	Position              int                    `json:"position"`
	HTMLURL               string                 `json:"html_url"`
	CompletionRequirement *CompletionRequirement `json:"completion_requirement,omitempty"`
}

type CompletionRequirement struct {
	Type      string   `json:"type"`
	MinScore  *float64 `json:"min_score,omitempty"`
	Completed bool     `json:"completed"`
}

func (c *Client) FetchModules(ctx context.Context, courseID int64) ([]Module, error) {
	//create endpoint using the domain and the expected course
	endpoint := fmt.Sprintf("%s/api/v1/courses/%d/modules", c.domain, courseID)
	// parse a useable url type via the string
	reqURL, err := url.Parse(endpoint)

	if err != nil {
		return nil, fmt.Errorf("invalid canvas url: %w", err)
	}
	
	// create the query body
	q := reqURL.Query()
	q.Add("include[]", "items")
	q.Add("include[]", "content_details")
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	//create the query header
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	//execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed executing canvas request: %w", err)
	}
	defer resp.Body.Close()
	//check status of the request response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("canvas API returned status %d: %s", resp.StatusCode, string(body))
	}
	//module slice
	var modules []Module
	//store json into the modules
	if err := json.NewDecoder(resp.Body).Decode(&modules); err != nil {
		return nil, fmt.Errorf("failed decoding canvas response: %w", err)
	}
	//return the resulting module
	return modules, nil
}
