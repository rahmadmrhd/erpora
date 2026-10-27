// Package egress adalah stub minimal SDK untuk fixture arch test.
package egress

// Client meniru egress.Client asli.
type Client struct{}

// New meniru egress.New asli.
func New() (*Client, error) { return &Client{}, nil }
