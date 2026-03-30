// Package protocol provides protocol abstraction for golocate.
package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
)

// Result represents a search result.
type Result struct {
	Path string
	Name string
	Size int64
}

// SearchResults represents search results with metadata.
type SearchResults struct {
	Results []*Result
	Count   int
	Total   int
}

// ResponseSender 封装协议实现和连接对象
// 所有协议判断逻辑都在这个结构体内部，业务方法只调用统一的方法
type ResponseSender struct {
	conn  net.Conn
	proto Protocol
}

// NewResponseSender 创建响应发送器
func NewResponseSender(conn net.Conn, proto Protocol) *ResponseSender {
	return &ResponseSender{
		conn:  conn,
		proto: proto,
	}
}

// Send 通用发送方法，根据响应类型自动路由到具体发送逻辑
// 支持的类型：*SearchResults, map[string]any, *Result
func (s *ResponseSender) Send(resp any) error {
	switch v := resp.(type) {
	case *SearchResults:
		return s.sendSearchResults(v.Results, v.Count, v.Total)
	case []*Result:
		// 兼容直接传入 []*Result 的情况，默认 count 和 total 都等于长度
		return s.sendSearchResults(v, len(v), len(v))
	case *Result:
		// 单个结果，转换为切片
		return s.sendSearchResults([]*Result{v}, 1, 1)
	case map[string]any:
		return s.sendMap(v)
	default:
		return fmt.Errorf("unsupported response type: %T", resp)
	}
}

// sendSearchResults 发送搜索结果（批量）
func (s *ResponseSender) sendSearchResults(results []*Result, count, total int) error {
	switch s.proto.Name() {
	case "json":
		// JSON 协议：发送单个响应，包含所有结果
		resultList := make([]map[string]any, len(results))
		for i, result := range results {
			resultList[i] = map[string]any{
				"path": result.Path,
				"name": result.Name,
				"size": result.Size,
			}
		}

		resp := map[string]any{
			"count": count,
			"total": total,
			"paths": resultList,
		}

		log.Printf("[JSON Protocol] Sending response: count=%d, total=%d, paths=%d", count, total, len(resultList))
		writer := bufio.NewWriter(s.conn)
		encoder := json.NewEncoder(writer)
		if err := encoder.Encode(resp); err != nil {
			log.Printf("[JSON Protocol] Encode error: %v", err)
			return err
		}
		if err := writer.Flush(); err != nil {
			log.Printf("[JSON Protocol] Flush error: %v", err)
			return err
		}
		log.Printf("[JSON Protocol] Response sent successfully")
		return nil

	case "json-rpc":
		// JSON-RPC 协议：发送单个响应，包含所有结果
		resultList := make([]map[string]any, len(results))
		for i, result := range results {
			resultList[i] = map[string]any{
				"path": result.Path,
				"name": result.Name,
				"size": result.Size,
			}
		}

		jsonrpcResp := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]any{
				"count": count,
				"total": total,
				"paths": resultList,
			},
		}

		writer := bufio.NewWriter(s.conn)
		encoder := json.NewEncoder(writer)
		if err := encoder.Encode(jsonrpcResp); err != nil {
			return err
		}
		return writer.Flush()

	default:
		// Fast 协议：发送 fast 协议格式的响应
		paths := make([]string, len(results))
		for i, result := range results {
			paths[i] = result.Path
		}

		writer := bufio.NewWriter(s.conn)
		fmt.Fprintf(writer, "count=%d\n", count)
		fmt.Fprintf(writer, "total=%d\n", total)
		fmt.Fprint(writer, "\n") // empty line marks end of headers

		for _, path := range paths {
			fmt.Fprintln(writer, path)
		}

		return writer.Flush()
	}
}

// sendMap 发送 map 类型的响应（用于 status 和 config）
func (s *ResponseSender) sendMap(result map[string]any) error {
	log.Printf("[ResponseSender] sendMap called, protocol=%s", s.proto.Name())
	switch s.proto.Name() {
	case "json":
		// JSON 协议：发送 {"type":"status","result":{...}}
		resp := map[string]any{
			"type":   "status",
			"result": result,
		}
		writer := bufio.NewWriter(s.conn)
		encoder := json.NewEncoder(writer)
		log.Printf("[JSON Protocol] Sending map response: %v", resp)
		if err := encoder.Encode(resp); err != nil {
			log.Printf("[JSON Protocol] Encode error: %v", err)
			return err
		}
		if err := writer.Flush(); err != nil {
			log.Printf("[JSON Protocol] Flush error: %v", err)
			return err
		}
		log.Printf("[JSON Protocol] Map response sent successfully")
		return nil

	case "json-rpc":
		// JSON-RPC 协议：发送 {"jsonrpc":"2.0","id":1,"result":{...}}
		jsonrpcResp := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  result,
		}
		writer := bufio.NewWriter(s.conn)
		encoder := json.NewEncoder(writer)
		if err := encoder.Encode(jsonrpcResp); err != nil {
			return err
		}
		return writer.Flush()

	default:
		// Fast 协议：发送 key=value 格式的响应
		writer := bufio.NewWriter(s.conn)
		for key, value := range result {
			fmt.Fprintf(writer, "%s=%v\n", key, value)
		}
		fmt.Fprint(writer, "\n") // empty line marks end of headers
		return writer.Flush()
	}
}

// SendError 发送错误响应
func (s *ResponseSender) SendError(errMsg string) error {
	switch s.proto.Name() {
	case "json":
		// JSON 协议：发送 {"type":"error","error":"..."}
		resp := map[string]any{
			"type":  "error",
			"error": errMsg,
		}
		writer := bufio.NewWriter(s.conn)
		encoder := json.NewEncoder(writer)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
		return writer.Flush()

	case "json-rpc":
		// JSON-RPC 协议：发送 {"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"..."}}
		jsonrpcResp := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"error": map[string]any{
				"code":    -1,
				"message": errMsg,
			},
		}
		writer := bufio.NewWriter(s.conn)
		encoder := json.NewEncoder(writer)
		if err := encoder.Encode(jsonrpcResp); err != nil {
			return err
		}
		return writer.Flush()

	default:
		// Fast 协议：发送 error=...
		writer := bufio.NewWriter(s.conn)
		fmt.Fprintf(writer, "error=%s\n", errMsg)
		fmt.Fprint(writer, "\n") // empty line marks end of headers
		return writer.Flush()
	}
}
