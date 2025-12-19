package d365

import (
	"encoding/json"
	"net/url"
)

// WebResource represents a Dynamics 365 web resource
type WebResource struct {
	ID      string `json:"webresourceid"`
	Name    string `json:"name"`
	Version int64  `json:"versionnumber,omitempty"`
}

// WebResourceResponse represents the API response for web resources
type WebResourceResponse struct {
	Value []WebResource `json:"value"`
}

// ListWebResources retrieves web resources (HTML, CSS, JS only, custom/unmanaged only)
func (c *Client) ListWebResources() ([]WebResource, error) {
	// Filter by webresourcetype: 1=HTML, 2=CSS, 3=JS
	// Also filter by ismanaged eq false to only get custom (unmanaged) resources
	filter := url.QueryEscape("(webresourcetype eq 1 or webresourcetype eq 2 or webresourcetype eq 3) and ismanaged eq false")
	path := "/webresourceset?$select=webresourceid,name,versionnumber&$filter=" + filter + "&$orderby=name"

	body, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var response WebResourceResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.Value, nil
}

// UpdateWebResourceContent updates the content of a web resource
func (c *Client) UpdateWebResourceContent(webResourceID, base64Content string) error {
	path := "/webresourceset(" + webResourceID + ")"

	payload := map[string]string{
		"content": base64Content,
	}

	_, err := c.doRequest("PATCH", path, payload)
	return err
}
