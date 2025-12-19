package d365

import "fmt"

// PublishWebResource publishes a web resource
func (c *Client) PublishWebResource(webResourceID string) error {
	path := "/PublishXml"

	paramXML := fmt.Sprintf(
		"<importexportxml><webresources><webresource>%s</webresource></webresources></importexportxml>",
		webResourceID,
	)

	payload := map[string]string{
		"ParameterXml": paramXML,
	}

	_, err := c.doRequest("POST", path, payload)
	return err
}
