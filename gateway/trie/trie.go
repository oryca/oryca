package trie

import (
	"github.com/oryca/oryca/gateway/model"
	"regexp"
	"strings"
	"sync"
)

type TrieNode struct {
	children   map[string]*TrieNode
	paramChild *TrieNode
	paramName  string // ชื่อ path param ของ paramChild เช่น "collectionId"
	isEnd      bool
	service    *model.GatewayService
	resource   *model.ResourcePath

	// wildcard suffix (path ลงท้ายด้วย /*) ที่ anchor อยู่ node นี้ — เก็บแยกจาก
	// isEnd/service/resource ด้านบนเสมอ เพราะ exact match ที่จบพอดีตรงนี้ (เช่น "/items")
	// กับ wildcard ที่เริ่มจากตรงนี้ (เช่น "/items/*") เป็นคนละ resource กัน ถ้าใช้ field
	// ร่วมกันตัวที่ insert ทีหลังจะเขียนทับอีกตัวไปเลย (เคยเป็นบั๊กจริง — ราคา/ปลายทาง
	// ของ resource หนึ่งถูกอีก resource หนึ่งที่ path segment ซ้อนกันเขียนทับ)
	hasWildcard      bool
	wildcardService  *model.GatewayService
	wildcardResource *model.ResourcePath
}

type PathTrie struct {
	root *TrieNode
	mu   sync.RWMutex
}

type PathMatch struct {
	Service    *model.GatewayService
	Resource   *model.ResourcePath
	PathParams map[string]string
	Remaining  string
}

func New() *PathTrie {
	return &PathTrie{
		root: &TrieNode{children: make(map[string]*TrieNode)},
	}
}

func isPathParam(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

func paramName(segment string) string {
	return segment[1 : len(segment)-1]
}

func (pt *PathTrie) Insert(svc *model.GatewayService) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	for _, res := range svc.ResourcePaths {
		pt.insertResource(svc, res)
	}
}

func (pt *PathTrie) insertResource(svc *model.GatewayService, res *model.ResourcePath) {
	fullPath := strings.TrimRight(svc.BasePath, "/") + "/" + strings.TrimLeft(res.Path, "/")
	segments := strings.Split(strings.Trim(fullPath, "/"), "/")
	current := pt.root

	for _, seg := range segments {
		if seg == "" {
			continue
		}
		// wildcard suffix — mark เป็น wildcard anchor ที่ node ปัจจุบัน แยกจาก exact-match
		// ของ node เดียวกัน (ถ้ามี resource อื่นจบพอดีที่ path เดียวกันนี้) แล้วหยุด
		if seg == "*" {
			current.hasWildcard = true
			current.wildcardService = svc
			current.wildcardResource = res
			return
		}
		if isPathParam(seg) {
			if current.paramChild == nil {
				current.paramChild = &TrieNode{children: make(map[string]*TrieNode)}
				current.paramName = paramName(seg)
			}
			current = current.paramChild
		} else {
			if current.children[seg] == nil {
				current.children[seg] = &TrieNode{children: make(map[string]*TrieNode)}
			}
			current = current.children[seg]
		}
	}

	current.isEnd = true
	current.service = svc
	current.resource = res
}

func (pt *PathTrie) Delete(serviceID string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.deleteFromNode(pt.root, serviceID)
}

func (pt *PathTrie) deleteFromNode(node *TrieNode, serviceID string) {
	if node == nil {
		return
	}
	if node.isEnd && node.service != nil && node.service.ID == serviceID {
		node.isEnd = false
		node.service = nil
		node.resource = nil
	}
	if node.hasWildcard && node.wildcardService != nil && node.wildcardService.ID == serviceID {
		node.hasWildcard = false
		node.wildcardService = nil
		node.wildcardResource = nil
	}
	for _, child := range node.children {
		pt.deleteFromNode(child, serviceID)
	}
	if node.paramChild != nil {
		pt.deleteFromNode(node.paramChild, serviceID)
	}
}

func (pt *PathTrie) LoadBulk(services []*model.GatewayService) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.root = &TrieNode{children: make(map[string]*TrieNode)}
	for _, svc := range services {
		for _, res := range svc.ResourcePaths {
			pt.insertResource(svc, res)
		}
	}
}

func (pt *PathTrie) FindBestMatch(requestPath string) *PathMatch {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	segments := strings.Split(strings.Trim(requestPath, "/"), "/")
	return pt.search(pt.root, segments, 0, map[string]string{}, nil)
}

func (pt *PathTrie) search(node *TrieNode, segments []string, idx int, params map[string]string, resourcePath *model.ResourcePath) *PathMatch {
	if idx >= len(segments) {
		if node.isEnd {
			paramsCopy := make(map[string]string, len(params))
			for k, v := range params {
				paramsCopy[k] = v
			}
			return &PathMatch{
				Service:    node.service,
				Resource:   node.resource,
				PathParams: paramsCopy,
			}
		}
		// ไม่มี exact resource ที่ node นี้ แต่ถ้ามี wildcard anchor อยู่ (เช่น resource ถูก
		// กำหนดเป็น "/*" ล้วนๆ ไม่มี path อื่นแยกต่างหาก) bare path ที่ตรงกับ parent ของ "*"
		// พอดีก็ควร match ด้วย — exact ยัง priority สูงกว่าเสมอ (เช็คก่อนหน้านี้แล้ว) กรณีนี้
		// เป็น fallback เฉพาะตอนไม่มี exact แข่งอยู่ที่ node เดียวกันเท่านั้น
		if node.hasWildcard {
			paramsCopy := copyParams(params)
			return &PathMatch{
				Service:    node.wildcardService,
				Resource:   node.wildcardResource,
				PathParams: paramsCopy,
			}
		}
		return nil
	}

	seg := segments[idx]

	// กรณี 1: exact match — priority สูงสุด
	if child, ok := node.children[seg]; ok {
		if m := pt.search(child, segments, idx+1, params, resourcePath); m != nil {
			return m
		}
	}

	// กรณี 2: pattern match เช่น {y}.png
	for childSeg, child := range node.children {
		if strings.Contains(childSeg, "{") && strings.Contains(childSeg, ".") {
			if matchesPattern(seg, childSeg) {
				// copy เฉพาะเมื่อ pattern match แล้วเท่านั้น
				newParams := copyParams(params)
				newParams[extractPatternParamName(childSeg)] = extractParamFromPattern(seg)
				if m := pt.search(child, segments, idx+1, newParams, resourcePath); m != nil {
					return m
				}
			}
		}
	}

	// กรณี 3: path param {name}
	if node.paramChild != nil {
		newParams := copyParams(params)
		key := node.paramName
		if key == "" {
			key = "_"
		}
		newParams[key] = seg
		if m := pt.search(node.paramChild, segments, idx+1, newParams, resourcePath); m != nil {
			return m
		}
	}

	// กรณี 4: suffix wildcard
	if node.hasWildcard {
		paramsCopy := copyParams(params)
		return &PathMatch{
			Service:    node.wildcardService,
			Resource:   node.wildcardResource,
			PathParams: paramsCopy,
			Remaining:  strings.Join(segments[idx:], "/"),
		}
	}

	return nil
}

func copyParams(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func matchesPattern(segment, pattern string) bool {
	if !strings.Contains(pattern, "{") {
		return false
	}
	segExt, patExt := "", ""
	if i := strings.LastIndex(segment, "."); i != -1 {
		segExt = segment[i:]
	}
	if i := strings.LastIndex(pattern, "."); i != -1 {
		patExt = pattern[i:]
	}
	return segExt != "" && segExt == patExt
}

var reParam = regexp.MustCompile(`\{([^}]+)\}`)

func extractPatternParamName(pattern string) string {
	m := reParam.FindStringSubmatch(pattern)
	if len(m) > 1 {
		return m[1]
	}
	return "_"
}

func extractParamFromPattern(segment string) string {
	if i := strings.LastIndex(segment, "."); i != -1 {
		return segment[:i]
	}
	return segment
}
