package d365

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// Solution represents a Dynamics 365 solution
type Solution struct {
	ID           string `json:"solutionid"`
	UniqueName   string `json:"uniquename"`
	FriendlyName string `json:"friendlyname"`
	Version      string `json:"version"`
}

// SolutionResponse represents the API response for solutions
type SolutionResponse struct {
	Value []Solution `json:"value"`
}

// ListSolutions retrieves unmanaged solutions ordered by createdon descending
func (c *Client) ListSolutions() ([]Solution, error) {
	filter := url.QueryEscape("ismanaged eq false")
	orderby := url.QueryEscape("createdon desc")
	path := "/solutions?$select=solutionid,uniquename,friendlyname,version&$filter=" + filter + "&$orderby=" + orderby

	body, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var response SolutionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.Value, nil
}

// AddWebResourceToSolution adds a web resource to a solution
func (c *Client) AddWebResourceToSolution(solutionUniqueName, webResourceID string) error {
	path := "/AddSolutionComponent"

	// ComponentType 61 = Web Resource
	payload := map[string]any{
		"ComponentId":               webResourceID,
		"ComponentType":             61,
		"SolutionUniqueName":        solutionUniqueName,
		"AddRequiredComponents":     false,
		"DoNotIncludeSubcomponents": false,
	}

	_, err := c.doRequest("POST", path, payload)
	if err != nil {
		return fmt.Errorf("failed to add to solution: %w", err)
	}

	return nil
}
