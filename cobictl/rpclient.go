package cobictl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	jsonrpc "github.com/catalogfi/cobi/rpc"
)

type client struct {
	User      string
	Pass      string
	Protocol  string
	RPCServer string
}

type Client interface {
	SendPostRequest(method string, jsonData []byte) (json.RawMessage, error)
}

func NewClient(userName string, password string, protocol string, rpcServer string) Client {
	return &client{
		User:      userName,
		Pass:      password,
		Protocol:  protocol,
		RPCServer: rpcServer,
	}
}

// sendPostRequest sends the marshalled JSON-RPC command using HTTP-POST mode
// to the server described in the passed config struct.  It also attempts to
// unmarshal the response as a JSON-RPC response and returns either the result
// field or the error field depending on whether or not there is an error.
func (c *client) SendPostRequest(method string, jsonData []byte) (json.RawMessage, error) {
	payload := jsonrpc.Request{
		Version: "2.0",
		Method:  method,
		Params:  json.RawMessage(jsonData),
	}
	marshalledJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, (fmt.Errorf("failed to marshal payload: %w", err))
	}

	url := c.Protocol + "://" + c.RPCServer
	bodyReader := bytes.NewReader(marshalledJSON)
	httpRequest, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		return nil, err
	}
	httpRequest.Close = true
	httpRequest.Header.Set("Content-Type", "application/json")

	// Configure basic access authorization.
	httpRequest.SetBasicAuth(c.User, c.Pass)

	// Create the new HTTP client that is configured according to the user-
	// specified options and submit the request.
	httpResponse, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}

	// Read the raw bytes and close the response.
	respBytes, err := ioutil.ReadAll(httpResponse.Body)
	httpResponse.Body.Close()
	if err != nil {
		err = fmt.Errorf("error reading json reply: %v", err)
		return nil, err
	}

	// Handle unsuccessful HTTP responses
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		// Generate a standard error to return if the server body is
		// empty.  This should not happen very often, but it's better
		// than showing nothing in case the target server has a poor
		// implementation.
		if len(respBytes) == 0 {
			return nil, fmt.Errorf("%d %s", httpResponse.StatusCode,
				http.StatusText(httpResponse.StatusCode))
		}
		return nil, fmt.Errorf("%s", respBytes)
	}

	// Unmarshal the response.
	var resp jsonrpc.Response
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("error Occrured : %s with Data : %s", resp.Error.Message, resp.Error.Data)
	}
	return resp.Result, nil
}
