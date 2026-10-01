package canvas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Teacher struct {
	ID             int64  `json:"id"`
	DisplayName    string `json:"display_name"`
	AvatarImageURL string `json:"avatar_image_url"`
	HTMLURL        string `json:"html_url"`
	Email          string `json:"email,omitempty"`
}

type Enrollment struct {
	Type                 string   `json:"type"`
	Role                 string   `json:"role"`
	ComputedCurrentScore *float64 `json:"computed_current_score"`
	ComputedFinalScore   *float64 `json:"computed_final_score"`
	ComputedCurrentGrade *string  `json:"computed_current_grade"`
	ComputedFinalGrade   *string  `json:"computed_final_grade"`
}

type Course struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	CourseCode  string       `json:"course_code"`
	Enrollments []Enrollment `json:"enrollments"`
	Teacher     []Teacher    `json:"teachers"`
}

type Client struct {
	domain     string
	token      string
	httpClient *http.Client
}

func NewClient(domain string, token string) *Client {
	return &Client{
		domain: domain,
		token:  token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

}

func (c *Client) FetchFavoriteCoursesGrades(ctx context.Context) ([]Course, error) {
	//using client generate the expected end point
	endpoint := fmt.Sprintf("%s/api/v1/users/self/favorites/courses", c.domain)
	//get the actual endpoint by parsing the string
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid canvas url: %w", err)
	}
	//create a query to the endpoint
	q := reqURL.Query()
	//specify that we need total_scores and teacher info
	q.Add("include[]", "total_scores")
	q.Add("include[]", "teachers")
	//encode the query so the endpoint can process the request
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating canvas request: %w", err)
	}
	//create the get header with our api token
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	//send the request to the website and log the response
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed executing canvas request: %w", err)
	}
	//always close the request
	defer resp.Body.Close()

	//check status to see if a succesful response like 200 was logged
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		//unsuccesful api request results in an error
		return nil, fmt.Errorf("canvas API returned status %d: %s", resp.StatusCode, string(body))
	}

	//create a Course slice
	var courses []Course
	// decode the response into the course slice, if an error occurs during this an error is sent by the server
	if err := json.NewDecoder(resp.Body).Decode(&courses); err != nil {
		return nil, fmt.Errorf("failed decoding canvas response: %w", err)
	}
	//return the resulting course json
	return courses, nil
}
