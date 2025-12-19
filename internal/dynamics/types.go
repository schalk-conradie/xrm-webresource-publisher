package dynamics

// WebResource represents a web resource in Dynamics 365
type WebResource struct {
	ID              string `json:"webresourceid"`
	Name            string `json:"name"`
	DisplayName     string `json:"displayname"`
	WebResourceType int    `json:"webresourcetype"`
	Content         string `json:"content,omitempty"`
	VersionNumber   int64  `json:"versionnumber"`
}
