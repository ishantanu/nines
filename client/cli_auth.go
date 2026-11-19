package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

// CLIAuthenticationCapabilities discovers Nines-mediated login. It neither
// sends the client's bearer credential nor starts an interactive authorization.
func (c *Client) CLIAuthenticationCapabilities(ctx context.Context) (v1.CLIAuthenticationCapabilities, error) {
	var out v1.CLIAuthenticationCapabilities
	err := c.cliAuthRequest(ctx, "GET", "/auth/cli/capabilities", nil, &out)
	return out, err
}
func (c *Client) StartCLIDeviceAuthorization(ctx context.Context, challenge string) (v1.CLIDeviceAuthorization, error) {
	var out v1.CLIDeviceAuthorization
	err := c.cliAuthRequest(ctx, "POST", "/auth/cli/device/start", v1.CLIDeviceStart{Tenant: c.tenant, Challenge: challenge}, &out)
	return out, err
}
func (c *Client) PollCLIDeviceAuthorization(ctx context.Context, in v1.CLIDevicePoll) (v1.CLIDeviceResult, error) {
	var out v1.CLIDeviceResult
	err := c.cliAuthRequest(ctx, "POST", "/auth/cli/device/token", in, &out)
	return out, err
}
func (c *Client) CancelCLIDeviceAuthorization(ctx context.Context, in v1.CLIDevicePoll) error {
	return c.cliAuthRequest(ctx, "POST", "/auth/cli/device/cancel", in, nil)
}
func (c *Client) cliAuthRequest(ctx context.Context, method, path string, in, out any) error {
	var b []byte
	var err error
	if in != nil {
		b, err = json.Marshal(in)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("CLI authentication request failed")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		return &APIError{StatusCode: res.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, (128<<10)+1))
	if err != nil || len(data) > 128<<10 {
		return errors.New("CLI authentication response unreadable or exceeds bounds")
	}
	if out == nil {
		return nil
	}
	if json.Unmarshal(data, out) != nil {
		return errors.New("invalid CLI authentication response")
	}
	return nil
}
