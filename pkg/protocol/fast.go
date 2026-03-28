// Package protocol provides fast text protocol implementation.
package protocol

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// fastProtocol implements the Protocol interface for fast text protocol.
type fastProtocol struct{}

// NewFastProtocol creates a new fast protocol instance.
func NewFastProtocol() Protocol {
	return &fastProtocol{}
}

func (p *fastProtocol) ParseRequest(reader *bufio.Reader) (*Request, error) {
	req := &Request{
		Limit: 100, // default
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\n")
		
		// Empty line marks end of headers
		if line == "" {
			break
		}

		// Parse key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // skip invalid lines
		}

		key := strings.ToLower(parts[0])
		value := parts[1]

		switch key {
		case "method":
			req.Method = value
		case "mode":
			req.Mode = value
		case "path":
			req.Path = value
		case "content":
			req.Content = value
		case "accept_response_format":
			req.AcceptResponseFormat = value
		case "ignore_case":
			req.IgnoreCase = value == "true"
		case "limit":
			if n, err := strconv.Atoi(value); err == nil {
				req.Limit = n
			}
		}
	}

	return req, nil
}

func (p *fastProtocol) WriteRequest(writer *bufio.Writer, req *Request) error {
	fmt.Fprintf(writer, "method=%s\n", req.Method)
	if req.Mode != "" {
		fmt.Fprintf(writer, "mode=%s\n", req.Mode)
	}
	if req.Path != "" {
		fmt.Fprintf(writer, "path=%s\n", req.Path)
	}
	if req.Content != "" {
		fmt.Fprintf(writer, "content=%s\n", req.Content)
	}
	fmt.Fprintf(writer, "ignore_case=%v\n", req.IgnoreCase)
	fmt.Fprintf(writer, "limit=%d\n", req.Limit)
	fmt.Fprint(writer, "\n") // empty line marks end of headers
	return writer.Flush()
}

func (p *fastProtocol) ParseResponse(reader *bufio.Reader) (*Response, error) {
	resp := &Response{}

	// Parse headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\n")
		
		// Empty line marks end of headers
		if line == "" {
			break
		}

		// Parse key=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(parts[0])
		value := parts[1]

		switch key {
		case "count":
			if n, err := strconv.Atoi(value); err == nil {
				resp.Count = n
			}
		case "error":
			resp.Error = value
		}
	}

	// Read paths
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\n")
		if line == "" {
			break
		}
		resp.Paths = append(resp.Paths, line)
	}

	return resp, nil
}

func (p *fastProtocol) WriteResponse(writer *bufio.Writer, resp *Response) error {
	if resp.Error != "" {
		fmt.Fprintf(writer, "error=%s\n", resp.Error)
	}
	fmt.Fprintf(writer, "count=%d\n", resp.Count)
	fmt.Fprint(writer, "\n") // empty line marks end of headers
	
	for _, path := range resp.Paths {
		fmt.Fprintln(writer, path)
	}
	
	return writer.Flush()
}

func (p *fastProtocol) Name() string {
	return "fast"
}
