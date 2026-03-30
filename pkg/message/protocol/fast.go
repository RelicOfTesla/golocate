// Package protocol provides fast text protocol implementation.
package protocol

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
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
	req, _, err := p.ParseRequestWithRemainder(reader)
	return req, err
}

// ParseRequestWithRemainder parses a fast protocol request and returns remaining data (for sticky packet handling).
func (p *fastProtocol) ParseRequestWithRemainder(reader *bufio.Reader) (*Request, []byte, error) {
	req := &Request{
		Limit: 100, // default
	}

	// Read lines until we find an empty line (end of request)
	var lines []string
	var lineBuffer bytes.Buffer
	
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// EOF reached, stop reading
				break
			}
			return nil, nil, err
		}
		
		line = strings.TrimRight(line, "\n")
		
		// Empty line marks end of current request
		if line == "" {
			break
		}
		
		lines = append(lines, line)
		lineBuffer.WriteString(line)
		lineBuffer.WriteString("\n")
	}

	// Check for sticky packets: read all remaining buffered data
	var remainder []byte
	if reader.Buffered() > 0 {
		// Read all remaining data from the buffer
		remainingBytes, err := io.ReadAll(reader)
		if err != nil {
			log.Printf("[Fast Protocol] Error reading remainder: %v", err)
		} else if len(remainingBytes) > 0 {
			remainder = remainingBytes
			log.Printf("[Fast Protocol] Detected sticky packet, remainder: %d bytes", len(remainder))
		}
	}

	// Parse key=value pairs
	for _, line := range lines {
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
		case "id":
			// Try to parse as integer first, then as string
			if n, err := strconv.Atoi(value); err == nil {
				req.ID = n
			} else {
				req.ID = value
			}
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
			log.Printf("[Fast Protocol] Parsed ignore_case: value=%q, result=%v", value, req.IgnoreCase)
		case "limit":
			if n, err := strconv.Atoi(value); err == nil {
				req.Limit = n
			}
		case "basename":
			req.Basename = value == "true"
		case "regex":
			req.Regex = value == "true"
		case "extended_regex":
			req.ExtendedRegex = value == "true"
		case "offset":
			if n, err := strconv.ParseInt(value, 10, 64); err == nil {
				req.Offset = n
			}
		case "sort_field":
			req.SortField = value
		case "sort_order":
			req.SortOrder = value
		}
	}

	return req, remainder, nil
}

func (p *fastProtocol) WriteRequest(writer *bufio.Writer, req *Request) error {
	fmt.Fprintf(writer, "method=%s\n", req.Method)
	if req.ID != nil {
		fmt.Fprintf(writer, "id=%v\n", req.ID)
	}
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
	fmt.Fprintf(writer, "basename=%v\n", req.Basename)
	fmt.Fprintf(writer, "regex=%v\n", req.Regex)
	fmt.Fprintf(writer, "extended_regex=%v\n", req.ExtendedRegex)
	fmt.Fprintf(writer, "offset=%d\n", req.Offset)
	if req.SortField != "" {
		fmt.Fprintf(writer, "sort_field=%s\n", req.SortField)
	}
	if req.SortOrder != "" {
		fmt.Fprintf(writer, "sort_order=%s\n", req.SortOrder)
	}
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
		case "id":
			// Try to parse as integer first, then as string
			if n, err := strconv.Atoi(value); err == nil {
				resp.ID = n
			} else {
				resp.ID = value
			}
		case "count":
			if n, err := strconv.Atoi(value); err == nil {
				resp.Count = n
			}
		case "total":
			if n, err := strconv.Atoi(value); err == nil {
				resp.Total = n
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
	if resp.ID != nil {
		fmt.Fprintf(writer, "id=%v\n", resp.ID)
	}
	if resp.Error != "" {
		fmt.Fprintf(writer, "error=%s\n", resp.Error)
	}
	fmt.Fprintf(writer, "count=%d\n", resp.Count)
	fmt.Fprintf(writer, "total=%d\n", resp.Total)
	fmt.Fprint(writer, "\n") // empty line marks end of headers
	
	for _, path := range resp.Paths {
		fmt.Fprintln(writer, path)
	}
	
	return writer.Flush()
}

func (p *fastProtocol) Name() string {
	return "fast"
}

// NewStreamMsg creates a new stream message (reserved for future streaming support).
// Currently returns nil, error as streaming is not yet implemented.
func (p *fastProtocol) NewStreamMsg() (any, error) {
	return nil, fmt.Errorf("streaming not implemented for fast protocol")
}
