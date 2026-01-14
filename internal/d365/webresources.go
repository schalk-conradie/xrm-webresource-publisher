package d365

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// WebResourceType represents the type of web resource
type WebResourceType int

const (
	WebResourceTypeHTML WebResourceType = 1
	WebResourceTypeCSS  WebResourceType = 2
	WebResourceTypeJS   WebResourceType = 3
	WebResourceTypeXML  WebResourceType = 4
	WebResourceTypePNG  WebResourceType = 5
	WebResourceTypeJPG  WebResourceType = 6
	WebResourceTypeGIF  WebResourceType = 7
	WebResourceTypeXAP  WebResourceType = 8
	WebResourceTypeXSL  WebResourceType = 9
	WebResourceTypeICO  WebResourceType = 10
	WebResourceTypeSVG  WebResourceType = 11
	WebResourceTypeResx WebResourceType = 12
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

// CreateWebResourceRequest represents the request to create a web resource
type CreateWebResourceRequest struct {
	Name            string `json:"name"`
	DisplayName     string `json:"displayname"`
	Content         string `json:"content"`
	WebResourceType int    `json:"webresourcetype"`
}

// GetWebResourceTypeFromExtension returns the web resource type based on file extension
func GetWebResourceTypeFromExtension(filename string) (WebResourceType, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".html", ".htm":
		return WebResourceTypeHTML, nil
	case ".css":
		return WebResourceTypeCSS, nil
	case ".js":
		return WebResourceTypeJS, nil
	case ".xml":
		return WebResourceTypeXML, nil
	case ".png":
		return WebResourceTypePNG, nil
	case ".jpg", ".jpeg":
		return WebResourceTypeJPG, nil
	case ".gif":
		return WebResourceTypeGIF, nil
	case ".xap":
		return WebResourceTypeXAP, nil
	case ".xsl", ".xslt":
		return WebResourceTypeXSL, nil
	case ".ico":
		return WebResourceTypeICO, nil
	case ".svg":
		return WebResourceTypeSVG, nil
	case ".resx":
		return WebResourceTypeResx, nil
	default:
		return 0, fmt.Errorf("unsupported file type: %s", ext)
	}
}

// ListWebResources retrieves web resources (HTML, CSS, JS only, custom/unmanaged only)
func (c *Client) ListWebResources() ([]WebResource, error) {
	// Filter by webresourcetype: 1=HTML, 2=CSS, 3=JS
	// Also filter by ismanaged eq false to only get unmanaged resources
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

// CreateWebResource creates a new web resource and returns its ID
func (c *Client) CreateWebResource(name, displayName, base64Content string, resourceType WebResourceType) (string, error) {
	path := "/webresourceset"

	payload := CreateWebResourceRequest{
		Name:            name,
		DisplayName:     displayName,
		Content:         base64Content,
		WebResourceType: int(resourceType),
	}

	body, err := c.doRequest("POST", path, payload)
	if err != nil {
		return "", fmt.Errorf("failed to create web resource: %w", err)
	}

	// Parse response to get the created resource ID
	var response WebResource
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return response.ID, nil
}
